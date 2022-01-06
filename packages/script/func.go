package script

import (
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
	"reflect"
	"strings"
)

// ExtendData is used for the definition of the extended functions and variables
type ExtendData struct {
	Objects    map[string]interface{}
	AutoPars   map[string]string
	WriteFuncs map[string]struct{}
}

func NewExtendData() *ExtendData {
	return &ExtendData{
		Objects: map[string]interface{}{
			"ExecContract": ExecContract,
			"CallContract": ExContract,
			"Settings":     GetSettings,
			"MemoryUsage":  MemoryUsage,
		},
		AutoPars: map[string]string{
			`*script.RunTime`: `rt`,
		},
		WriteFuncs: map[string]struct{}{"CallContract": {}}}
}

// ExecContract runs the name contract where txs contains the list of parameters and
// params are the values of parameters
func ExecContract(rt *RunTime, name, txs string, params ...interface{}) (interface{}, error) {

	contract, ok := rt.vm.Objects[name]
	if !ok {
		log.WithFields(log.Fields{"contract_name": name, "type": consts.ContractError}).Error("unknown contract")
		return nil, fmt.Errorf(eUnknownContract, name)
	}
	logger := log.WithFields(log.Fields{"contract_name": name, "type": consts.ContractError})
	cblock := contract.Value.CodeBlock()
	parnames := make(map[string]bool)
	pars := strings.Split(txs, `,`)
	if len(pars) != len(params) {
		logger.WithFields(log.Fields{"contract_params_len": len(pars), "contract_params_len_needed": len(params), "type": consts.ContractError}).Error("wrong contract parameters pars")
		return ``, errContractPars
	}
	for _, ipar := range pars {
		parnames[ipar] = true
	}
	if _, ok := (*rt.extend)[`loop_`+name]; ok {
		logger.WithFields(log.Fields{"type": consts.ContractError, "contract_name": name}).Error("there is loop in contract")
		return nil, fmt.Errorf(eContractLoop, name)
	}
	(*rt.extend)[`loop_`+name] = true
	defer delete(*rt.extend, `loop_`+name)

	prevExtend := make(map[string]interface{})
	for key, item := range *rt.extend {
		if isSysVar(key) {
			continue
		}
		prevExtend[key] = item
		delete(*rt.extend, key)
	}

	var isSignature bool
	if cblock.Info.ContractInfo().Tx != nil {
		for _, tx := range *cblock.Info.ContractInfo().Tx {
			if !parnames[tx.Name] {
				if !strings.Contains(tx.Tags, TagOptional) {
					logger.WithFields(log.Fields{"transaction_name": tx.Name, "type": consts.ContractError}).Error("transaction not defined")
					return ``, fmt.Errorf(eUndefinedParam, tx.Name)
				}
				(*rt.extend)[tx.Name] = reflect.New(tx.Type).Elem().Interface()
			}
			if tx.Name == `Signature` {
				isSignature = true
			}
		}
	}
	for i, ipar := range pars {
		(*rt.extend)[ipar] = params[i]
	}
	prevthis := (*rt.extend)[`this_contract`]
	_, nameContract := converter.ParseName(name)
	(*rt.extend)[`this_contract`] = nameContract

	prevparent := (*rt.extend)[`parent`]
	parent := ``
	for i := len(rt.blocks) - 1; i >= 0; i-- {
		if rt.blocks[i].Block.Type == ObjectType_Func && rt.blocks[i].Block.Parent != nil &&
			rt.blocks[i].Block.Parent.Type == ObjectType_Contract {
			parent = rt.blocks[i].Block.Parent.Info.ContractInfo().Name
			fid, fname := converter.ParseName(parent)
			cid, _ := converter.ParseName(name)
			if len(fname) > 0 {
				if fid == 0 {
					parent = `@` + fname
				} else if fid == cid {
					parent = fname
				}
			}
			break
		}
	}
	rt.cost -= CostContract
	if priceName, ok := ContractPrices[name]; ok {
		price := syspar.SysInt64(priceName)
		if price > 0 {
			rt.cost -= price
		}
		if rt.cost < 0 {
			rt.cost = 0
		}
	}

	var (
		stack Stacker
		err   error
	)
	if stack, ok = (*rt.extend)["sc"].(Stacker); ok {
		if err := stack.AppendStack(name); err != nil {
			return nil, err
		}
	}
	if (*rt.extend)[`sc`] != nil && isSignature {
		obj := rt.vm.Objects[`check_signature`]
		finfo := obj.Value.ExtFuncInfo()
		if err := finfo.Func.(func(*map[string]interface{}, string) error)(rt.extend, name); err != nil {
			logger.WithFields(log.Fields{"error": err, "func_name": finfo.Name, "type": consts.ContractError}).Error("executing exended function")
			return nil, err
		}
	}
	for _, method := range []string{`conditions`, `action`} {
		if block, ok := (*cblock).Objects[method]; ok && block.Type == ObjectType_Func {
			rtemp := NewRunTime(rt.vm, rt.cost)
			(*rt.extend)[`parent`] = parent
			_, err = rtemp.Run(block.Value.CodeBlock(), nil, rt.extend)
			rt.cost = rtemp.cost
			if err != nil {
				logger.WithFields(log.Fields{"error": err, "method_name": method, "type": consts.ContractError}).Error("executing contract method")
				break
			}
		}
	}
	if stack != nil {
		stack.PopStack(name)
	}
	if err != nil {
		return nil, err
	}
	(*rt.extend)[`parent`] = prevparent
	(*rt.extend)[`this_contract`] = prevthis

	result := (*rt.extend)[`result`]
	for key := range *rt.extend {
		if isSysVar(key) {
			continue
		}
		delete(*rt.extend, key)
	}

	for key, item := range prevExtend {
		(*rt.extend)[key] = item
	}

	return result, nil
}

