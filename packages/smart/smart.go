/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/crypto"

	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/utils"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

// Contract contains the information about the contract.
type Contract struct {
	Name          string
	Called        uint32
	FreeRequest   bool
	TxGovAccount  int64   // state wallet
	Rate          float64 // money rate
	TableAccounts string
	StackCont     []interface{} // Stack of called contracts
	Extend        *map[string]interface{}
	Block         *script.Block
}

func (c *Contract) Info() *script.ContractInfo {
	return c.Block.Info.(*script.ContractInfo)
}

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
		NewUserContract:     true,
		NewBadBlockContract: true,
	}
)

var (
	smartVM   *script.VM
	smartTest = make(map[string]string)
)

func testValue(name string, v ...interface{}) {
	smartTest[name] = fmt.Sprint(v...)
}

// GetTestValue returns the test value of the specified key
func GetTestValue(name string) string {
	return smartTest[name]
}

// GetLogger is returning logger
func (sc SmartContract) GetLogger() *log.Entry {
	var name string
	if sc.TxContract != nil {
		name = sc.TxContract.Name
	}
	return log.WithFields(log.Fields{"tx": fmt.Sprintf("%x", sc.TxHash), "obs": sc.OBS, "name": name, "tx_eco": sc.TxSmart.EcosystemID})
}

func InitVM() {
	vm := GetVM()

	vmt := defineVMType()

	EmbedFuncs(vm, vmt)
}

func newVM() *script.VM {
	vm := script.NewVM()
	vm.Extern = true
	vm.Extend(&script.ExtendData{Objects: map[string]interface{}{
		"Println": fmt.Println,
		"Sprintf": fmt.Sprintf,
}

func init() {
	smartVM = newVM()
}

// GetVM is returning smart vm
func GetAllContracts() (string, error) {
	var ret []string
	for k, _ := range smartVM.Objects {
		ret = append(ret, k)
	}

	sort.Strings(ret)
	resultByte, err := json.Marshal(ret)
	result := string(resultByte)
	return result, err
}

var SmartObjects map[string]*script.ObjInfo
var Chid uint32

func SavepointSmartVMObjects() {
	SmartObjects = make(map[string]*script.ObjInfo)
	for k, v := range smartVM.Objects {
		SmartObjects[k] = v
	}
	Chid = uint32(len(smartVM.Children))
}

func RollbackSmartVMObjects() {
	smartVM.Objects = make(map[string]*script.ObjInfo)
	for k, v := range SmartObjects {
		smartVM.Objects[k] = v
	}

	smartVM.Children = smartVM.Children[:Chid]
	SmartObjects = nil
}

func ReleaseSmartVMObjects() {
	SmartObjects = nil
	Chid = 0
}

// GetVM is returning smart vm
func GetVM() *script.VM {
	return smartVM
}

func vmExternOff(vm *script.VM) {
	vm.FlushExtern()
}

func vmCompile(vm *script.VM, src string, owner *script.OwnerInfo) error {
	return vm.Compile([]rune(src), owner)
}

// VMCompileBlock is compiling block
func VMCompileBlock(vm *script.VM, src string, owner *script.OwnerInfo) (*script.Block, error) {
	return vm.CompileBlock([]rune(src), owner)
}

func getContractList(src string) (list []string) {
	for _, funcCond := range []string{`ContractConditions`, `ContractAccess`} {
		if strings.Contains(src, funcCond) {
			if ret := regexp.MustCompile(funcCond +
				`\(\s*(.*)\s*\)`).FindStringSubmatch(src); len(ret) == 2 {
				for _, item := range strings.Split(ret[1], `,`) {
					list = append(list, strings.Trim(item, "\"` "))
				}
			}
		}
	}
	return
}

func VMCompileEval(vm *script.VM, src string, prefix uint32) error {
	var ok bool
	if len(src) == 0 {
		return nil
	}
	allowed := []string{`0`, `1`, `true`, `false`, `ContractConditions\(\s*\".*\"\s*\)`,
		`ContractAccess\(\s*\".*\"\s*\)`, `RoleAccess\(\s*.*\s*\)`}
	for _, v := range allowed {
		re := regexp.MustCompile(`^` + v + `$`)
		if re.Match([]byte(src)) {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf(eConditionNotAllowed, src)
	}
	err := vm.CompileEval(src, prefix)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`^@?[\d\w_]+$`)
	for _, item := range getContractList(src) {
		if len(item) == 0 || !re.Match([]byte(item)) {
			return errIncorrectParameter
		}
	}
	return nil
}

func VMEvalIf(vm *script.VM, src string, state uint32, extend *map[string]interface{}) (bool, error) {
	return vm.EvalIf(src, state, extend)
}

func VMFlushBlock(vm *script.VM, root *script.Block) {
	vm.FlushBlock(root)
}

func vmExtend(vm *script.VM, ext *script.ExtendData) {
	vm.Extend(ext)
}

func VMRun(vm *script.VM, block *script.Block, params []interface{}, extend *map[string]interface{}) (ret []interface{}, err error) {
	var cost int64
	if ecost, ok := (*extend)[`txcost`]; ok {
		cost = ecost.(int64)
	} else {
		cost = syspar.GetMaxCost()
	}
	rt := vm.RunInit(cost)
	ret, err = rt.Run(block, params, extend)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.VMError, "error": err, "original_contract": (*extend)[`original_contract`], "this_contract": (*extend)[`this_contract`], "ecosystem_id": (*extend)[`ecosystem_id`]}).Error("running block in smart vm")
		return nil, err
	}
	(*extend)[`txcost`] = rt.Cost()
	return
}

