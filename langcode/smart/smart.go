/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"ibax.io/store/utils"

	"ibax.io/store"

	"ibax.io/crypto"
	vm2 "ibax.io/vm"

	"ibax.io/common/consts"
	"ibax.io/common/converter"
	"ibax.io/conf"

	//"ibax.io/crypto/ecies"
	"ibax.io/store/model"

	//"encoding/base64"

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
	Block         *vm2.Block
}

func (c *Contract) Info() *vm2.ContractInfo {
	return c.Block.Info.(*vm2.ContractInfo)
}

const (
	// MaxPrice is a maximal value that price function can return
	MaxPrice = 100000000000000000

	CallDelayedContract = "@1CallDelayedContract"
	NewUserContract     = "@1NewUser"
	NewBadBlockContract = "@1NewBadBlock"
)

var (
	smartVM   *vm2.VM
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
	return log.WithFields(log.Fields{"obs": sc.OBS, "name": name})
}

func InitVM() {
	vm := GetVM()

	vmt := defineVMType()

	EmbedFuncs(vm, vmt)
}

func newVM() *vm2.VM {
	vm := vm2.NewVM()
	vm.Extern = true
	vm.Extend(&vm2.ExtendData{Objects: map[string]interface{}{
		"Println": fmt.Println,
		"Sprintf": fmt.Sprintf,
		"Float":   Float,
		"Money":   vm2.ValueToDecimal,
		`Test`:    testValue,
	}})
	return vm
}

func init() {
	smartVM = newVM()
}

// GetVM is returning langcode vm
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

var SmartObjects map[string]*vm2.ObjInfo
var Chid uint32

func SavepointSmartVMObjects() {
	SmartObjects = make(map[string]*vm2.ObjInfo)
	for k, v := range smartVM.Objects {
		SmartObjects[k] = v
	}
	Chid = uint32(len(smartVM.Children))
}

func RollbackSmartVMObjects() {
	smartVM.Objects = make(map[string]*vm2.ObjInfo)
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

// GetVM is returning langcode vm
func GetVM() *vm2.VM {
	return smartVM
}

func vmExternOff(vm *vm2.VM) {
	vm.FlushExtern()
}

func vmCompile(vm *vm2.VM, src string, owner *vm2.OwnerInfo) error {
	return vm.Compile([]rune(src), owner)
}

// VMCompileBlock is compiling block
func VMCompileBlock(vm *vm2.VM, src string, owner *vm2.OwnerInfo) (*vm2.Block, error) {
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

func VMCompileEval(vm *vm2.VM, src string, prefix uint32) error {
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
	re := regexp.MustCompile(`^@?[\d\w\_]+$`)
	for _, item := range getContractList(src) {
		if len(item) == 0 || !re.Match([]byte(item)) {
			return errIncorrectParameter
		}
	}
	return nil
}

func VMEvalIf(vm *vm2.VM, src string, state uint32, extend *map[string]interface{}) (bool, error) {
	return vm.EvalIf(src, state, extend)
}

func VMFlushBlock(vm *vm2.VM, root *vm2.Block) {
	vm.FlushBlock(root)
}

func vmExtend(vm *vm2.VM, ext *vm2.ExtendData) {
	vm.Extend(ext)
}

func VMRun(vm *vm2.VM, block *vm2.Block, params []interface{}, extend *map[string]interface{}) (ret []interface{}, err error) {
	var cost int64
	if ecost, ok := (*extend)[`txcost`]; ok {
		cost = ecost.(int64)
	} else {
		cost = store.GetMaxCost()
	}
	rt := vm.RunInit(cost)
	ret, err = rt.Run(block, params, extend)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.VMError, "error": err, "original_contract": (*extend)[`original_contract`], "this_contract": (*extend)[`this_contract`], "ecosystem_id": (*extend)[`ecosystem_id`]}).Error("running block in langcode vm")
	}
	(*extend)[`txcost`] = rt.Cost()
	return
}

