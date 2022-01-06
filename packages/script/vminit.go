/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"

	"github.com/IBAX-io/go-ibax/packages/consts"
	log "github.com/sirupsen/logrus"
)

const (
	// CostCall is the cost of the function calling
	CostCall = 50
	// CostContract is the cost of the contract calling
	CostContract = 100
	// CostExtend is the cost of the extend function calling
	CostExtend = 10

	TagFile      = "file"
	TagAddress   = "address"
	TagSignature = "signature"
	TagOptional  = "optional"
)

// ExtFuncInfo is the structure for the extrended function
type ExtFuncInfo struct {
	Name     string
	Params   []reflect.Type
	Results  []reflect.Type
	Auto     []string
	Variadic bool
	Func     interface{}
	CanWrite bool // If the function can update DB
}

// FieldInfo describes the field of the data structure
type FieldInfo struct {
	Name     string
	Type     reflect.Type
	Original uint32
	Tags     string
}

var ContractPrices = map[string]string{
	`@1NewTable`:       `price_create_table`,
	`@1NewContract`:    `price_create_contract`,
	`@1NewEcosystem`:   `price_create_ecosystem`,
	`@1NewMenu`:        `price_create_menu`,
	`@1NewPage`:        `price_create_page`,
	`@1NewColumn`:      `price_create_column`,
	`@1NewApplication`: `price_create_application`,
	`@1NewSnippet`:     `price_create_snippet`,
	`@1NewView`:        `price_create_view`,
	`@1NewToken`:       `price_create_token`,
	`@1NewAsset`:       `price_create_asset`,
	`@1NewLang`:        `price_create_lang`,
	`@1NewSection`:     `price_create_section`,
}

// ContainsTag returns whether the tag is contained in this field
func (fi *FieldInfo) ContainsTag(tag string) bool {
	return strings.Contains(fi.Tags, tag)
}

// ContractInfo contains the contract information
type ContractInfo struct {
	ID       uint32
	Name     string
	Owner    *OwnerInfo
	Used     map[string]bool // Called contracts
	Tx       *[]*FieldInfo
	Settings map[string]interface{}
	CanWrite bool // If the function can update DB
}

// FuncNameCmd for cmdFuncName
type FuncNameCmd struct {
	Name  string
	Count int
}

// FuncName is storing param of FuncName
type FuncName struct {
	Params   []reflect.Type
	Offset   []int
	Variadic bool
}

// FuncInfo contains the function information
type FuncInfo struct {
	Params   []reflect.Type
	Results  []reflect.Type
	Names    *map[string]FuncName
	Variadic bool
	ID       uint32
	CanWrite bool // If the function can update DB
}

// VarInfo contains the variable information
type VarInfo struct {
	Obj   *ObjInfo
	Owner *CodeBlock
}

// IndexInfo contains the information for SetIndex
type IndexInfo struct {
	VarOffset int
	Owner     *CodeBlock
	Extend    string
}

// VM is the main type of the virtual machine
type VM struct {
	*CodeBlock
	ExtCost       func(string) int64
	FuncCallsDB   map[string]struct{}
	Extern        bool  // extern mode of compilation
	ShiftContract int64 // id of the first contract
	logger        *log.Entry
}

// Stacker represents interface for working with call stack
type Stacker interface {
	AppendStack(fn string) error
	PopStack(fn string)
}

// NewVM creates a new virtual machine
func NewVM() *VM {
	vm := &VM{
		CodeBlock: NewCodeBlock(),
	}
	vm.logger = log.WithFields(log.Fields{"extern": vm.Extern, "vm_block_type": vm.CodeBlock.Type})
	return vm
}

func getNameByObj(obj *ObjInfo) (name string) {
	block := obj.Value.CodeBlock()
	for key, val := range block.Parent.Objects {
		if val == obj {
			name = key
			break
		}
	}
	return
}

// Call executes the name object with the specified params and extended variables and functions
func (vm *VM) Call(name string, params []interface{}, extend *map[string]interface{}) (ret []interface{}, err error) {
	var obj *ObjInfo
	if state, ok := (*extend)[`rt_state`]; ok {
		obj = vm.getObjByNameExt(name, state.(uint32))
	} else {
		obj = vm.getObjByName(name)
	}
	if obj == nil {
		vm.logger.WithFields(log.Fields{"type": consts.VMError, "vm_func_name": name}).Error("unknown function")
		return nil, fmt.Errorf(`unknown function %s`, name)
	}
	switch obj.Type {
	case ObjectType_Func:
		var cost int64
		if v, ok := (*extend)[`txcost`]; ok {
			cost = v.(int64)
		} else {
			cost = syspar.GetMaxCost()
		}
		rt := NewRunTime(vm, cost)
		ret, err = rt.Run(obj.Value.CodeBlock(), params, extend)
		(*extend)[`txcost`] = rt.Cost()
	case ObjectType_ExtFunc:
		finfo := obj.Value.ExtFuncInfo()
		foo := reflect.ValueOf(finfo.Func)
		var result []reflect.Value
		pars := make([]reflect.Value, len(finfo.Params))
		if finfo.Variadic {
			for i := 0; i < len(pars)-1; i++ {
				pars[i] = reflect.ValueOf(params[i])
			}
			pars[len(pars)-1] = reflect.ValueOf(params[len(pars)-1:])
			result = foo.CallSlice(pars)
		} else {
			for i := 0; i < len(pars); i++ {
				pars[i] = reflect.ValueOf(params[i])
			}
			result = foo.Call(pars)
		}
		for _, iret := range result {
			ret = append(ret, iret.Interface())
		}
	default:
		vm.logger.WithFields(log.Fields{"type": consts.VMError, "vm_func_name": name}).Error("unknown function")
		return nil, fmt.Errorf(`unknown function %s`, name)
	}
	return ret, err
}