func VMGetContract(vm *script.VM, name string, state uint32) *Contract {
	if len(name) == 0 {
		return nil
	}
	name = script.StateName(state, name)
	obj, ok := vm.Objects[name]

	if ok && obj.Type == script.ObjContract {
		return &Contract{Name: name, Block: obj.Value.(*script.Block)}
	}
	return nil
}

func VMObjectExists(vm *script.VM, name string, state uint32) bool {
	name = script.StateName(state, name)
	_, ok := vm.Objects[name]
	return ok
}

func vmGetUsedContracts(vm *script.VM, name string, state uint32, full bool) []string {
	contract := VMGetContract(vm, name, state)
	if contract == nil || contract.Block.Info.(*script.ContractInfo).Used == nil {
		return nil
	}
	ret := make([]string, 0)
	used := make(map[string]bool)
	for key := range contract.Block.Info.(*script.ContractInfo).Used {
		ret = append(ret, key)
		used[key] = true
		if full {
			sub := vmGetUsedContracts(vm, key, state, full)
			for _, item := range sub {
				if _, ok := used[item]; !ok {
					ret = append(ret, item)
					used[item] = true
				}
			}
		}
	}
	return ret
}

func VMGetContractByID(vm *script.VM, id int32) *Contract {
	var tableID int64
	if id > consts.ShiftContractID {
		tableID = int64(id - consts.ShiftContractID)
		id = int32(tableID + vm.ShiftContract)
	}
	idcont := id
	if len(vm.Children) <= int(idcont) {
		return nil
	}
	if vm.Children[idcont] == nil || vm.Children[idcont].Type != script.ObjContract {
		return nil
	}
	if tableID > 0 && vm.Children[idcont].Info.(*script.ContractInfo).Owner.TableID != tableID {
		return nil
	}
	return &Contract{Name: vm.Children[idcont].Info.(*script.ContractInfo).Name,
		Block: vm.Children[idcont]}
}

func vmExtendCost(vm *script.VM, ext func(string) int64) {
	vm.ExtCost = ext
}

func vmFuncCallsDB(vm *script.VM, funcCallsDB map[string]struct{}) {
	vm.FuncCallsDB = funcCallsDB
}

// ExternOff switches off the extern compiling mode in smartVM
func ExternOff() {
	vmExternOff(smartVM)
}

// Compile compiles contract source code in smartVM
func Compile(src string, owner *script.OwnerInfo) error {
	return vmCompile(smartVM, src, owner)
}

// CompileBlock calls CompileBlock for smartVM
func CompileBlock(src string, owner *script.OwnerInfo) (*script.Block, error) {
	return VMCompileBlock(smartVM, src, owner)
}

// CompileEval calls CompileEval for smartVM
func CompileEval(src string, prefix uint32) error {
	return VMCompileEval(smartVM, src, prefix)
}

// EvalIf calls EvalIf for smartVM
func EvalIf(src string, state uint32, extend *map[string]interface{}) (bool, error) {
	return VMEvalIf(smartVM, src, state, extend)
}