func VMGetContract(vm *vm2.VM, name string, state uint32) *Contract {
	if len(name) == 0 {
		return nil
	}
	name = vm2.StateName(state, name)
	obj, ok := vm.Objects[name]

	if ok && obj.Type == vm2.ObjContract {
		return &Contract{Name: name, Block: obj.Value.(*vm2.Block)}
	}
	return nil
}

func VMObjectExists(vm *vm2.VM, name string, state uint32) bool {
	name = vm2.StateName(state, name)
	_, ok := vm.Objects[name]
	return ok
}

func vmGetUsedContracts(vm *vm2.VM, name string, state uint32, full bool) []string {
	contract := VMGetContract(vm, name, state)
	if contract == nil || contract.Block.Info.(*vm2.ContractInfo).Used == nil {
		return nil
	}
	ret := make([]string, 0)
	used := make(map[string]bool)
	for key := range contract.Block.Info.(*vm2.ContractInfo).Used {
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

func VMGetContractByID(vm *vm2.VM, id int32) *Contract {
	var tableID int64
	if id > consts.ShiftContractID {
		tableID = int64(id - consts.ShiftContractID)
		id = int32(tableID + vm.ShiftContract)
	}
	idcont := id
	if len(vm.Children) <= int(idcont) {
		return nil
	}
	if vm.Children[idcont] == nil || vm.Children[idcont].Type != vm2.ObjContract {
		return nil
	}
	if tableID > 0 && vm.Children[idcont].Info.(*vm2.ContractInfo).Owner.TableID != tableID {
		return nil
	}
	return &Contract{Name: vm.Children[idcont].Info.(*vm2.ContractInfo).Name,
		Block: vm.Children[idcont]}
}

func vmExtendCost(vm *vm2.VM, ext func(string) int64) {
	vm.ExtCost = ext
}

func vmFuncCallsDB(vm *vm2.VM, funcCallsDB map[string]struct{}) {
	vm.FuncCallsDB = funcCallsDB
}

// ExternOff switches off the extern compiling mode in smartVM
func ExternOff() {
	vmExternOff(smartVM)
}

// Compile compiles contract source code in smartVM
func Compile(src string, owner *vm2.OwnerInfo) error {
	return vmCompile(smartVM, src, owner)
}

// CompileBlock calls CompileBlock for smartVM
func CompileBlock(src string, owner *vm2.OwnerInfo) (*vm2.Block, error) {
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
func FlushBlock(root *vm2.Block) {
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
func Extend(ext *vm2.ExtendData) {
	vmExtend(smartVM, ext)
}

// Run executes Block in smartVM
func Run(block *vm2.Block, params []interface{}, extend *map[string]interface{}) (ret []interface{}, err error) {
	return VMRun(smartVM, block, params, extend)
}

// ActivateContract sets Active status of the contract in smartVM
func ActivateContract(tblid, state int64, active bool) {
	for i, item := range smartVM.Block.Children {
		if item != nil && item.Type == vm2.ObjContract {
			cinfo := item.Info.(*vm2.ContractInfo)
			if cinfo.Owner.TableID == tblid && cinfo.Owner.StateID == uint32(state) {
				smartVM.Children[i].Info.(*vm2.ContractInfo).Owner.Active = active
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
		if item != nil && item.Type == vm2.ObjContract {
			cinfo := item.Info.(*vm2.ContractInfo)
			if cinfo.Owner.TableID == tblid && cinfo.Owner.StateID == uint32(state) {
				smartVM.Children[i].Info.(*vm2.ContractInfo).Owner.WalletID = wallet
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
func (contract *Contract) GetFunc(name string) *vm2.Block {
	if block, ok := (*contract).Block.Objects[name]; ok && block.Type == vm2.ObjFunc {
		return block.Value.(*vm2.Block)
	}
	return nil
}

func loadContractList(list []model.Contract) error {
	if smartVM.ShiftContract == 0 {
		LoadSysFuncs(smartVM, 1)
		smartVM.ShiftContract = int64(len(smartVM.Children) - 1)
	}

	for _, item := range list {
		clist, err := vm2.ContractsList(item.Value)
		if err != nil {
			return err
		}
		owner := vm2.OwnerInfo{
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

func defineVMType() vm2.VMType {

	if conf.Config.IsOBS() {
		return vm2.VMTypeOBS
	}

	if conf.Config.IsOBSMaster() {
		return vm2.VMTypeOBSMaster
	}

	return vm2.VMTypeSmart
}

// LoadContracts reads and compiles contracts from smart_contracts tables
func LoadContracts() error {
	contract := &model.Contract{}
	count, err := contract.Count()
	if err != nil {
		return logErrorDB(err, "getting count of contracts")
	}

	defer ExternOff()
	var offset int64
	listCount := int64(consts.ContractList)
	for ; offset < count; offset += listCount {
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

func LoadSysFuncs(vm *vm2.VM, state int) error {
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
	return vmCompile(vm, code, &vm2.OwnerInfo{StateID: uint32(state)})
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
	if sc.BlockData != nil {
		block = sc.BlockData.BlockID
		blockKeyID = sc.BlockData.KeyID
		blockTime = sc.BlockData.Time
		blockNodePosition = sc.BlockData.NodePosition
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
		`pre_block_data_hash`: hex.EncodeToString(sc.PreBlockData.Hash),
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

// AccessTable checks the access right to the table
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
	if !store.IsPrivateBlockchain() {
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
		sc.TxCost = store.GetMaxCost()
	}
	return sc.TxCost
}

func (sc *SmartContract) payContract(fuelRate decimal.Decimal, payWallet *model.Key,
	toID int64, errNeedPay bool) error {
	logger := sc.GetLogger()
	placeholder := `Commission for execution of %s contract`
	comment := fmt.Sprintf(placeholder, sc.TxContract.Name)

	money := sc.TxUsedCost.Mul(fuelRate).Add(sc.TXBlockFuel)
	if !errNeedPay {
		money = money.Add(sc.NewElementPricesFuel)
	} else {
		comment = "(error)" + comment
		ts := model.TransactionStatus{}
		if err := ts.UpdatePenalty(sc.DbTransaction, sc.TxHash); err != nil {
			return err
		}
	}
	wltAmount, ierr := decimal.NewFromString(payWallet.Amount)
	if ierr != nil {
		logger.WithFields(log.Fields{"type": consts.ConversionError, "error": ierr, "value": payWallet.Amount}).Error("converting pay wallet amount from string to decimal")
		return ierr
	}

	if len(sc.TxSmart.PayOver) > 0 {
		payover, err := decimal.NewFromString(sc.TxSmart.PayOver)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": sc.TxSmart.TokenEcosystem}).Error("converting tx langcode pay over from string to decimal")
			return err
		}
		if payover.LessThan(decimal.New(0, 0)) {
			return fmt.Errorf(eGreaterThan, sc.TxSmart.PayOver)
		}
		money = money.Add(StringToAmount(sc.TxSmart.PayOver))
	}
	if len(sc.TxSmart.Expedite) > 0 {
		expedite, err := decimal.NewFromString(sc.TxSmart.Expedite)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": sc.TxSmart.TokenEcosystem}).Error("converting tx langcode expedite from string to decimal")
			return err
		}
		if expedite.LessThan(decimal.New(0, 0)) {
			return fmt.Errorf(eGreaterThan, sc.TxSmart.Expedite)
		}
		money = money.Add(StringToAmount(sc.TxSmart.Expedite))
	}
	if wltAmount.Cmp(money) < 0 {
		money = wltAmount
	}

	commission := money.Mul(decimal.New(store.SysInt64(store.CommissionSize), 0)).Div(decimal.New(100, 0)).Floor()
	walletTable := model.KeyTableName(sc.TxSmart.TokenEcosystem)
	fromIDString := converter.Int64ToStr(payWallet.ID)

	balance := func(db *model.DbTransaction, fid, tid int64, a decimal.Decimal) (fb, tb decimal.Decimal, err error) {
		if fid == tid {
			toKey := &model.Key{}
			toKey.SetTablePrefix(sc.TxSmart.TokenEcosystem)
			_, _ = toKey.GetTr(db, tid)
			tb, err = decimal.NewFromString(toKey.Amount)
			if err != nil {
				return
			}
			fb = tb
			return
		}

		fromKey := &model.Key{}
		fromKey.SetTablePrefix(sc.TxSmart.TokenEcosystem)
		_, _ = fromKey.GetTr(db, fid)

		toKey := &model.Key{}
		toKey.SetTablePrefix(sc.TxSmart.TokenEcosystem)
		_, _ = toKey.GetTr(db, tid)

		fb, err = decimal.NewFromString(fromKey.Amount)
		if err != nil {
			return
		}
		tb, err = decimal.NewFromString(toKey.Amount)
		if err != nil {
			return
		}
		return
	}

	payCommission := func(toID string, sum decimal.Decimal, t int64) error {
		if _, _, err := sc.update(
			[]string{"+amount"}, []interface{}{sum}, walletTable, "id", toID); err != nil {
			return err
		}
		if _, _, err := sc.update([]string{`-amount`}, []interface{}{sum}, walletTable, `id`,
			fromIDString); err != nil {
			return errCommission
		}
		fromIDBalance, toIDBalance, err := balance(sc.DbTransaction, converter.StrToInt64(fromIDString), converter.StrToInt64(toID), sum)
		if err != nil {
			return err
		}
		_, _, err = sc.insert(
			[]string{
				"sender_id",
				"recipient_id",
				"sender_balance",
				"recipient_balance",
				"amount",
				"comment",
				"block_id",
				"txhash",
				"ecosystem",
				"type",
				"created_at",
			},
			[]interface{}{
				fromIDString,
				toID,
				fromIDBalance,
				toIDBalance,
				sum,
				comment,
				sc.BlockData.BlockID,
				sc.TxHash,
				sc.TxSmart.TokenEcosystem,
				t,
				sc.BlockData.Time,
			},
			`1_history`)

		if err != nil {
			return err
		}

		return nil
	}

	if err := payCommission(converter.Int64ToStr(toID), money.Sub(commission), 1); err != nil {
		if err != errUpdNotExistRecord {
			return err
		}
		return errUpdNotExistRecord
		//money = commission
	}

	if err := payCommission(store.GetCommissionWallet(sc.TxSmart.TokenEcosystem), commission, 2); err != nil {
		if err != errUpdNotExistRecord {
			return err
		}
		return errUpdNotExistRecord
		//money = money.Sub(commission)
	}
	return nil
}

func (sc *SmartContract) GetSignedBy(public []byte) (int64, error) {
	signedBy := sc.TxSmart.KeyID
	if sc.TxSmart.SignedBy != 0 {
		var isNode bool
		signedBy = sc.TxSmart.SignedBy
		fullNodes := store.GetNodes()
		if sc.TxContract.Name != CallDelayedContract && sc.TxContract.Name != NewUserContract &&
			sc.TxContract.Name != NewBadBlockContract {
			return 0, errDelayedContract
		}
		if len(fullNodes) > 0 {
			for _, node := range fullNodes {
				if crypto.Address(node.PublicKey) == signedBy {
					isNode = true
					break
				}
			}
		} else {
			isNode = crypto.Address(store.GetNodePubKey()) == signedBy
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
		result                 string
		err                    error
		public                 []byte
		sizeFuel, toID, fromID int64
		//
		isFound                    bool
		fuelRate, newElementPrices decimal.Decimal
	)
	logger := sc.GetLogger()
	payWallet := &model.Key{}

	sc.Key = &model.Key{}
	sc.TxSmart.TokenEcosystem = consts.TokenEcosystem

	retError := func(err error) (string, error) {
		eText := err.Error()
		if !strings.HasPrefix(eText, `{`) && err != vm2.ErrVMTimeLimit {
			if throw, ok := err.(*ThrowError); ok {
				out, errThrow := json.Marshal(throw)
				if errThrow != nil {
					out = []byte(`{"type": "panic", "error": "marshalling throw"}`)
				}
				err = errors.New(string(out))
			} else {
				err = vm2.SetVMError(`panic`, eText)
			}
		}
		return ``, err
	}

	if !sc.OBS {
		toID = sc.BlockData.KeyID
		fromID = sc.TxSmart.KeyID
	}
	if len(sc.TxSmart.PublicKey) > 0 && string(sc.TxSmart.PublicKey) != `null` {
		public = sc.TxSmart.PublicKey
	}

	sc.Key.SetTablePrefix(sc.TxSmart.EcosystemID)
	signedBy, err := sc.GetSignedBy(public)
	if err != nil {
		return retError(err)
	}

	//
	isFound, err = sc.Key.Get(sc.DbTransaction, signedBy)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting wallet")
		return retError(err)
	}

	if !isFound {
		err = fmt.Errorf(eKeyNotFound, converter.AddressToString(signedBy))
		logger.WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("looking for keyid")
		return retError(err)
	}
	if sc.Key.Deleted == 1 {
		return retError(errDeletedKey)
	}
	if len(sc.Key.PublicKey) > 0 {
		public = sc.Key.PublicKey
	}
	if len(public) == 0 {
		logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("empty public key")
		return retError(errEmptyPublicKey)
	}
	sc.PublicKeys = append(sc.PublicKeys, public)

	var CheckSignResult bool

	CheckSignResult, err = utils.CheckSign(sc.PublicKeys, sc.TxHash, sc.TxSignature, false)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("checking tx data sign")
		return retError(err)
	}
	if !CheckSignResult {
		logger.WithFields(log.Fields{"type": consts.InvalidObject}).Error("incorrect sign")
		return retError(errIncorrectSign)
	}

	methods := []string{`conditions`, `action`}
	sc.TxContract.Extend = sc.getExtend()
	if err = sc.AppendStack(sc.TxContract.Name); err != nil {
		logger.WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("loop in contract")
		return retError(err)
	}
	sc.VM = GetVM()

	needPayment := sc.TxSmart.EcosystemID > 0 && !sc.OBS && !store.IsPrivateBlockchain() && sc.payFreeContract()
	if needPayment {
		if sc.TxSmart.TokenEcosystem == 0 {
			sc.TxSmart.TokenEcosystem = 1
		}

		parTokenEcosysFuelRate := store.GetFuelRate(sc.TxSmart.TokenEcosystem)
		fuelRate, err = decimal.NewFromString(parTokenEcosysFuelRate)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": sc.TxSmart.TokenEcosystem}).Error("converting ecosystem fuel rate from string to decimal")
			return retError(err)
		}

		if fuelRate.Cmp(decimal.New(0, 0)) <= 0 {
			logger.WithFields(log.Fields{"type": consts.ParameterExceeded}).Error("Fuel rate must be greater than 0")
			return retError(errFuelRate)
		}

		cntrctOwnerInfo := sc.TxContract.Block.Info.(*vm2.ContractInfo).Owner

		if cntrctOwnerInfo.WalletID != 0 {
			fromID = cntrctOwnerInfo.WalletID
			sc.TxSmart.TokenEcosystem = cntrctOwnerInfo.TokenID
		} else if len(sc.TxSmart.PayOver) > 0 {
			_, err = decimal.NewFromString(sc.TxSmart.PayOver)
			if err != nil {
				logger.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": sc.TxSmart.TokenEcosystem}).Error("converting tx langcode pay over from string to decimal")
				return retError(err)
			}
			//fuelRate = fuelRate.Add(payOver)
		}

		var isEcosysWallet bool
		if sc.TxSmart.EcosystemID != sc.TxSmart.TokenEcosystem {
			ew := &model.StateParameter{}
			ew.SetTablePrefix(converter.Int64ToStr(sc.TxSmart.EcosystemID))
			if foundEcosystemWallet, err := ew.Get(sc.DbTransaction, "ecosystem_wallet"); err != nil {
				return retError(err)
			} else if foundEcosystemWallet && len(ew.Value) > 0 {
				ecosystemWallet := AddressToID(ew.Value)
				if ecosystemWallet != 0 {
					fromID = ecosystemWallet
					isEcosysWallet = true
				}
			}
		}

		payWallet.SetTablePrefix(sc.TxSmart.TokenEcosystem)
		if found, err := payWallet.Get(sc.DbTransaction, fromID); err != nil || !found {
			if !found {
				return retError(errKeyIDAccount)
			}

			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting wallet")
			return retError(err)
		}

		if cntrctOwnerInfo.WalletID == 0 && !isEcosysWallet &&
			!bytes.Equal(sc.Key.PublicKey, payWallet.PublicKey) &&
			!bytes.Equal(sc.TxSmart.PublicKey, payWallet.PublicKey) &&
			sc.TxSmart.SignedBy == 0 {
			return retError(errDiffKeys)
		}

		amount := decimal.New(0, 0)
		if len(payWallet.Amount) > 0 {
			amount, err = decimal.NewFromString(payWallet.Amount)
			if err != nil {
				logger.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": payWallet.Amount}).Error("converting pay wallet amount from string to decimal")
				return retError(err)
			}
		}

		maxpay := decimal.New(0, 0)
		if len(payWallet.Maxpay) > 0 {
			maxpay, err = decimal.NewFromString(payWallet.Maxpay)
			if err != nil {
				logger.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": payWallet.Maxpay}).Error("converting pay wallet maxpay from string to decimal")
				return retError(err)
			}
		}

		if maxpay.GreaterThan(decimal.New(0, 0)) && maxpay.LessThan(amount) {
			amount = maxpay
		}

		if sc.TxSmart.EcosystemID != 1 {
			sc.TXBlockFuel = decimal.New(store.SysInt64(store.PriceTxSizeWallet), 0).Mul(decimal.New(store.SysInt64(store.PriceCreateRate), 0)).Mul(fuelRate).Mul(decimal.New(sc.TxSize, 0)).Div(decimal.New(int64(consts.ChainSize), 0)).Floor()
			if sc.TXBlockFuel.LessThanOrEqual(decimal.New(0, 0)) {
				sc.TXBlockFuel = decimal.New(1, 0)
			}
		}
		if priceName, ok := vm2.ContractPrices[sc.TxContract.Name]; ok {
			newElementPricesInt := SysParamInt(priceName)
			newElementPricesStr, err := decimal.NewFromString(converter.Int64ToStr(newElementPricesInt))
			if err != nil {
				logger.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": sc.TxSmart.TokenEcosystem}).Error("converting new element price from string to decimal")
				return retError(err)
			}
			newElementPrices = newElementPricesStr.Mul(decimal.New(store.SysInt64(store.PriceCreateRate), 0)).Mul(fuelRate)
			if newElementPrices.GreaterThan(decimal.New(MaxPrice, 0)) {
				logger.WithFields(log.Fields{"type": consts.NoFunds}).Error("Price value is more than the highest value")
				return retError(errMaxPrice)
			}
			if newElementPrices.LessThan(decimal.New(0, 0)) {
				logger.WithFields(log.Fields{"type": consts.NoFunds}).Error("Price value is negative")
				return retError(errNegPrice)
			}
			sc.NewElementPricesFuel = newElementPrices
		}
		if len(sc.TxSmart.Expedite) > 0 {
			expedite, err := decimal.NewFromString(sc.TxSmart.Expedite)
			if err != nil {
				logger.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": sc.TxSmart.TokenEcosystem}).Error("converting tx langcode expedite from string to decimal")
				return retError(err)
			}
			if expedite.LessThan(decimal.New(0, 0)) {
				return retError(fmt.Errorf(eGreaterThan, sc.TxSmart.Expedite))
			}
			newElementPrices = newElementPrices.Add(StringToAmount(sc.TxSmart.Expedite))
		}
		sizeFuel = store.GetSizeFuel() * sc.TxSize / 1024
		if amount.LessThanOrEqual(newElementPrices.Add(sc.TXBlockFuel)) {
			logger.WithFields(log.Fields{"type": consts.NoFunds}).Error("current balance is not enough")
			return retError(errCurrentBalance)
		}
	}
	(*sc.TxContract.Extend)["gen_block"] = sc.GenBlock
	(*sc.TxContract.Extend)["time_limit"] = sc.TimeLimit
	ctrctExtend := *sc.TxContract.Extend
	before := ctrctExtend[`txcost`].(int64)

	// Payment for the size
	ctrctExtend[`txcost`] = ctrctExtend[`txcost`].(int64) - sizeFuel
	if ctrctExtend[`txcost`].(int64) <= 0 {
		logger.WithFields(log.Fields{"type": consts.NoFunds}).Error("current balance is not enough for payment")
		return retError(errCurrentBalance)
	}

	_, nameContract := converter.ParseName(sc.TxContract.Name)
	ctrctExtend[`original_contract`] = nameContract
	ctrctExtend[`this_contract`] = nameContract

	//Add sub node processing
	subNodeMode := 0 //1:"This is a sub node"; 0:"This is not a sub node"
	if conf.Config.IsSubNode() {
		subNodeMode = 1
	}

	privateDataFlag := 0
	addrMatchFlag := 0
	if (subNodeMode == 1) && (len(sc.TxSmart.Header.PrivateFor) > 0) && (sc.TxSmart.Header.PrivateFor[0] != "0") {
		fmt.Printf("subnode: This is a private transaction! And I'm a sub node\n")
		privateDataFlag = 1
		//_, NodePublicKey := utils.GetNodeKeys()
		//
		NodePrivateKey, NodePublicKey := utils.GetNodeKeys()
		if len(NodePrivateKey) < 1 {
			log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node private key is empty")
			err = errors.New(`empty node private key`)
			return retError(err)
		}
		for _, value := range sc.TxSmart.PrivateFor[1:] {
			NodePublicKeyItem := value[2:]
			if strings.EqualFold(NodePublicKeyItem, NodePublicKey) {
				fmt.Printf("subnode: Addr Matched: %v\n", NodePublicKey)
				addrMatchFlag = 1
				break
			}
		}
		if addrMatchFlag == 0 {
			fmt.Printf("subnode: Addr not matched! %v\n", NodePublicKey)
		}
	}

	if (addrMatchFlag == 1) && (sc.TxSmart.Header.PrivateFor[0] == "1") { //1 Hash Up to Chain
		privateDataHash, ok := ctrctExtend[`Data`]
		if ok == true {
			////
			var privateData model.SubNodeDestDataHash
			privateData, err := privateData.Get(privateDataHash.(string))
			if err != nil {
				logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting SubNodeDestData!")
				var m model.PrivatePackets
				m, err := m.Get(privateDataHash.(string))
				if err != nil {
					logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting private packet")
					fmt.Println("SubNode DataHash:", privateDataHash.(string))
					time.Sleep(time.Millisecond * 2)
					return retError(err)
				}
				ctrctExtend[`Data`] = string(m.Data)
			} else {
				ctrctExtend[`Data`] = string(privateData.Data)
			}
			//
			//var m model.PrivatePackets
			//m, err := m.Get(privateDataHash.(string))
			//if err != nil {
			//	logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting private packet")
			//	return retError(err)
			//}
			//ctrctExtend[`Data`] = string(m.Data)
		} else {
			fmt.Printf("subnode: The data does not need to be replaced!\n")
		}
	}

	if (addrMatchFlag == 1) && (sc.TxSmart.Header.PrivateFor[0] == "2") { //2 All Data Up to Chain
		//privateFile, ok := ctrctExtend[`Data`]
		//if ok == true {
		//	node_pri := store.GetNodePrivKey()
		//	privateFileStr := privateFile.(string)
		//	decodeBytes, err := base64.StdEncoding.DecodeString(privateFileStr)
		//	if err != nil {
		//		return retError(err)
		//	}
		//
		//	//privateFileStr := privateFile.(string)
		//	//privateFileByte, err :=  ecies.EccDeCrypto([]byte(privateFileStr), node_pri)
		//	privateFileByte, err := ecies.EccDeCrypto(decodeBytes, node_pri)
		//	if err != nil {
		//		logger.WithFields(log.Fields{"type": consts.CryptoError}).Error("Decryption error")
		//		return retError(err)
		//	}
		//
		//	ctrctExtend[`Data`] = string(privateFileByte)
		//} else {
		//	fmt.Printf("subnode: File was not updated!\n")
		//}
		//
		fmt.Printf("subnode: The data does not need to be replaced!\n")
	}
	//subnode End

	sc.TxContract.FreeRequest = false

	//Add sub node processing
	if (subNodeMode == 0) || (subNodeMode == 1 && sc.TxSmart.EcosystemID == 1) || (subNodeMode == 1 && privateDataFlag == 0) || (subNodeMode == 1 && addrMatchFlag == 1) {
		//fmt.Printf("subnode: Run VM!\n")
		if needPayment {
			var estimateCost int64
			var estimateAmount decimal.Decimal
			wltAmount, ierr := decimal.NewFromString(payWallet.Amount)
			if ierr != nil {
				logger.WithFields(log.Fields{"type": consts.ConversionError, "error": ierr, "value": payWallet.Amount}).Error("converting pay wallet amount from string to decimal")
				return retError(ierr)
			}
			for i := uint32(0); i < 2; i++ {
				cfunc := sc.TxContract.GetFunc(methods[i])
				if cfunc == nil {
					continue
				}
				estimateCost = converter.StrToInt64(converter.IntToStr(len(cfunc.Vars) + len(cfunc.Code)))
				estimateAmount = estimateAmount.Add(decimal.New(estimateCost*2, 0).Mul(fuelRate))
				if wltAmount.Cmp(estimateAmount) < 0 {
					return retError(errCurrentBalance)
				}
			}
		}
		for i := uint32(0); i < 2; i++ {
			cfunc := sc.TxContract.GetFunc(methods[i])
			if cfunc == nil {
				continue
			}

			sc.TxContract.Called = 1 << i
			if _, err = VMRun(sc.VM, cfunc, nil, sc.TxContract.Extend); err != nil {
				break
			}
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

	if err != nil {
		if needPayment {
			if ierr := sc.DbTransaction.RollbackSavepoint(point, consts.SavePointMarkBlock); ierr != nil {
				return retError(ierr)
			}
			if ierr := sc.DbTransaction.Savepoint(point, consts.SavePointMarkBlock); ierr != nil {
				return retError(ierr)
			}
			if ierr := sc.payContract(fuelRate, payWallet, toID, true); ierr != nil {
				if yerr := sc.DbTransaction.RollbackSavepoint(point, consts.SavePointMarkBlock); yerr != nil {
					return retError(yerr)
				}
				return retError(ierr)
			}
			return err.Error(), nil
		}
		return retError(err)
	}

	if needPayment {
		payWallet.SetTablePrefix(sc.TxSmart.TokenEcosystem)
		if found, err := payWallet.Get(sc.DbTransaction, fromID); err != nil || !found {
			if !found {
				return retError(errKeyIDAccount)
			}
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting wallet")
			return retError(err)
		}
		if ierr := sc.payContract(fuelRate, payWallet, toID, false); ierr != nil {
			return retError(ierr)
		}
	}
	return result, nil
}

func (sc *SmartContract) payFreeContract() bool {
	var (
		pfca  []string
		ispay bool
	)

	pfc := store.SysString(store.PayFreeContract)
	if len(pfc) > 0 {
		pfca = strings.Split(pfc, ",")
	}
	for _, value := range pfca {
		if strings.TrimSpace(value) == sc.TxContract.Name {
			ispay = true
			break
		}
	}
	return !ispay
}
