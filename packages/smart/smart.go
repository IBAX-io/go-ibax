/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/IBAX-io/go-ibax/packages/common"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	qb "github.com/IBAX-io/go-ibax/packages/storage/sqldb/queryBuilder"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
	"github.com/pkg/errors"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

const (
	// MaxPrice is a maximal value that price function can return
	MaxPrice = 100000000000000000
)

const (
	CallDelayedContract = "@1CallDelayedContract"
	NewUserContract     = "@1NewUser"
	NewBadBlockContract = "@1NewBadBlock"
)

var (
	builtinContract = map[string]bool{
		CallDelayedContract: true,
		NewBadBlockContract: true,
		BftEvidenceContract: true,
	}
)

// SmartContract is storing smart contract data
type SmartContract struct {
	ChildChain     bool
	Rollback       bool
	FullAccess     bool
	SysUpdate      bool
	VM             *script.VM
	TxSmart        *types.SmartTransaction
	TxData         map[string]any
	TxContract     *Contract
	TxFuel         int64           // The fuel of executing contract
	TxCost         int64           // Maximum cost of executing contract
	TxUsedCost     decimal.Decimal // Used cost of CPU resources
	TXBlockFuel    decimal.Decimal
	BlockHeader    *types.BlockHeader
	PreBlockHeader *types.BlockHeader
	Loop           map[string]bool
	Hash           []byte
	Payload        []byte
	Timestamp      int64
	TxSignature    []byte
	TxSize         int64
	Size           common.StorageSize
	SignerKey      crypto.AccountKey // the key that signed the transaction
	DbTransaction  *sqldb.DbTransaction
	Rand           *rand.Rand
	FlushRollback  []*FlushInfo
	Notifications  types.Notifications
	GenBlock       bool
	TimeLimit      int64
	Key            *sqldb.Key
	RollBackTx     []*types.RollbackTx
	multiPays      multiPays
	// FeeCredits are the fees the transaction pays to accounts, which the block credits
	FeeCredits      []FeeCredit
	Penalty         bool
	TokenEcosystems map[int64]any
	OutputsMap      map[sqldb.KeyUTXO][]sqldb.SpentInfo
	TxInputsMap     map[sqldb.KeyUTXO][]sqldb.SpentInfo
	TxOutputsMap    map[sqldb.KeyUTXO][]sqldb.SpentInfo
	PrevSysPar      map[string]string
	EcoParams       []sqldb.EcoParam
	// callAs is the account that CallContractAs runs a contract as, 0 outside it
	callAs int64
}

// AppendStack adds an element to the stack of contract call or removes the top element when name is empty
func (sc *SmartContract) AppendStack(fn string) error {
	if sc.isAllowStack(fn) {
		cont := sc.TxContract
		for _, item := range cont.StackCont {
			if item == fn {
				return fmt.Errorf(eContractLoop, fn)
			}
		}
		cont.StackCont = append(cont.StackCont, fn)
		sc.TxContract.Extend[script.Extend_stack] = cont.StackCont
	}
	return nil
}

func (sc *SmartContract) PopStack(fn string) {
	if sc.isAllowStack(fn) {
		cont := sc.TxContract
		if len(cont.StackCont) > 0 {
			cont.StackCont = cont.StackCont[:len(cont.StackCont)-1]
			sc.TxContract.Extend[script.Extend_stack] = cont.StackCont
		}
	}
}

func (sc *SmartContract) isAllowStack(fn string) bool {
	// Stack contains only contracts
	c := VMGetContract(sc.VM, fn, uint32(sc.TxSmart.EcosystemID))
	return c != nil
}

func InitVM() {
	script.GetVM().SetExtendCost(getCost)
	script.GetVM().SetFuncCallsDB(funcCallsDBP)
	script.GetVM().Extend(extendData(defineVMType()))
}