// FlushBlock calls FlushBlock for smartVM
func FlushBlock(root *script.Block) {
	VMFlushBlock(smartVM, root)
}

// ExtendCost sets the cost of calling extended obj in smartVM
func ExtendCost(ext func(string) int64) {
	vmExtendCost(smartVM, ext)
}

func FuncCallsDB(funcCallsDB map[string]struct{}) {
	vmFuncCallsDB(smartVM, funcCallsDB)
}

// Extend set extended variable and functions in smartVM
func Extend(ext *script.ExtendData) {
	vmExtend(smartVM, ext)
}

// Run executes Block in smartVM
func Run(block *script.Block, params []interface{}, extend *map[string]interface{}) (ret []interface{}, err error) {
	return ret, err
	//return VMRun(smartVM, block, params, extend)
}

// ActivateContract sets Active status of the contract in smartVM
func ActivateContract(tblid, state int64, active bool) {
	for i, item := range smartVM.Block.Children {
		if item != nil && item.Type == script.ObjContract {
			cinfo := item.Info.(*script.ContractInfo)
			if cinfo.Owner.TableID == tblid && cinfo.Owner.StateID == uint32(state) {
				smartVM.Children[i].Info.(*script.ContractInfo).Owner.Active = active
			}
		}
	}
}

// SetContractWallet changes WalletID of the contract in smartVM
func SetContractWallet(sc *SmartContract, tblid, state int64, wallet int64) error {
	if err := validateAccess(sc, "SetContractWallet"); err != nil {
		return err
	}
	for i, item := range smartVM.Block.Children {
		if item != nil && item.Type == script.ObjContract {
			cinfo := item.Info.(*script.ContractInfo)
			if cinfo.Owner.TableID == tblid && cinfo.Owner.StateID == uint32(state) {
				smartVM.Children[i].Info.(*script.ContractInfo).Owner.WalletID = wallet
			}
		}
	}
	return nil
}

// GetContract returns true if the contract exists in smartVM
func GetContract(name string, state uint32) *Contract {
	return VMGetContract(smartVM, name, state)
}

// GetUsedContracts returns the list of contracts which are called from the specified contract
func GetUsedContracts(name string, state uint32, full bool) []string {
	return vmGetUsedContracts(smartVM, name, state, full)
}

// GetContractByID returns true if the contract exists
func GetContractByID(id int32) *Contract {
	return VMGetContractByID(smartVM, id)
}

// GetFunc returns the block of the specified function in the contract
func (contract *Contract) GetFunc(name string) *script.Block {
	if block, ok := (*contract).Block.Objects[name]; ok && block.Type == script.ObjFunc {
		return block.Value.(*script.Block)
	}
	return nil
}

func loadContractList(list []model.Contract) error {
	if smartVM.ShiftContract == 0 {
		LoadSysFuncs(smartVM, 1)
		smartVM.ShiftContract = int64(len(smartVM.Children) - 1)
	}

	for _, item := range list {
		clist, err := script.ContractsList(item.Value)
		if err != nil {
			return err
		}
		owner := script.OwnerInfo{
			StateID:  uint32(item.EcosystemID),
			Active:   false,
			TableID:  item.ID,
			WalletID: item.WalletID,
			TokenID:  item.TokenID,
		}
		if err = Compile(item.Value, &owner); err != nil {
			logErrorValue(err, consts.EvalError, "Load Contract", strings.Join(clist, `,`))
		}
	}
	return nil
}

func defineVMType() script.VMType {

	if conf.Config.IsOBS() {
		return script.VMTypeOBS
	}

	if conf.Config.IsOBSMaster() {
		return script.VMTypeOBSMaster
	}

	return script.VMTypeSmart
}

// LoadContracts reads and compiles contracts from smart_contracts tables
func LoadContracts() error {
	contract := &model.Contract{}
	count, err := contract.Count()
	if err != nil {
		return logErrorDB(err, "getting count of contracts")
	}

	defer ExternOff()
	var offset int
	listCount := consts.ContractList
	for ; int64(offset) < count; offset += listCount {
		list, err := contract.GetList(offset, listCount)
		if err != nil {
			return logErrorDB(err, "getting list of contracts")
		}
		if err = loadContractList(list); err != nil {
			return err
		}
	}
	return nil
}