// ExContract executes the name contract in the state with specified parameters
func ExContract(rt *RunTime, state uint32, name string, params *types.Map) (interface{}, error) {

	name = StateName(state, name)
	contract, ok := rt.vm.Objects[name]
	if !ok {
		log.WithFields(log.Fields{"contract_name": name, "type": consts.ContractError}).Error("unknown contract")
		return nil, fmt.Errorf(eUnknownContract, name)
	}
	if params == nil {
		params = types.NewMap()
	}
	logger := log.WithFields(log.Fields{"contract_name": name, "type": consts.ContractError})
	names := make([]string, 0)
	vals := make([]interface{}, 0)
	cblock := contract.Value.CodeBlock()
	if cblock.Info.ContractInfo().Tx != nil {
		for _, tx := range *cblock.Info.ContractInfo().Tx {
			val, ok := params.Get(tx.Name)
			if !ok {
				if !strings.Contains(tx.Tags, TagOptional) {
					logger.WithFields(log.Fields{"transaction_name": tx.Name, "type": consts.ContractError}).Error("transaction not defined")
					return nil, fmt.Errorf(eUndefinedParam, tx.Name)
				}
				val = reflect.New(tx.Type).Elem().Interface()
			}
			names = append(names, tx.Name)
			vals = append(vals, val)
		}
	}
	if len(vals) == 0 {
		vals = append(vals, ``)
	}
	return ExecContract(rt, name, strings.Join(names, `,`), vals...)
}

// GetSettings returns the value of the parameter
func GetSettings(rt *RunTime, cntname, name string) (interface{}, error) {
	contract, ok := rt.vm.Objects[cntname]
	if !ok {
		log.WithFields(log.Fields{"contract_name": name, "type": consts.ContractError}).Error("unknown contract")
		return nil, fmt.Errorf(`unknown contract %s`, cntname)
	}
	cblock := contract.Value.CodeBlock()
	if cblock.Info.ContractInfo().Settings != nil {
		if val, ok := cblock.Info.ContractInfo().Settings[name]; ok {
			return val, nil
		}
	}
	return ``, nil
}

func MemoryUsage(rt *RunTime) int64 {
	return rt.mem
}