// extendData is what the contracts of a VM of the type call in Go, and the parameters of it the
// VM passes by itself
func extendData(vt script.VMType) *script.ExtendData {
	return &script.ExtendData{
		Objects: EmbedFuncs(vt), AutoPars: map[string]string{
			`*smart.SmartContract`: `sc`,
			`*script.RunTime`:      `rt`,
		},
		WriteFuncs: writeFuncs,
	}
}

func defineVMType() script.VMType {
	if conf.Config.IsChildChain() {
		return script.VMType_CLB
	}
	if conf.Config.IsChainHost() {
		return script.VMType_CLBMaster
	}
	return script.VMType_Smart
}

// GetLogger is returning logger
func (sc *SmartContract) GetLogger() *log.Entry {
	var name string
	if sc.TxContract != nil {
		name = sc.TxContract.Name
	}
	return log.WithFields(log.Fields{"tx": fmt.Sprintf("%x", sc.Hash), "clb": sc.ChildChain, "name": name, "tx_eco": sc.TxSmart.EcosystemID})
}

func GetAllContracts() (string, error) {
	var ret []string
	for k := range script.GetVM().Objects {
		ret = append(ret, k)
	}

	sort.Strings(ret)
	resultByte, err := json.Marshal(ret)
	result := string(resultByte)
	return result, err
}

// ActivateContract sets Active status of the contract in script.GetVM()
func ActivateContract(tblid, state int64, active bool) {
	for i, item := range script.GetVM().CodeBlock.Children {
		if item != nil && item.Type == script.ObjectType_Contract {
			cinfo := item.GetContractInfo()
			if cinfo.Owner.TableID == tblid && cinfo.Owner.StateID == uint32(state) {
				script.GetVM().Children[i].GetContractInfo().Owner.Active = active
			}
		}
	}
}

// SetContractWallet changes WalletID of the contract in script.GetVM()
func SetContractWallet(sc *SmartContract, tblid, state int64, wallet int64) error {
	if err := sc.serialOnly("the wallet of a contract"); err != nil {
		return err
	}
	if err := validateAccess(sc, "SetContractWallet"); err != nil {
		return err
	}
	for i, item := range script.GetVM().CodeBlock.Children {
		if item != nil && item.Type == script.ObjectType_Contract {
			cinfo := item.GetContractInfo()
			if cinfo.Owner.TableID == tblid && cinfo.Owner.StateID == uint32(state) {
				script.GetVM().Children[i].GetContractInfo().Owner.WalletID = wallet
			}
		}
	}
	return nil
}

func (sc *SmartContract) getExtend() map[string]any {
	var block, blockTime, blockKeyID, blockNodePosition int64
	var perBlockHash string
	if sc.BlockHeader != nil {
		block = sc.BlockHeader.BlockId
		blockKeyID = sc.BlockHeader.KeyId
		blockTime = sc.BlockHeader.Timestamp
		blockNodePosition = sc.BlockHeader.NodePosition
	}
	if sc.PreBlockHeader != nil {
		perBlockHash = hex.EncodeToString(sc.PreBlockHeader.BlockHash)
	}
	head := sc.TxSmart
	extend := map[string]any{
		script.Extend_type:          head.ID,
		script.Extend_time:          sc.Timestamp,
		script.Extend_ecosystem_id:  head.EcosystemID,
		script.Extend_node_position: blockNodePosition,
		script.Extend_block:         block,
		script.Extend_key_id:        sc.Key.ID,
		script.Extend_account_id:    sc.Key.AccountID,
		script.Extend_block_key_id:  blockKeyID,
		script.Extend_parent:        ``,
		script.Extend_txcost:        sc.GetContractLimit(),
		script.Extend_txhash:        sc.Hash,
		//script.Extend_result:              ``,
		script.Extend_sc:                  sc,
		script.Extend_contract:            sc.TxContract,
		script.Extend_block_time:          blockTime,
		script.Extend_original_contract:   ``,
		script.Extend_this_contract:       ``,
		script.Extend_guest_key:           consts.GuestKey,
		script.Extend_guest_account:       consts.GuestAddress,
		script.Extend_black_hole_key:      converter.HoleAddrMap[converter.BlackHoleAddr].K,
		script.Extend_black_hole_account:  converter.HoleAddrMap[converter.BlackHoleAddr].S,
		script.Extend_white_hole_key:      converter.HoleAddrMap[converter.WhiteHoleAddr].K,
		script.Extend_white_hole_account:  converter.HoleAddrMap[converter.WhiteHoleAddr].S,
		script.Extend_pre_block_data_hash: perBlockHash,
		script.Extend_gen_block:           sc.GenBlock,
		script.Extend_time_limit:          sc.TimeLimit,
	}
	// The contracts called so far, so that code run apart from the transaction's contract
	// (ContractConditions, the conditions of rows and tables) sees the same $stack. A copy: the
	// stack grows and shrinks in place
	if sc.TxContract != nil && len(sc.TxContract.StackCont) > 0 {
		extend[script.Extend_stack] = append([]any(nil), sc.TxContract.StackCont...)
	}
	for key, val := range sc.TxData {
		extend[key] = val
	}

	return extend
}