func LoadSysFuncs(vm *script.VM, state int) error {
	code := `func DBFind(table string).Columns(columns string).Where(where map)
	.WhereId(id int).Order(order string).Limit(limit int).Offset(offset int).All(all bool) array {
   return DBSelect(table, columns, id, order, offset, limit, where, all)
}

func One(list array, name string) string {
   if list {
	   var row map 
	   row = list[0]
	   if Contains(name, "->") {
		   var colfield array
		   var val string
		   colfield = Split(ToLower(name), "->")
		   val = row[Join(colfield, ".")]
		   if !val && row[colfield[0]] {
			   var fields map
			   var i int
			   fields = JSONToMap(row[colfield[0]])
			   val = fields[colfield[1]]
			   i = 2
			   while i < Len(colfield) {
					if GetType(val) == "map[string]interface {}" {
						val = val[colfield[i]]
						if !val {
							break
						}
					  	i= i+1
				   	} else {
						break
				   	}
			   }
		   }
		   if !val {
			   return ""
		   }
		   return val
	   }
	   return row[name]
   }
   return nil
}

func Row(list array) map {
   var ret map
   if list {
	   ret = list[0]
   }
   return ret
}

func DBRow(table string).Columns(columns string).Where(where map)
   .WhereId(id int).Order(order string) map {
   
   var result array
   result = DBFind(table).Columns(columns).Where(where).WhereId(id).Order(order)

   var row map
   if Len(result) > 0 {
	   row = result[0]
   }

   return row
}

func ConditionById(table string, validate bool) {
   var row map
   row = DBRow(table).Columns("conditions").WhereId($Id)
   if !row["conditions"] {
	   error Sprintf("Item %d has not been found", $Id)
   }

   Eval(row["conditions"])

   if validate {
	   ValidateCondition($Conditions,$ecosystem_id)
   }
}

func CurrentKeyFromAccount(account string) int {
	var row map
	row = DBRow("@1keys").Columns("id").Where({"account": account, "deleted": 0})
	if row {
		return row["id"]
	}
	return 0
}`
	return vmCompile(vm, code, &script.OwnerInfo{StateID: uint32(state)})
}

// LoadContract reads and compiles contract of new state
func LoadContract(transaction *model.DbTransaction, ecosystem int64) (err error) {

	contract := &model.Contract{}

	defer ExternOff()
	list, err := contract.GetFromEcosystem(transaction, ecosystem)
	if err != nil {
		return logErrorDB(err, "selecting all contracts from ecosystem")
	}
	if err = loadContractList(list); err != nil {
		return err
	}
	return
}

func (sc *SmartContract) getExtend() *map[string]interface{} {
	var block, blockTime, blockKeyID, blockNodePosition int64
	var perBlockHash string
	if sc.BlockData != nil {
		block = sc.BlockData.BlockID
		blockKeyID = sc.BlockData.KeyID
		blockTime = sc.BlockData.Time
		blockNodePosition = sc.BlockData.NodePosition
	}
	if sc.PreBlockData != nil {
		perBlockHash = hex.EncodeToString(sc.PreBlockData.Hash)
	}
	head := sc.TxSmart
	extend := map[string]interface{}{
		`type`:                head.ID,
		`time`:                head.Time,
		`ecosystem_id`:        head.EcosystemID,
		`node_position`:       blockNodePosition,
		`block`:               block,
		`key_id`:              sc.Key.ID,
		`account_id`:          sc.Key.AccountID,
		`block_key_id`:        blockKeyID,
		`parent`:              ``,
		`txcost`:              sc.GetContractLimit(),
		`txhash`:              sc.TxHash,
		`result`:              ``,
		`sc`:                  sc,
		`contract`:            sc.TxContract,
		`block_time`:          blockTime,
		`original_contract`:   ``,
		`this_contract`:       ``,
		`guest_key`:           consts.GuestKey,
		`guest_account`:       consts.GuestAddress,
		`pre_block_data_hash`: perBlockHash,
		`gen_block`:           sc.GenBlock,
		`time_limit`:          sc.TimeLimit,
	}
	for key, val := range sc.TxData {
		extend[key] = val
	}

	return &extend
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
		tables := &model.Table{}
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

func (sc *SmartContract) AccessTablePerm(table, action string) (map[string]string, error) {
	var (
		err             error
		tablePermission map[string]string
	)
	logger := sc.GetLogger()
	isRead := action == `read`
	if GetTableName(sc, table) == `1_parameters` || GetTableName(sc, table) == `1_app_params` {
		if isRead || sc.TxSmart.KeyID == converter.StrToInt64(EcosysParam(sc, `founder_account`)) {
			return tablePermission, nil
		}
		logger.WithFields(log.Fields{"type": consts.AccessDenied}).Error("Access denied")
		return tablePermission, errAccessDenied
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
	tables := &model.Table{}
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
			logger.WithFields(log.Fields{"action": action, "permissions": tablePermission[action], "type": consts.EvalError}).Error("access denied")
			return tablePermission, errAccessDenied
		}
	}
	return tablePermission, nil
}

