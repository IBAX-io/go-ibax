/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package script

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"

	log "github.com/sirupsen/logrus"
)

type GlobalVm struct {
	mu      sync.Mutex
	smartVM *VM
}

func init() {
	_vm = newVM()
}

var (
	_vm *GlobalVm
)

func newVM() *GlobalVm {
	vm := NewVM()
	return &GlobalVm{
		smartVM: vm,
	}
}

// GetVM is returning smart vm
func GetVM() *VM {
	_vm.mu.Lock()
	defer _vm.mu.Unlock()
	return _vm.smartVM
}

var smartObjects map[string]*ObjInfo
var children uint32

func SavepointSmartVMObjects() {
	smartObjects = make(map[string]*ObjInfo)
	for k, v := range GetVM().Objects {
		smartObjects[k] = v
	}
	children = uint32(len(GetVM().Children))
}

func RollbackSmartVMObjects() {
	GetVM().Objects = make(map[string]*ObjInfo)
	for k, v := range smartObjects {
		GetVM().Objects[k] = v
	}

	GetVM().Children = GetVM().Children[:children]
	smartObjects = nil
}

func ReleaseSmartVMObjects() {
	smartObjects = nil
	children = 0
}

func VMCompileEval(vm *VM, src string, prefix uint32) error {
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

// VMRun executes CodeBlock in vm
func VMRun(vm *VM, block *CodeBlock, params []any, extend map[string]any, hash []byte) (ret []any, err error) {
	if block == nil {
		return nil, fmt.Errorf(`code block is nil`)
	}
	var cost int64
	if ecost, ok := extend[Extend_txcost]; ok {
		cost = ecost.(int64)
	} else {
		cost = syspar.GetMaxCost()
	}
	rt := NewRunTime(vm, cost)
	if block.isParentContract() {
		rt.cost -= block.parentContractCost()
	}
	ret, err = rt.Run(block, params, extend)
	extend[Extend_txcost] = rt.Cost()
	if err != nil {
		vm.logger.WithFields(log.Fields{"type": consts.VMError, "tx_hash": fmt.Sprintf("%x", hash), "error": err, "original_contract": extend[Extend_original_contract], "this_contract": extend[Extend_this_contract], "ecosystem_id": extend[Extend_ecosystem_id]}).Error("running block in smart vm")
		return nil, err
	}
	return
}

func VMObjectExists(vm *VM, name string, state uint32) bool {
	name = StateName(state, name)
	_, ok := vm.Objects[name]
	return ok
}

// SetExtendCost sets the cost of calling extended obj in vm
func (vm *VM) SetExtendCost(ext func(string) int64) {
	vm.ExtCost = ext
}

// SetFuncCallsDB Set up functions that can edit the database in vm
func (vm *VM) SetFuncCallsDB(funcCallsDB map[string]struct{}) {
	vm.FuncCallsDB = funcCallsDB
}

func LoadSysFuncs(vm *VM, state int) error {
	code := `func DBFind(table string).Select(query string).Columns(columns string).Where(where map)
	.WhereId(id int).Order(order string).Limit(limit int).Offset(offset int).Group(group string).All(all bool) array {
   return DBSelect(table, columns, id, order, offset, limit, where, query, group, all)
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
			   fields = JSONDecode(row[colfield[0]])
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
	   return Str(row[name])
   }
   return ""
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
	return vm.Compile([]rune(code), &OwnerInfo{StateID: uint32(state)})
}