func PrefixName(table string) (prefix, name string) {
	name = table
	if off := strings.IndexByte(table, '_'); off > 0 && table[0] >= '0' && table[0] <= '9' {
		prefix = table[:off]
		name = table[off+1:]
	}
	return
}

func (sc *SmartContract) IsCustomTable(table string) (isCustom bool, err error) {
	prefix, name := PrefixName(table)
	if len(prefix) > 0 {
		tables := &sqldb.Table{}
		tables.SetTablePrefix(prefix)
		found, err := tables.Get(sc.DbTransaction, name)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

// isParamsTable tells whether the table is one of the parameters of the ecosystem, the
// parameters or the parameters of its applications
func (sc *SmartContract) isParamsTable(table string) bool {
	name := qb.GetTableName(sc.TxSmart.EcosystemID, table)
	return name == `1_parameters` || name == `1_app_params`
}

// founderGate tells how a write of the parameters of the ecosystem of the transaction is checked.
// See paramsFounderGate.
func (sc *SmartContract) founderGate() (gated, allowed bool, err error) {
	mode, err := sqldb.ControlMode(sc.DbTransaction, sc.TxSmart.EcosystemID)
	if err != nil {
		return false, false, logErrorDB(err, "getting the control mode")
	}
	founder, err := EcosysParam(sc, `founder_account`)
	if err != nil {
		return false, false, err
	}
	gated, allowed = paramsFounderGate(mode, sc.actingKeyID(), converter.StrToInt64(founder))
	return gated, allowed, nil
}

// paramsFounderGate tells how a write of the parameters of an ecosystem is checked. In control
// mode 2 the votings govern the ecosystem, and its founder may have handed it over and destroyed
// his key: the permissions of the tables and of their columns check the write (gated is false),
// which name the contracts that change the parameters, themselves run by a voting. In any other
// mode the founder alone writes them (gated is true; allowed tells whether the key is his).
func paramsFounderGate(controlMode, keyID, founder int64) (gated, allowed bool) {
	if controlMode == 2 {
		return false, false
	}
	return true, keyID != 0 && keyID == founder
}

// bftTables are the tables of the evidence and the reviews of the BFT consensus (F18). The
// evidence and review transactions of the node write them; no contract writes them or changes
// their columns or permissions, whatever their permissions say.
var bftTables = map[string]bool{`1_bft_evidence`: true, `1_bft_reviews`: true}

// errBftTable refuses a contract a write or a change of a table of bftTables
var errBftTable = errors.New(`the table is written by the consensus only`)

// denyBftTable refuses a change of a table of bftTables, named as a contract names it
func (sc *SmartContract) denyBftTable(name string) error {
	if table := qb.GetTableName(sc.TxSmart.EcosystemID, strings.ToLower(name)); bftTables[table] {
		return fmt.Errorf("table %s: %w", table, errBftTable)
	}
	return nil
}

func (sc *SmartContract) AccessTablePerm(table, action string) (map[string]string, error) {
	var (
		err             error
		tablePermission map[string]string
	)
	logger := sc.GetLogger()
	isRead := action == `read`
	if !isRead && bftTables[strings.ToLower(table)] {
		return tablePermission, fmt.Errorf("table %s: %w", table, errBftTable)
	}
	if sc.isParamsTable(table) {
		if isRead {
			return tablePermission, nil
		}
		gated, allowed, err := sc.founderGate()
		if err != nil {
			return tablePermission, err
		}
		if gated {
			if allowed {
				return tablePermission, nil
			}
			logger.WithFields(log.Fields{"type": consts.AccessDenied}).Error("Access denied")
			return tablePermission, errAccessDenied
		}
	}

	if isCustom, err := sc.IsCustomTable(table); err != nil {
		logger.WithFields(log.Fields{"table": table, "error": err, "type": consts.DBError}).Error("checking custom table")
		return tablePermission, err
	} else if !isCustom {
		if isRead {
			return tablePermission, nil
		}
		return tablePermission, fmt.Errorf(eNotCustomTable, table)
	}

	prefix, name := PrefixName(table)
	tables := &sqldb.Table{}
	tables.SetTablePrefix(prefix)
	tablePermission, err = tables.GetPermissions(sc.DbTransaction, name, "")
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting table permissions")
		return tablePermission, err
	}
	if len(tablePermission[action]) > 0 {
		ret, err := sc.EvalIf(tablePermission[action])
		if err != nil {
			logger.WithFields(log.Fields{"table": table, "action": action, "permissions": tablePermission[action], "error": err, "type": consts.EvalError}).Error("evaluating table permissions for action")
			return tablePermission, err
		}
		if !ret {
			logger.WithFields(log.Fields{"table": table, "action": action, "permissions": tablePermission[action], "type": consts.EvalError}).Error("access denied")
			return tablePermission, fmt.Errorf("table: %w", errAccessDenied)
		}
	}
	return tablePermission, nil
}

// AccessTable checks the access right to the table
func (sc *SmartContract) AccessTable(table, action string) error {
	if action != `read` && bftTables[strings.ToLower(table)] {
		return fmt.Errorf("table %s: %w", table, errBftTable)
	}
	if sc.FullAccess {
		return nil
	}
	_, err := sc.AccessTablePerm(table, action)
	return err
}

func getPermColumns(input string) (perm permColumn, err error) {
	if strings.HasPrefix(input, `{`) {
		err = unmarshalJSON([]byte(input), &perm, `on perm columns`)
	} else {
		perm.Update = input
	}
	return
}

// AccessColumns checks access rights to the columns
func (sc *SmartContract) AccessColumns(table string, columns *[]string, update bool) error {
	logger := sc.GetLogger()
	if sc.FullAccess {
		return nil
	}
	if sc.isParamsTable(table) {
		if !update {
			return nil
		}
		gated, allowed, err := sc.founderGate()
		if err != nil {
			return err
		}
		if gated {
			if allowed {
				return nil
			}
			log.WithFields(log.Fields{"key_id": sc.actingKeyID()}).Error("ACCESS DENIED")
			return errAccessDenied
		}
	}
	prefix, name := PrefixName(table)
	tables := &sqldb.Table{}
	tables.SetTablePrefix(prefix)
	found, err := tables.Get(sc.DbTransaction, name)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting table columns")
		return err
	}
	if !found {
		if !update {
			return nil
		}
		return fmt.Errorf(eTableNotFound, table)
	}
	var cols map[string]string
	err = json.Unmarshal([]byte(tables.Columns), &cols)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("getting table columns")
		return err
	}
	colNames := make([]string, 0, len(*columns))
	for _, col := range *columns {
		if col == `*` {
			for column := range cols {
				colNames = append(colNames, column)
			}
			continue
		}
		colNames = append(colNames, col)
	}

	colList := make([]string, len(colNames))
	for i, col := range colNames {
		colname := converter.Sanitize(col, `->`)
		if strings.Contains(colname, `->`) {
			colname = colname[:strings.Index(colname, `->`)]
		}
		colList[i] = colname
	}
	checked := make(map[string]bool)
	var notaccess bool
	for i, name := range colList {
		if status, ok := checked[name]; ok {
			if !status {
				colList[i] = ``
			}
			continue
		}
		cond := cols[name]
		if len(cond) > 0 {
			perm, err := getPermColumns(cond)
			if err != nil {
				logger.WithFields(log.Fields{"type": consts.InvalidObject, "error": err}).Error("getting access columns")
				return err
			}
			if update {
				cond = perm.Update
			} else {
				cond = perm.Read
			}
			if len(cond) > 0 {
				ret, err := sc.EvalIf(cond)
				if err != nil {
					logger.WithFields(log.Fields{"condition": cond, "column": name,
						"type": consts.EvalError}).Error("evaluating condition")
					return err
				}
				checked[name] = ret
				if !ret {
					if update {
						logger.WithFields(log.Fields{"table": table, "column": name, "condition": cond, "type": consts.EvalError}).Error("access denied")
						return fmt.Errorf("column: %w", errAccessDenied)
					}
					colList[i] = ``
					notaccess = true
				}
			}
		}
	}
	if !update && notaccess {
		retColumn := make([]string, 0)
		for i, val := range colList {
			if val != `` {
				retColumn = append(retColumn, colNames[i])
			}
		}
		if len(retColumn) == 0 {
			return errAccessDenied
		}
		*columns = retColumn
	}
	return nil
}

