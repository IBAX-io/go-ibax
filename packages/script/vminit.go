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

// ExtFuncInfo is the structure for the extended function
type ExtFuncInfo struct {
	Name     string
	Params   []reflect.Type
	Results  []reflect.Type
	Auto     []string
	Variadic bool
	Func     any
	CanWrite bool // If the function can update DB
}

// FieldInfo describes the field of the data structure
type FieldInfo struct {
	Name     string
	Type     reflect.Type
	Original uint32
	Tags     string
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
	Settings map[string]any
	CanWrite bool // If the function can update DB
}

func (c *ContractInfo) TxMap() map[string]*FieldInfo {
	if c == nil {
		return nil
	}
	var m = make(map[string]*FieldInfo)
	for _, n := range *c.Tx {
		m[n.Name] = nil
	}
	return m
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
	Name    string
	Params  []reflect.Type
	Results []reflect.Type
	//tail function
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
		Extern:    true,
	}
	vm.logger = log.WithFields(log.Fields{"type": consts.VMError, "extern": vm.Extern, "vm_block_type": vm.CodeBlock.Type})
	return vm
}

func getNameByObj(obj *ObjInfo) (name string) {
	block := obj.GetCodeBlock()
	for key, val := range block.Parent.Objects {
		if val == obj {
			name = key
			break
		}
	}
	return
}

// Call executes the name object with the specified params and extended variables and functions
func (vm *VM) Call(name string, params []any, extend map[string]any) (ret []any, err error) {
	var obj *ObjInfo
	if state, ok := extend[Extend_rt_state]; ok {
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
		if v, ok := extend[Extend_txcost]; ok {
			cost = v.(int64)
		} else {
			cost = syspar.GetMaxCost()
		}
		rt := NewRunTime(vm, cost)
		ret, err = rt.Run(obj.GetCodeBlock(), params, extend)
		extend[Extend_txcost] = rt.Cost()
	case ObjectType_ExtFunc:
		finfo := obj.GetExtFuncInfo()
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