// AccessTable checks the access right to the table
func (sc *SmartContract) AccessTable(table, action string) error {
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
	if GetTableName(sc, table) == `1_parameters` || GetTableName(sc, table) == `1_app_params` {
		if update {
			if sc.TxSmart.KeyID == converter.StrToInt64(EcosysParam(sc, `founder_account`)) {
				return nil
			}
			log.WithFields(log.Fields{"txSmart.KeyID": sc.TxSmart.KeyID}).Error("ACCESS DENIED")
			return errAccessDenied
		}
		return nil
	}
	prefix, name := PrefixName(table)
	tables := &model.Table{}
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
						return errAccessDenied
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

func (sc *SmartContract) CheckAccess(tableName, columns string, ecosystem int64) (table string, perm map[string]string,
	cols string, err error) {
	var collist []string

	table = converter.ParseTable(tableName, ecosystem)
	collist, err = GetColumns(columns)
	if err != nil {
		return
	}
	if !syspar.IsPrivateBlockchain() {
		cols = PrepareColumns(collist)
		return
	}
	perm, err = sc.AccessTablePerm(table, `read`)
	if err != nil {
		return
	}
	if err = sc.AccessColumns(table, &collist, false); err != nil {
		return
	}
	cols = PrepareColumns(collist)
	return
}

// AccessRights checks the access right by executing the condition value
func (sc *SmartContract) AccessRights(condition string, iscondition bool) error {
	sp := &model.StateParameter{}
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
	return VMEvalIf(sc.VM, conditions, uint32(sc.TxSmart.EcosystemID), sc.getExtend())
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
		honorNodes := syspar.GetNodes()
		if !builtinContract[sc.TxContract.Name] {
			return 0, errDelayedContract
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

		if sc.TxContract.Name == NewUserContract && !isNode {
			return signedBy, nil
		}
		if !isNode {
			return 0, errDelayedContract
		}
	} else if len(public) > 0 && sc.TxSmart.KeyID != crypto.Address(public) {
		return 0, errDiffKeys
	}
	return signedBy, nil
}

// CallContract calls the contract functions according to the specified flags
func (sc *SmartContract) CallContract(point int) (string, error) {
	var (
		result string
		err    error
	)
	logger := sc.GetLogger()

	retError := func(err error) (string, error) {
		eText := err.Error()
		if !strings.HasPrefix(eText, `{`) && err != script.ErrVMTimeLimit {
			if throw, ok := err.(*ThrowError); ok {
				out, errThrow := json.Marshal(throw)
				if errThrow != nil {
					out = []byte(`{"type": "panic", "error": "marshalling throw"}`)
				}
				err = errors.New(string(out))
			} else {
				err = script.SetVMError(`panic`, eText)
			}
		}
		return ``, err
	}

	sc.Key = &model.Key{}
	if err = sc.checkTxSign(); err != nil {
		return retError(err)
	}

	needPayment := sc.needPayment()
	if needPayment {
		err = sc.prepareMultiPay()
		if err != nil {
			logger.WithFields(log.Fields{"error": err}).Error("prepare multi")
			return retError(err)
		}
	}

	sc.TxContract.Extend = sc.getExtend()
	if err = sc.AppendStack(sc.TxContract.Name); err != nil {
		logger.WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("loop in contract")
		return retError(err)
	}
	sc.VM = GetVM()

	ctrctExtend := *sc.TxContract.Extend
	before := ctrctExtend[`txcost`].(int64)
	txSizeFuel := syspar.GetSizeFuel() * sc.TxSize / 1024
	ctrctExtend[`txcost`] = ctrctExtend[`txcost`].(int64) - txSizeFuel

	_, nameContract := converter.ParseName(sc.TxContract.Name)
	ctrctExtend[`original_contract`] = nameContract
	ctrctExtend[`this_contract`] = nameContract

	methods := []string{`conditions`, `action`}
	var (
		estimate decimal.Decimal
		cfuncs   []*script.Block
	)
	for i := 0; i < len(methods); i++ {
		cfunc := sc.TxContract.GetFunc(methods[i])
		if cfunc == nil {
			continue
		}
		if needPayment {
			for _, pay := range sc.multiPays {
				wltAmount, _ := decimal.NewFromString(pay.payWallet.Amount)
				estimateCost := converter.StrToInt64(converter.IntToStr(len(cfunc.Vars) + len(cfunc.Code)))
				estimate = estimate.Add(decimal.New(estimateCost*2, 0).Mul(pay.fuelRate))
				if wltAmount.Cmp(estimate) < 0 {
					return retError(errCurrentBalance)
				}
			}

		}
		cfuncs = append(cfuncs, cfunc)
	}

	for i := 0; i < len(cfuncs); i++ {
		sc.TxContract.Called = 1 << i
		if _, err = VMRun(sc.VM, cfuncs[i], nil, sc.TxContract.Extend); err != nil {
			break
		}
	}
	sc.TxFuel = before - ctrctExtend[`txcost`].(int64)
	sc.TxUsedCost = decimal.New(sc.TxFuel, 0)

	if err == nil {
		if ctrctExtend[`result`] != nil {
			result = fmt.Sprint(ctrctExtend[`result`])
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
		if needPayment {
			if ierr := sc.DbTransaction.ResetSavepoint(consts.SetSavePointMarkBlock(point)); ierr != nil {
				return retError(ierr)
			}
			if ierr := sc.payContract(true); ierr != nil {
				if yerr := sc.DbTransaction.RollbackSavepoint(consts.SetSavePointMarkBlock(point)); yerr != nil {
					return retError(yerr)
				}
				return ierr.Error(), nil
			}
			return err.Error(), nil
		}
		return retError(err)
	}

	if needPayment {
		if ierr := sc.payContract(false); ierr != nil {
			err = ierr
			goto lp
		}
	}
	return result, nil
}

func (sc *SmartContract) checkTxSign() error {
	var public []byte
	if len(sc.TxSmart.PublicKey) > 0 && string(sc.TxSmart.PublicKey) != `null` {
		public = sc.TxSmart.PublicKey
	}
	signedBy, err := sc.GetSignedBy(public)
	if err != nil {
		return err
	}

	isFound, err := sc.Key.SetTablePrefix(sc.TxSmart.EcosystemID).Get(sc.DbTransaction, signedBy)
	if err != nil {
		sc.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting wallet")
		return err
	}

	if !isFound {
		err = fmt.Errorf(eEcoKeyNotFound, converter.AddressToString(signedBy), sc.TxSmart.EcosystemID)
		sc.GetLogger().WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("looking for keyid")
		return err
	}
	if sc.Key.Disable() {
		err = fmt.Errorf(eEcoKeyDisable, converter.AddressToString(signedBy), sc.TxSmart.EcosystemID)
		sc.GetLogger().WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("disable keyid")
		return err
	}
	if len(sc.Key.PublicKey) > 0 {
		public = sc.Key.PublicKey
	}
	if len(public) == 0 {
		sc.GetLogger().WithFields(log.Fields{"type": consts.EmptyObject}).Error("empty public key")
		return errEmptyPublicKey
	}
	sc.PublicKeys = append(sc.PublicKeys, public)

	var CheckSignResult bool

	CheckSignResult, err = utils.CheckSign(sc.PublicKeys, sc.TxHash, sc.TxSignature, false)
	if err != nil {
		sc.GetLogger().WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("checking tx data sign")
		return err
	}
	if !CheckSignResult {
		sc.GetLogger().WithFields(log.Fields{"type": consts.InvalidObject}).Error("incorrect sign")
		return errIncorrectSign
	}
	return nil
}