// AccessRights checks the access right by executing the condition value
func (sc *SmartContract) AccessRights(condition string, iscondition bool) error {
	sp := &sqldb.StateParameter{}
	prefix := converter.Int64ToStr(sc.TxSmart.EcosystemID)

	sp.SetTablePrefix(prefix)
	_, err := sp.Get(sc.DbTransaction, condition)
	if err != nil {
		return err
	}
	conditions := sp.Value
	if iscondition {
		conditions = sp.Conditions
	}
	if len(conditions) > 0 {
		ret, err := sc.EvalIf(conditions)
		if err != nil {
			return err
		}
		if !ret {
			return errAccessDenied
		}
	} else {
		return fmt.Errorf(eNotCondition, condition)
	}
	return nil
}

// EvalIf counts and returns the logical value of the specified expression
func (sc *SmartContract) EvalIf(conditions string) (bool, error) {
	return sc.VM.EvalIf(conditions, uint32(sc.TxSmart.EcosystemID), sc.getExtend())
}

// GetContractLimit returns the default maximal cost of contract
func (sc *SmartContract) GetContractLimit() (ret int64) {
	// default maximum cost of F
	if len(sc.TxSmart.MaxSum) > 0 {
		sc.TxCost = converter.StrToInt64(sc.TxSmart.MaxSum)
	} else {
		sc.TxCost = syspar.GetMaxCost()
	}
	return sc.TxCost
}

func (sc *SmartContract) GetSignedBy(public []byte) (int64, error) {
	signedBy := sc.TxSmart.KeyID
	if sc.TxSmart.SignedBy != 0 {
		var isNode bool
		signedBy = sc.TxSmart.SignedBy
		// A transaction that names a signer other than its own key is a node-signed delayed
		// contract. The transfer transactions carry no contract, so such a signer is refused
		// here: without this a header could name a node and skip the contract rule below.
		if sc.TxContract == nil {
			return 0, fmt.Errorf("%w: %v", errDelayedContract, `signed by another key without a contract`)
		}
		if syspar.IsCandidateNodeMode() {
			return signedBy, nil
		}
		honorNodes := syspar.GetNodes()
		delay := sqldb.DelayedContract{}
		if ok, _ := delay.GetByContract(sc.DbTransaction, sc.TxContract.Name); !ok && !builtinContract[sc.TxContract.Name] {
			return 0, fmt.Errorf("%w: %v", errDelayedContract, sc.TxContract.Name)
		}
		if len(honorNodes) > 0 {
			for _, node := range honorNodes {
				if crypto.Address(node.PublicKey) == signedBy {
					isNode = true
					break
				}
			}
		} else {
			isNode = crypto.Address(syspar.GetNodePubKey()) == signedBy
		}

		if !isNode {
			return 0, errDelayedContract
		}
	} else if len(public) > 0 {
		key, err := crypto.ParseAccountKey(public)
		if err != nil {
			return 0, err
		}
		if key.Address() != sc.TxSmart.KeyID {
			return 0, errDiffKeys
		}
	}
	return signedBy, nil
}

// CallContract calls the contract functions according to the specified flags
func (sc *SmartContract) CallContract(point string) (string, error) {
	var (
		result string
		err    error
	)
	logger := sc.GetLogger()

	retError := func(err error) (string, error) {
		eText := err.Error()
		if !strings.HasPrefix(eText, `{`) && err != script.ErrVMTimeLimit {
			err = script.SetVMError(`panic`, eText)
		}
		return ``, err
	}

	// The signer is checked before the transaction runs (CheckTxSign, called from
	// SmartTransactionParser.Action for every transaction type); a contract body is never
	// entered without it.
	sc.FeeCredits = nil
	needPayment := sc.needPayment()
	if needPayment {
		err = sc.prepareMultiPay()
		if err != nil {
			logger.WithError(err).Error("prepare multi")
			return ``, err
		}
	}

	sc.TxContract.Extend = sc.getExtend()
	if err = sc.AppendStack(sc.TxContract.Name); err != nil {
		logger.WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("loop in contract")
		return retError(err)
	}
	sc.VM = script.GetVM()

	ctrctExtend := sc.TxContract.Extend
	before := ctrctExtend[script.Extend_txcost].(int64)
	txSizeFuel := syspar.GetSizeFuel() * sc.TxSize / 1024
	ctrctExtend[script.Extend_txcost] = ctrctExtend[script.Extend_txcost].(int64) - txSizeFuel

	_, nameContract := converter.ParseName(sc.TxContract.Name)
	ctrctExtend[script.Extend_original_contract] = nameContract
	ctrctExtend[script.Extend_this_contract] = nameContract

	methods := []string{`conditions`, `action`}
	start := time.Now()
	err = script.RunContractById(sc.VM, int32(sc.TxSmart.ID), methods, sc.TxContract.Extend, sc.Hash)
	if err != nil && sc.GenBlock && syspar.IsBftConsensus() && time.Since(start) >= time.Duration(sc.TimeLimit)*time.Millisecond {
		// the time limit of the proposer stopped the contract, and the validators run it with
		// no limit: the block leaves it out rather than takes it as failed
		return ``, ErrTimeLeft
	}
	if ctrctExtend[script.Extend_txcost].(int64) < 0 {
		sc.TxFuel = before
	} else {
		sc.TxFuel = before - ctrctExtend[script.Extend_txcost].(int64)
	}
	sc.TxUsedCost = decimal.New(sc.TxFuel, 0)

	if err == nil {
		if ctrctExtend[script.Extend_result] != nil {
			result = fmt.Sprint(ctrctExtend[script.Extend_result])
			if !utf8.ValidString(result) {
				result, err = retError(errNotValidUTF)
			}
			if len(result) > 255 {
				result = result[:255] + `...`
			}
		}
	}
lp:
	if err != nil {
		sc.RollBackTx = nil
		if errReset := sc.DbTransaction.ResetSavepoint(point); errReset != nil {
			return retError(errors.Wrap(err, errReset.Error()))
		}
		if needPayment {
			if errPay := sc.payContract(true); errPay != nil {
				sc.RollBackTx = nil
				sc.FeeCredits = nil
				if errRollsp := sc.DbTransaction.RollbackSavepoint(point); errRollsp != nil {
					return retError(errors.Wrap(err, errRollsp.Error()))
				}
				return errors.Wrap(err, errPay.Error()).Error(), nil
			}
			return err.Error(), nil
		}
		return retError(err)
	}

	if needPayment {
		if errPay := sc.payContract(false); errPay != nil {
			err = errPay
			goto lp
		}
	}
	return result, nil
}

// CheckTxSign verifies that the key that signs the transaction is the signer the header names,
// and that the signature holds. It is called before a transaction runs, whatever its type: a
// contract transaction does not reach its contract body and a transfer transaction does not move
// funds until this passes. The key is the one registered for the signer in 1_keys when there is
// one, otherwise the public key in the header for a signer that is still registering.
func (sc *SmartContract) CheckTxSign() error {
	var public []byte
	if len(sc.TxSmart.PublicKey) > 0 && string(sc.TxSmart.PublicKey) != `null` {
		public = sc.TxSmart.PublicKey
	}
	signedBy, err := sc.GetSignedBy(public)
	if err != nil {
		return err
	}

	// A frozen ecosystem (a suspended agency of the federal chain) runs no transaction until ecosystem
	// 1 resumes it
	frozen, err := sqldb.IsFrozen(sc.DbTransaction, sc.TxSmart.EcosystemID)
	if err != nil {
		sc.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting ecosystem")
		return err
	}
	if frozen {
		err = fmt.Errorf(eEcoFrozen, sc.TxSmart.EcosystemID)
		sc.GetLogger().WithFields(log.Fields{"type": consts.AccessDenied, "error": err}).Error("frozen ecosystem")
		return err
	}
	isFound, err := sc.Key.SetTablePrefix(sc.TxSmart.EcosystemID).Get(sc.DbTransaction, signedBy)
	if err != nil {
		sc.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting wallet")
		return err
	}

	if !isFound {
		// A key the ecosystem does not know signs only its own registration: @1NewUser, with the
		// public key in the header (GetSignedBy has checked that it is the signer's)
		if sc.TxContract == nil || sc.TxContract.Name != NewUserContract || sc.TxSmart.SignedBy != 0 || len(public) == 0 {
			err = fmt.Errorf(eEcoKeyNotFound, converter.AddressToString(signedBy), sc.TxSmart.EcosystemID)
			sc.GetLogger().WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("looking for keyid")
			return err
		}
		// $key_id and $account_id are the signer's, as for any other transaction
		sc.Key.ID = signedBy
		sc.Key.AccountID = converter.AddressToString(signedBy)
	}
	if isFound && len(sc.Key.Conditions) > 0 {
		err = fmt.Errorf(eKeyControlled, converter.AddressToString(signedBy))
		sc.GetLogger().WithFields(log.Fields{"type": consts.AccessDenied, "error": err}).Error("signed by an account of contracts")
		return err
	}
	if isFound && sc.Key.Disable() {
		err = fmt.Errorf(eEcoKeyDisable, converter.AddressToString(signedBy), sc.TxSmart.EcosystemID)
		sc.GetLogger().WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("disable keyid")
		return err
	}
	var registered []byte
	if isFound {
		registered = sc.Key.PublicKey
	}
	key, err := syspar.GetAccountAlgorithms().Signer(registered, public, signedBy, sc.blockTime())
	if err != nil {
		sc.GetLogger().WithFields(log.Fields{"type": consts.CryptoError, "error": err, "key_id": signedBy}).Error("account key of the signer")
		return err
	}
	signature, err := utils.TxSignature(sc.TxSignature)
	if err != nil {
		sc.GetLogger().WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err}).Error("reading tx signature")
		return err
	}
	ok, err := key.Verify(sc.Hash, signature)
	if err != nil || !ok {
		sc.GetLogger().WithFields(log.Fields{"type": consts.InvalidObject, "error": err}).Error("incorrect sign")
		return errIncorrectSign
	}
	sc.SignerKey = key
	return nil
}

// blockTime is the time of the block the transaction is in, in Unix seconds: what the days of
// account_algorithms are compared with. Off the chain (child chains, checks before a block) it is
// the clock.
func (sc *SmartContract) blockTime() int64 {
	if sc.BlockHeader == nil || sc.ChildChain {
		return time.Now().Unix()
	}
	return sc.BlockHeader.Timestamp
}
