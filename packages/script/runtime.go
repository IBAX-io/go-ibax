/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

const (
	statusNormal = iota
	statusReturn
	statusContinue
	statusBreak

	// Decimal is the constant string for decimal type
	Decimal = `decimal.Decimal`
	// Interface is the constant string for interface type
	Interface = `interface`
	File      = `*types.Map`

	brackets = `[]`

	maxArrayIndex = 1000000
	maxMapCount   = 100000
	maxCallDepth  = 1000
	memoryLimit   = 128 << 20 // 128 MB
	MaxErrLen     = 150
)

var sysVars = map[string]struct{}{
	`block`:               {},
	`block_key_id`:        {},
	`block_time`:          {},
	`data`:                {},
	`ecosystem_id`:        {},
	`key_id`:              {},
	`account_id`:          {},
	`node_position`:       {},
	`parent`:              {},
	`original_contract`:   {},
	`sc`:                  {},
	`contract`:            {},
	`stack`:               {},
	`this_contract`:       {},
	`time`:                {},
	`type`:                {},
	`txcost`:              {},
	`txhash`:              {},
	`guest_key`:           {},
	`gen_block`:           {},
	`time_limit`:          {},
	`pre_block_data_hash`: {},
}

var (
	ErrMemoryLimit = errors.New("Memory limit exceeded")
	ErrVMTimeLimit = errors.New(`time limit exceeded`)
)

// VMError represents error of VM
type VMError struct {
	Type  string `json:"type"`
	Error string `json:"error"`
}

type blockStack struct {
	Block  *CodeBlock
	Offset int
}

// ErrInfo stores info about current contract or function
type ErrInfo struct {
	Name string
	Line uint16
}

// RunTime is needed for the execution of the byte-code
type RunTime struct {
	stack     []interface{}
	blocks    []*blockStack
	vars      []interface{}
	extend    *map[string]interface{}
	vm        *VM
	cost      int64
	err       error
	unwrap    bool
	timeLimit bool
	callDepth uint16
	mem       int64
	memVars   map[interface{}]int64
	errInfo   ErrInfo
}

// NewRunTime creates a new RunTime for the virtual machine
func NewRunTime(vm *VM, cost int64) *RunTime {
	return &RunTime{
		stack:   make([]interface{}, 0, 1024),
		vm:      vm,
		cost:    cost,
		memVars: make(map[interface{}]int64),
	}
}

func isSysVar(name string) bool {
	if _, ok := sysVars[name]; ok || strings.HasPrefix(name, `loop_`) {
		return true
	}
	return false
}

func (rt *RunTime) callFunc(cmd uint16, obj *ObjInfo) (err error) {
	var (
		count, in int
	)
	if rt.callDepth >= maxCallDepth {
		return fmt.Errorf("max call depth")
	}

	rt.callDepth++
	defer func() {
		rt.callDepth--
	}()

	size := len(rt.stack)
	in = obj.getInParams()
	if rt.unwrap && cmd == cmdCallVari && size > 1 &&
		reflect.TypeOf(rt.stack[size-2]).String() == `[]interface {}` {
		count = rt.stack[size-1].(int)
		arr := rt.stack[size-2].([]interface{})
		rt.stack = rt.stack[:size-2]
		for _, item := range arr {
			rt.stack = append(rt.stack, item)
		}
		rt.stack = append(rt.stack, count-1+len(arr))
		size = len(rt.stack)
	}
	rt.unwrap = false
	if cmd == cmdCallVari {
		count = rt.stack[size-1].(int)
		size--
	} else {
		count = in
	}
	if obj.Type == ObjectType_Func {
		var imap map[string][]interface{}
		if obj.Value.CodeBlock().Info.FuncInfo().Names != nil {
			if rt.stack[size-1] != nil {
				imap = rt.stack[size-1].(map[string][]interface{})
			}
			rt.stack = rt.stack[:size-1]
		}
		if cmd == cmdCallVari {
			parcount := count + 1 - in
			if parcount < 0 {
				log.WithFields(log.Fields{"type": consts.VMError}).Error(errWrongCountPars.Error())
				return errWrongCountPars
			}
			pars := make([]interface{}, parcount)
			shift := size - parcount
			for i := parcount; i > 0; i-- {
				pars[i-1] = rt.stack[size+i-parcount-1]
			}
			rt.stack = rt.stack[:shift]
			rt.stack = append(rt.stack, pars)
		}
		finfo := obj.Value.CodeBlock().Info.FuncInfo()
		if len(rt.stack) < len(finfo.Params) {
			log.WithFields(log.Fields{"type": consts.VMError}).Error(errWrongCountPars.Error())
			return errWrongCountPars
		}
		for i, v := range finfo.Params {
			switch v.String() {
			case `string`, `int64`:
				if reflect.TypeOf(rt.stack[len(rt.stack)-in+i]) != v {
					log.WithFields(log.Fields{"type": consts.VMError}).Error(eTypeParam)
					return fmt.Errorf(eTypeParam, i+1)
				}
			}
		}
		if obj.Value.CodeBlock().Info.FuncInfo().Names != nil {
			rt.stack = append(rt.stack, imap)
		}
		_, err = rt.RunCode(obj.Value.CodeBlock())
	} else {
		finfo := obj.Value.ExtFuncInfo()
		foo := reflect.ValueOf(finfo.Func)
		var result []reflect.Value
		pars := make([]reflect.Value, in)
		limit := 0
		var (
			stack Stacker
			ok    bool
		)
		if stack, ok = (*rt.extend)["sc"].(Stacker); ok {
			if err := stack.AppendStack(finfo.Name); err != nil {
				return err
			}
		}
		(*rt.extend)[`rt`] = rt
		auto := 0
		for k := 0; k < in; k++ {
			if len(finfo.Auto[k]) > 0 {
				auto++
			}
		}
		shift := size - count + auto
		if finfo.Variadic {
			shift = size - count
			count += auto
			limit = count - in + 1
		}
		i := count
		for ; i > limit; i-- {
			if len(finfo.Auto[count-i]) > 0 {
				pars[count-i] = reflect.ValueOf((*rt.extend)[finfo.Auto[count-i]])
				auto--
			} else {
				pars[count-i] = reflect.ValueOf(rt.stack[size-i+auto])
			}
			if !pars[count-i].IsValid() {
				pars[count-i] = reflect.Zero(reflect.TypeOf(string(``)))
			}
		}
		if i > 0 {
			pars[in-1] = reflect.ValueOf(rt.stack[size-i : size])
		}
		if finfo.Name == `ExecContract` && (pars[2].Type().String() != `string` || !pars[3].IsValid()) {
			return fmt.Errorf(`unknown function %v`, pars[1])
		}
		if finfo.Variadic {
			result = foo.CallSlice(pars)
		} else {
			result = foo.Call(pars)
		}
		rt.stack = rt.stack[:shift]
		if stack != nil {
			stack.PopStack(finfo.Name)
		}

		for i, iret := range result {
			// first return value of every extend function that makes queries to DB is cost
			if i == 0 && rt.vm.FuncCallsDB != nil {
				if _, ok := rt.vm.FuncCallsDB[finfo.Name]; ok {
					cost := iret.Int()
					if cost > rt.cost {
						rt.cost = 0
						rt.vm.logger.WithFields(log.Fields{"type": consts.VMError}).Error("paid CPU resource is over")
						return fmt.Errorf("paid CPU resource is over")
					}

					rt.cost -= cost
					continue
				}
			}
			if finfo.Results[i].String() == `error` {
				if iret.Interface() != nil {
					rt.errInfo = ErrInfo{Name: finfo.Name}
					return iret.Interface().(error)
				}
			} else {
				rt.stack = append(rt.stack, iret.Interface())
			}
		}
	}
	return
}

func (rt *RunTime) extendFunc(name string) error {
	var (
		ok bool
		f  interface{}
	)
	if f, ok = (*rt.extend)[name]; !ok || reflect.ValueOf(f).Kind().String() != `func` {
		return fmt.Errorf(`unknown function %s`, name)
	}
	size := len(rt.stack)
	foo := reflect.ValueOf(f)

	count := foo.Type().NumIn()
	pars := make([]reflect.Value, count)
	for i := count; i > 0; i-- {
		pars[count-i] = reflect.ValueOf(rt.stack[size-i])
	}
	result := foo.Call(pars)

	rt.stack = rt.stack[:size-count]
	for i, iret := range result {
		if foo.Type().Out(i).String() == `error` {
			if iret.Interface() != nil {
				return iret.Interface().(error)
			}
		} else {
			rt.stack = append(rt.stack, iret.Interface())
		}
	}
	return nil
}

func calcMem(v interface{}) (mem int64) {
	rv := reflect.ValueOf(v)

	switch rv.Kind() {
	case reflect.Bool:
		mem = 1
	case reflect.Int8, reflect.Uint8:
		mem = 1
	case reflect.Int16, reflect.Uint16:
		mem = 2
	case reflect.Int32, reflect.Uint32:
		mem = 4
	case reflect.Int64, reflect.Uint64, reflect.Int, reflect.Uint:
		mem = 8
	case reflect.Float32:
		mem = 4
	case reflect.Float64:
		mem = 8
	case reflect.String:
		mem += int64(rv.Len())
	case reflect.Slice, reflect.Array:
		mem = 12
		for i := 0; i < rv.Len(); i++ {
			mem += calcMem(rv.Index(i).Interface())
		}
	case reflect.Map:
		mem = 4
		for _, k := range rv.MapKeys() {
			mem += calcMem(k.Interface())
			mem += calcMem(rv.MapIndex(k).Interface())
		}
	default:
		mem = int64(unsafe.Sizeof(v))
	}

	return
}

func (rt *RunTime) setExtendVar(k string, v interface{}) {
	(*rt.extend)[k] = v
	rt.recalcMemExtendVar(k)
}

func (rt *RunTime) recalcMemExtendVar(k string) {
	mem := calcMem((*rt.extend)[k])
	rt.mem += mem - rt.memVars[k]
	rt.memVars[k] = mem
}

func (rt *RunTime) addVar(v interface{}) {
	rt.vars = append(rt.vars, v)
	mem := calcMem(v)
	rt.memVars[len(rt.vars)-1] = mem
	rt.mem += mem
}

func (rt *RunTime) setVar(k int, v interface{}) {
	rt.vars[k] = v
	rt.recalcMemVar(k)
}

func (rt *RunTime) recalcMemVar(k int) {
	mem := calcMem(rt.vars[k])
	rt.mem += mem - rt.memVars[k]
	rt.memVars[k] = mem
}

func valueToBool(v interface{}) bool {
	switch val := v.(type) {
	case int:
		if val != 0 {
			return true
		}
	case int64:
		if val != 0 {
			return true
		}
	case float64:
		if val != 0.0 {
			return true
		}
	case bool:
		return val
	case string:
		return len(val) > 0
	case []uint8:
		return len(val) > 0
	case []interface{}:
		return val != nil && len(val) > 0
	case map[string]interface{}:
		return val != nil && len(val) > 0
	case map[string]string:
		return val != nil && len(val) > 0
	case *types.Map:
		return val != nil && val.Size() > 0
	default:
		dec, _ := decimal.NewFromString(fmt.Sprintf(`%v`, val))
		return dec.Cmp(decimal.New(0, 0)) != 0
	}
	return false
}

// ValueToFloat converts interface (string, float64 or int64) to float64
func ValueToFloat(v interface{}) (ret float64) {
	var err error
	switch val := v.(type) {
	case float64:
		ret = val
	case int64:
		ret = float64(val)
	case string:
		ret, err = strconv.ParseFloat(val, 64)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": val}).Error("converting value from string to float")
		}
	}
	return
}

// ValueToDecimal converts interface (string, float64, Decimal or int64) to Decimal
func ValueToDecimal(v interface{}) (ret decimal.Decimal, err error) {
	switch val := v.(type) {
	case float64:
		ret = decimal.NewFromFloat(val).Floor()
	case string:
		ret, err = decimal.NewFromString(val)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": val}).Error("converting value from string to decimal")
		} else {
			ret = ret.Floor()
		}
	case int64:
		ret = decimal.New(val, 0)
	default:
		ret = val.(decimal.Decimal)
	}
	return
}

// SetCost sets the max cost of the execution.
func (rt *RunTime) SetCost(cost int64) {
	rt.cost = cost
}

// Cost return the remain cost of the execution.
func (rt *RunTime) Cost() int64 {
	return rt.cost
}

// SetVMError sets error of VM
func SetVMError(eType string, eText interface{}) error {
	errText := fmt.Sprintf(`%v`, eText)
	if len(errText) > MaxErrLen {
		errText = errText[:MaxErrLen] + `...`
	}
	out, err := json.Marshal(&VMError{Type: eType, Error: errText})
	if err != nil {
		log.WithFields(log.Fields{"type": consts.JSONMarshallError, "error": err}).Error("marshalling VMError")
		out = []byte(`{"type": "panic", "error": "marshalling VMError"}`)
	}
	return fmt.Errorf(string(out))
}

func (rt *RunTime) getResultValue(item mapItem) (value interface{}, err error) {
	switch item.Type {
	case mapConst:
		value = item.Value
	case mapExtend:
		value = (*rt.extend)[item.Value.(string)]
	case mapVar:
		ivar := item.Value.(*VarInfo)
		var i int
		for i = len(rt.blocks) - 1; i >= 0; i-- {
			if ivar.Owner == rt.blocks[i].Block {
				value = rt.vars[rt.blocks[i].Offset+ivar.Obj.Value.Int()]
				break
			}
		}
		if i < 0 {
			err = fmt.Errorf(eWrongVar, ivar.Obj.Value)
		}
	case mapMap:
		value, err = rt.getResultMap(item.Value.(*types.Map))
	case mapArray:
		value, err = rt.getResultArray(item.Value.([]mapItem))
	}
	return
}

func (rt *RunTime) getResultArray(cmd []mapItem) ([]interface{}, error) {
	initArr := make([]interface{}, 0)
	for _, val := range cmd {
		value, err := rt.getResultValue(val)
		if err != nil {
			return nil, err
		}
		initArr = append(initArr, value)
	}
	return initArr, nil
}

func (rt *RunTime) getResultMap(cmd *types.Map) (*types.Map, error) {
	initMap := types.NewMap()
	for _, key := range cmd.Keys() {
		val, _ := cmd.Get(key)
		value, err := rt.getResultValue(val.(mapItem))
		if err != nil {
			return nil, err
		}
		initMap.Set(key, value)
	}
	return initMap, nil
}

func isSelfAssignment(dest, value interface{}) bool {
	if _, ok := value.([]interface{}); !ok {
		if _, ok = value.(*types.Map); !ok {
			return false
		}
	}
	if reflect.ValueOf(dest).Pointer() == reflect.ValueOf(value).Pointer() {
		return true
	}
	switch v := value.(type) {
	case []interface{}:
		for _, item := range v {

			if isSelfAssignment(dest, item) {
				return true
			}
		}
	case *types.Map:
		for _, item := range v.Values() {
			if isSelfAssignment(dest, item) {
				return true
			}
		}
	}
	return false
}

// RunCode executes CodeBlock
func (rt *RunTime) RunCode(block *CodeBlock) (status int, err error) {
	var cmd *ByteCode
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprintf(`%v`, r))
		}
		if err != nil && !strings.HasPrefix(err.Error(), `{`) {
			var curContract, line string
			if block.isParentContract() {
				stack := block.Parent.Info.ContractInfo()
				curContract = stack.Name
			}
			if stack, ok := (*rt.extend)["stack"].([]interface{}); ok {
				curContract = stack[len(stack)-1].(string)
			}

			line = "]"
			if cmd != nil {
				line = fmt.Sprintf(":%d]", cmd.Line)
			}

			if len(rt.errInfo.Name) > 0 && rt.errInfo.Name != `ExecContract` {
				err = fmt.Errorf("%s [%s %s%s", err, rt.errInfo.Name, curContract, line)
				rt.errInfo.Name = ``
			} else {
				out := err.Error()
				if strings.HasSuffix(out, `]`) {
					prev := strings.LastIndexByte(out, ' ')
					if strings.HasPrefix(out[prev+1:], curContract+`:`) {
						out = out[:prev+1]
					} else {
						out = out[:len(out)-1] + ` `
					}
				} else {
					out += ` [`
				}
				err = fmt.Errorf(`%s%s%s`, out, curContract, line)
			}
		}
	}()
	top := make([]interface{}, 8)
	rt.blocks = append(rt.blocks, &blockStack{Block: block, Offset: len(rt.vars)})
	var namemap map[string][]interface{}
	if block.Type == ObjectType_Func && block.Info.FuncInfo().Names != nil {
		if rt.stack[len(rt.stack)-1] != nil {
			namemap = rt.stack[len(rt.stack)-1].(map[string][]interface{})
		}
		rt.stack = rt.stack[:len(rt.stack)-1]
	}
	start := len(rt.stack)
	varoff := len(rt.vars)
	for vkey, vpar := range block.Vars {
		rt.cost--
		var value interface{}
		if block.Type == ObjectType_Func && vkey < len(block.Info.FuncInfo().Params) {
			value = rt.stack[start-len(block.Info.FuncInfo().Params)+vkey]
		} else {
			value = reflect.New(vpar).Elem().Interface()
			if vpar == reflect.TypeOf(&types.Map{}) {
				value = types.NewMap()
			} else if vpar == reflect.TypeOf([]interface{}{}) {
				value = make([]interface{}, 0, len(rt.vars)+1)
			}
		}
		rt.addVar(value)
	}
	if namemap != nil {
		for key, item := range namemap {
			params := (*block.Info.FuncInfo().Names)[key]
			for i, value := range item {
				if params.Variadic && i >= len(params.Params)-1 {
					off := varoff + params.Offset[len(params.Params)-1]
					rt.setVar(off, append(rt.vars[off].([]interface{}), value))
				} else {
					rt.setVar(varoff+params.Offset[i], value)
				}
			}
		}
	}
	if block.Type == ObjectType_Func {
		start -= len(block.Info.FuncInfo().Params)
	}
	var (
		assign []*VarInfo
		tmpInt int64
		tmpDec decimal.Decimal
	)
	labels := make([]int, 0)
main:
	for ci := 0; ci < len(block.Code); ci++ {
		rt.cost--
		if rt.cost <= 0 {
			break
		}
		if rt.timeLimit {
			err = ErrVMTimeLimit
			break
		}

		if rt.mem > memoryLimit {
			rt.vm.logger.WithFields(log.Fields{"type": consts.VMError}).Warn(ErrMemoryLimit)
			err = ErrMemoryLimit
			break
		}

		cmd = block.Code[ci]
		var bin interface{}
		size := len(rt.stack)
		if size < int(cmd.Cmd>>8) {
			rt.vm.logger.WithFields(log.Fields{"type": consts.VMError}).Error("stack is empty")
			err = fmt.Errorf(`stack is empty`)
			break
		}
		for i := 1; i <= int(cmd.Cmd>>8); i++ {
			top[i-1] = rt.stack[size-i]
		}
		switch cmd.Cmd {
		case cmdPush:
			rt.stack = append(rt.stack, cmd.Value)
		case cmdPushStr:
			rt.stack = append(rt.stack, cmd.Value.(string))
		case cmdIf:
			if valueToBool(rt.stack[len(rt.stack)-1]) {
				status, err = rt.RunCode(cmd.Value.(*CodeBlock))
			}
		case cmdElse:
			if !valueToBool(rt.stack[len(rt.stack)-1]) {
				status, err = rt.RunCode(cmd.Value.(*CodeBlock))
			}
		case cmdWhile:
			val := rt.stack[len(rt.stack)-1]
			rt.stack = rt.stack[:len(rt.stack)-1]
			if valueToBool(val) {
				status, err = rt.RunCode(cmd.Value.(*CodeBlock))
				newci := labels[len(labels)-1]
				labels = labels[:len(labels)-1]
				if status == statusContinue {
					ci = newci - 1
					status = statusNormal
					continue
				}
				if status == statusBreak {
					status = statusNormal
					break
				}
			}
		case cmdLabel:
			labels = append(labels, ci)
		case cmdContinue:
			status = statusContinue
		case cmdBreak:
			status = statusBreak
		case cmdAssignVar:
			assign = cmd.Value.([]*VarInfo)
		case cmdAssign:
			count := len(assign)
			for ivar, item := range assign {
				if item.Owner == nil {
					if (*item).Obj.Type == ObjectType_Extend {
						if isSysVar((*item).Obj.Value.String()) {
							err = fmt.Errorf(eSysVar, (*item).Obj.Value.String())
							rt.vm.logger.WithFields(log.Fields{"type": consts.VMError, "error": err}).Error("modifying system variable")
							break main
						}
						rt.setExtendVar((*item).Obj.Value.String(), rt.stack[len(rt.stack)-count+ivar])
					}
				} else {
					var i int
					for i = len(rt.blocks) - 1; i >= 0; i-- {
						if item.Owner == rt.blocks[i].Block {
							k := rt.blocks[i].Offset + item.Obj.Value.Int()
							switch rt.blocks[i].Block.Vars[item.Obj.Value.Int()].String() {
							case Decimal:
								var v decimal.Decimal
								v, err = ValueToDecimal(rt.stack[len(rt.stack)-count+ivar])
								if err != nil {
									break main
								}
								rt.setVar(k, v)
							default:
								rt.setVar(k, rt.stack[len(rt.stack)-count+ivar])
							}
							break
						}
					}
				}
			}
		case cmdReturn:
			status = statusReturn
		case cmdError:
			eType := msgError
			if cmd.Value.(uint32) == keyWarning {
				eType = msgWarning
			} else if cmd.Value.(uint32) == keyInfo {
				eType = msgInfo
			}
			err = SetVMError(eType, rt.stack[len(rt.stack)-1])
		case cmdFuncName:
			ifunc := cmd.Value.(FuncNameCmd)
			mapoff := len(rt.stack) - 1 - ifunc.Count
			if rt.stack[mapoff] == nil {
				rt.stack[mapoff] = make(map[string][]interface{})
			}
			params := make([]interface{}, 0, ifunc.Count)
			for i := 0; i < ifunc.Count; i++ {
				cur := rt.stack[mapoff+1+i]
				if i == ifunc.Count-1 && rt.unwrap &&
					reflect.TypeOf(cur).String() == `[]interface {}` {
					params = append(params, cur.([]interface{})...)
					rt.unwrap = false
				} else {
					params = append(params, cur)
				}
			}
			rt.stack[mapoff].(map[string][]interface{})[ifunc.Name] = params
			rt.stack = rt.stack[:mapoff+1]
			continue
		case cmdCallVari, cmdCall:
			if cmd.Value.(*ObjInfo).Type == ObjectType_ExtFunc {
				finfo := cmd.Value.(*ObjInfo).Value.ExtFuncInfo()
				if rt.vm.ExtCost != nil {
					cost := rt.vm.ExtCost(finfo.Name)
					if cost > rt.cost {
						rt.cost = 0
						break main
					} else if cost == -1 {
						rt.cost -= CostCall
					} else {
						rt.cost -= cost
					}
				}
			} else {
				rt.cost -= CostCall
			}
			err = rt.callFunc(cmd.Cmd, cmd.Value.(*ObjInfo))
		case cmdVar:
			ivar := cmd.Value.(*VarInfo)
			var i int
			for i = len(rt.blocks) - 1; i >= 0; i-- {
				if ivar.Owner == rt.blocks[i].Block {
					rt.stack = append(rt.stack, rt.vars[rt.blocks[i].Offset+ivar.Obj.Value.Int()])
					break
				}
			}
			if i < 0 {
				rt.vm.logger.WithFields(log.Fields{"type": consts.VMError, "var": ivar.Obj.Value}).Error("wrong var")
				err = fmt.Errorf(`wrong var %v`, ivar.Obj.Value)
				break main
			}
		case cmdExtend, cmdCallExtend:
			if val, ok := (*rt.extend)[cmd.Value.(string)]; ok {
				rt.cost -= CostExtend
				if cmd.Cmd == cmdCallExtend {
					err = rt.extendFunc(cmd.Value.(string))
					if err != nil {
						rt.vm.logger.WithFields(log.Fields{"type": consts.VMError, "error": err, "cmd": cmd.Value.(string)}).Error("executing extended function")
						err = fmt.Errorf(`extend function %s %s`, cmd.Value.(string), err.Error())
						break main
					}
				} else {
					switch varVal := val.(type) {
					case int:
						val = int64(varVal)
					}
					rt.stack = append(rt.stack, val)
				}
			} else {
				rt.vm.logger.WithFields(log.Fields{"type": consts.VMError, "cmd": cmd.Value.(string)}).Error("unknown extend identifier")
				err = fmt.Errorf(`unknown extend identifier %s`, cmd.Value.(string))
			}
		case cmdIndex:
			rv := reflect.ValueOf(rt.stack[size-2])
			itype := reflect.TypeOf(rt.stack[size-2]).String()

			switch {
			case itype == `*types.Map`:
				if reflect.TypeOf(rt.stack[size-1]).String() != `string` {
					err = fmt.Errorf(eMapIndex, reflect.TypeOf(rt.stack[size-1]).String())
					break
				}
				v, found := rt.stack[size-2].(*types.Map).Get(rt.stack[size-1].(string))
				if found {
					rt.stack[size-2] = v
				} else {
					rt.stack[size-2] = nil
				}
				rt.stack = rt.stack[:size-1]
			case itype[:2] == brackets:
				if reflect.TypeOf(rt.stack[size-1]).String() != `int64` {
					err = fmt.Errorf(eArrIndex, reflect.TypeOf(rt.stack[size-1]).String())
					break
				}
				v := rv.Index(int(rt.stack[size-1].(int64)))
				if v.IsValid() {
					rt.stack[size-2] = v.Interface()
				} else {
					rt.stack[size-2] = nil
				}
				rt.stack = rt.stack[:size-1]
			default:
				itype := reflect.TypeOf(rt.stack[size-2]).String()
				rt.vm.logger.WithFields(log.Fields{"type": consts.VMError, "vm_type": itype}).Error("type does not support indexing")
				err = fmt.Errorf(`Type %s doesn't support indexing`, itype)
			}
		case cmdSetIndex:
			itype := reflect.TypeOf(rt.stack[size-3]).String()
			indexInfo := cmd.Value.(*IndexInfo)
			var indexKey int
			if indexInfo.Owner != nil {
				for i := len(rt.blocks) - 1; i >= 0; i-- {
					if indexInfo.Owner == rt.blocks[i].Block {
						indexKey = rt.blocks[i].Offset + indexInfo.VarOffset
						break
					}
				}
			}
			if isSelfAssignment(rt.stack[size-3], rt.stack[size-1]) {
				err = errSelfAssignment
				break main
			}

			switch {
			case itype == `*types.Map`:
				if rt.stack[size-3].(*types.Map).Size() > maxMapCount {
					err = errMaxMapCount
					break
				}
				if reflect.TypeOf(rt.stack[size-2]).String() != `string` {
					err = fmt.Errorf(eMapIndex, reflect.TypeOf(rt.stack[size-2]).String())
					break
				}
				rt.stack[size-3].(*types.Map).Set(rt.stack[size-2].(string),
					reflect.ValueOf(rt.stack[size-1]).Interface())
				rt.stack = rt.stack[:size-2]
			case itype[:2] == brackets:
				if reflect.TypeOf(rt.stack[size-2]).String() != `int64` {
					err = fmt.Errorf(eArrIndex, reflect.TypeOf(rt.stack[size-2]).String())
					break
				}
				ind := rt.stack[size-2].(int64)
				if strings.Contains(itype, Interface) {
					slice := rt.stack[size-3].([]interface{})
					if int(ind) >= len(slice) {
						if ind > maxArrayIndex {
							err = errMaxArrayIndex
							break
						}
						slice = append(slice, make([]interface{}, int(ind)-len(slice)+1)...)
						indexInfo := cmd.Value.(*IndexInfo)
						if indexInfo.Owner == nil { // Extend variable $varname
							(*rt.extend)[indexInfo.Extend] = slice
						} else {
							rt.vars[indexKey] = slice
						}
						rt.stack[size-3] = slice
					}
					slice[ind] = rt.stack[size-1]
				} else {
					slice := rt.stack[size-3].([]map[string]string)
					slice[ind] = rt.stack[size-1].(map[string]string)
				}
				rt.stack = rt.stack[:size-2]
			default:
				rt.vm.logger.WithFields(log.Fields{"type": consts.VMError, "vm_type": itype}).Error("type does not support indexing")
				err = fmt.Errorf(`Type %s doesn't support indexing`, itype)
			}

			if indexInfo.Owner == nil {
				rt.recalcMemExtendVar(indexInfo.Extend)
			} else {
				rt.recalcMemVar(indexKey)
			}
		case cmdUnwrapArr:
			if reflect.TypeOf(rt.stack[size-1]).String() == `[]interface {}` {
				rt.unwrap = true
			}
		case cmdSign:
			switch top[0].(type) {
			case float64:
				rt.stack[size-1] = -top[0].(float64)
			default:
				rt.stack[size-1] = -top[0].(int64)
			}
		case cmdNot:
			rt.stack[size-1] = !valueToBool(top[0])
		case cmdAdd:
			switch top[1].(type) {
			case string:
				switch top[0].(type) {
				case string:
					bin = top[1].(string) + top[0].(string)
				case int64:
					if tmpInt, err = converter.ValueToInt(top[1]); err == nil {
						bin = tmpInt + top[0].(int64)
					}
				case float64:
					bin = ValueToFloat(top[1]) + top[0].(float64)
				default:
					err = errUnsupportedType
					break main
				}
			case float64:
				switch top[0].(type) {
				case string, int64, float64:
					bin = top[1].(float64) + ValueToFloat(top[0])
				default:
					err = errUnsupportedType
					break main
				}
			case int64:
				switch top[0].(type) {
				case string, int64:
					if tmpInt, err = converter.ValueToInt(top[0]); err == nil {
						bin = top[1].(int64) + tmpInt
					}
				case float64:
					bin = ValueToFloat(top[1]) + top[0].(float64)
				default:
					err = errUnsupportedType
					break main
				}
			default:
				if reflect.TypeOf(top[1]).String() == Decimal &&
					reflect.TypeOf(top[0]).String() == Decimal {
					bin = top[1].(decimal.Decimal).Add(top[0].(decimal.Decimal))
				} else {
					err = errUnsupportedType
					break main
				}
			}
		case cmdSub:
			switch top[1].(type) {
			case string:
				switch top[0].(type) {
				case int64:
					if tmpInt, err = converter.ValueToInt(top[1]); err == nil {
						bin = tmpInt - top[0].(int64)
					}
				case float64:
					bin = ValueToFloat(top[1]) - top[0].(float64)
				default:
					err = errUnsupportedType
					break main
				}
			case float64:
				switch top[0].(type) {
				case string, int64, float64:
					bin = top[1].(float64) - ValueToFloat(top[0])
				default:
					err = errUnsupportedType
					break main
				}
			case int64:
				switch top[0].(type) {
				case int64, string:
					if tmpInt, err = converter.ValueToInt(top[0]); err == nil {
						bin = top[1].(int64) - tmpInt
					}
				case float64:
					bin = ValueToFloat(top[1]) - top[0].(float64)
				default:
					err = errUnsupportedType
					break main
				}
			default:
				if reflect.TypeOf(top[1]).String() == Decimal &&
					reflect.TypeOf(top[0]).String() == Decimal {
					bin = top[1].(decimal.Decimal).Sub(top[0].(decimal.Decimal))
				} else {
					err = errUnsupportedType
					break main
				}
			}
		case cmdMul:
			switch top[1].(type) {
			case string:
				switch top[0].(type) {
				case int64:
					if tmpInt, err = converter.ValueToInt(top[1]); err == nil {
						bin = tmpInt * top[0].(int64)
					}
				case float64:
					bin = ValueToFloat(top[1]) * top[0].(float64)
				default:
					err = errUnsupportedType
					break main
				}
			case float64:
				switch top[0].(type) {
				case string, int64, float64:
					bin = top[1].(float64) * ValueToFloat(top[0])
				default:
					err = errUnsupportedType
					break main
				}
			case int64:
				switch top[0].(type) {
				case int64, string:
					if tmpInt, err = converter.ValueToInt(top[0]); err == nil {
						bin = top[1].(int64) * tmpInt
					}
				case float64:
					bin = ValueToFloat(top[1]) * top[0].(float64)
				default:
					err = errUnsupportedType
					break main
				}
			default:
				if reflect.TypeOf(top[1]).String() == Decimal &&
					reflect.TypeOf(top[0]).String() == Decimal {
					bin = top[1].(decimal.Decimal).Mul(top[0].(decimal.Decimal))
				} else {
					err = errUnsupportedType
					break main
				}
			}
		case cmdDiv:
			switch top[1].(type) {
			case string:
				switch v := top[0].(type) {
				case int64:
					if v == 0 {
						err = errDivZero
						break main
					}
					if tmpInt, err = converter.ValueToInt(top[1]); err == nil {
						bin = tmpInt / v
					}
				case float64:
					if v == 0 {
						err = errDivZero
						break main
					}
					bin = ValueToFloat(top[1]) / v
				default:
					err = errUnsupportedType
					break main
				}
			case float64:
				switch top[0].(type) {
				case string, int64, float64:
					vFloat := ValueToFloat(top[0])
					if vFloat == 0 {
						err = errDivZero
						break main
					}
					bin = top[1].(float64) / vFloat
				default:
					err = errUnsupportedType
					break main
				}
			case int64:
				switch top[0].(type) {
				case int64, string:
					if tmpInt, err = converter.ValueToInt(top[0]); err == nil {
						if tmpInt == 0 {
							err = errDivZero
							break main
						}
						bin = top[1].(int64) / tmpInt
					}
				case float64:
					if top[0].(float64) == 0 {
						err = errDivZero
						break main
					}
					bin = ValueToFloat(top[1]) / top[0].(float64)
				default:
					err = errUnsupportedType
					break main
				}
			default:
				if reflect.TypeOf(top[1]).String() == Decimal &&
					reflect.TypeOf(top[0]).String() == Decimal {
					if top[0].(decimal.Decimal).Cmp(decimal.New(0, 0)) == 0 {
						err = errDivZero
						break main
					}
					bin = top[1].(decimal.Decimal).Div(top[0].(decimal.Decimal)).Floor()
				} else {
					err = errUnsupportedType
					break main
				}
			}
		case cmdAnd:
			bin = valueToBool(top[1]) && valueToBool(top[0])
		case cmdOr:
			bin = valueToBool(top[1]) || valueToBool(top[0])
		case cmdEqual, cmdNotEq:
			if top[1] == nil || top[0] == nil {
				bin = top[0] == top[1]
			} else {
				switch top[1].(type) {
				case string:
					switch top[0].(type) {
					case int64:
						if tmpInt, err = converter.ValueToInt(top[1]); err == nil {
							bin = tmpInt == top[0].(int64)
						}
					case float64:
						bin = ValueToFloat(top[1]) == top[0].(float64)
					default:
						if reflect.TypeOf(top[0]).String() == Decimal {
							if tmpDec, err = ValueToDecimal(top[1]); err != nil {
								break main
							}
							bin = tmpDec.Cmp(top[0].(decimal.Decimal)) == 0
						} else {
							bin = top[1].(string) == top[0].(string)
						}
					}
				case float64:
					bin = top[1].(float64) == ValueToFloat(top[0])
				case int64:
					switch top[0].(type) {
					case int64:
						bin = top[1].(int64) == top[0].(int64)
					case float64:
						bin = ValueToFloat(top[1]) == top[0].(float64)
					default:
						err = errUnsupportedType
						break main
					}
				case bool:
					switch top[0].(type) {
					case bool:
						bin = top[1].(bool) == top[0].(bool)
					default:
						err = errUnsupportedType
						break main
					}
				default:
					if tmpDec, err = ValueToDecimal(top[0]); err != nil {
						break main
					}
					bin = top[1].(decimal.Decimal).Cmp(tmpDec) == 0
				}
			}
			if cmd.Cmd == cmdNotEq {
				bin = !bin.(bool)
			}
		case cmdLess, cmdNotLess:
			switch top[1].(type) {
			case string:
				switch top[0].(type) {
				case int64:
					if tmpInt, err = converter.ValueToInt(top[1]); err == nil {
						bin = tmpInt < top[0].(int64)
					}
				case float64:
					bin = ValueToFloat(top[1]) < top[0].(float64)
				default:
					if reflect.TypeOf(top[0]).String() == Decimal {
						if tmpDec, err = ValueToDecimal(top[1]); err != nil {
							break main
						}
						bin = tmpDec.Cmp(top[0].(decimal.Decimal)) < 0
					} else {
						bin = top[1].(string) < top[0].(string)
					}
				}
			case float64:
				bin = top[1].(float64) < ValueToFloat(top[0])
			case int64:
				switch top[0].(type) {
				case int64:
					bin = top[1].(int64) < top[0].(int64)
				case float64:
					bin = ValueToFloat(top[1]) < top[0].(float64)
				default:
					err = errUnsupportedType
					break main
				}
			default:
				if tmpDec, err = ValueToDecimal(top[0]); err != nil {
					break main
				}
				bin = top[1].(decimal.Decimal).Cmp(tmpDec) < 0
			}
			if cmd.Cmd == cmdNotLess {
				bin = !bin.(bool)
			}
		case cmdGreat, cmdNotGreat:
			switch top[1].(type) {
			case string:
				switch top[0].(type) {
				case int64:
					if tmpInt, err = converter.ValueToInt(top[1]); err == nil {
						bin = tmpInt > top[0].(int64)
					}
				case float64:
					bin = ValueToFloat(top[1]) > top[0].(float64)
				default:
					if reflect.TypeOf(top[0]).String() == Decimal {
						if tmpDec, err = ValueToDecimal(top[1]); err != nil {
							break main
						}
						bin = tmpDec.Cmp(top[0].(decimal.Decimal)) > 0
					} else {
						bin = top[1].(string) > top[0].(string)
					}
				}
			case float64:
				bin = top[1].(float64) > ValueToFloat(top[0])
			case int64:
				switch top[0].(type) {
				case int64:
					bin = top[1].(int64) > top[0].(int64)
				case float64:
					bin = ValueToFloat(top[1]) > top[0].(float64)
				default:
					err = errUnsupportedType
					break main
				}
			default:
				if tmpDec, err = ValueToDecimal(top[0]); err != nil {
					break main
				}
				bin = top[1].(decimal.Decimal).Cmp(tmpDec) > 0
			}
			if cmd.Cmd == cmdNotGreat {
				bin = !bin.(bool)
			}
		case cmdArrayInit:
			initArray, err := rt.getResultArray(cmd.Value.([]mapItem))
			if err != nil {
				break main
			}
			rt.stack = append(rt.stack, initArray)
		case cmdMapInit:
			var initMap *types.Map
			initMap, err = rt.getResultMap(cmd.Value.(*types.Map))
			if err != nil {
				break main
			}
			rt.stack = append(rt.stack, initMap)
		default:
			rt.vm.logger.WithFields(log.Fields{"type": consts.VMError, "vm_cmd": cmd.Cmd}).Error("Unknown command")
			err = fmt.Errorf(`Unknown command %d`, cmd.Cmd)
		}
		if err != nil {
			rt.err = err
			break
		}
		if status == statusReturn || status == statusContinue || status == statusBreak {
			break
		}
		if (cmd.Cmd >> 8) == 2 {
			rt.stack[size-2] = bin
			rt.stack = rt.stack[:size-1]
		}
	}
	last := rt.blocks[len(rt.blocks)-1]
	rt.blocks = rt.blocks[:len(rt.blocks)-1]
	if status == statusReturn {
		if last.Block.Type == ObjectType_Func {
			lastResults := last.Block.Info.FuncInfo().Results
			if len(lastResults) > len(rt.stack) {
				var keyNames []string
				for i := 0; i < len(lastResults); i++ {
					keyNames = append(keyNames, lastResults[i].String())
				}
				err = fmt.Errorf("not enough arguments to return, need [%s]", strings.Join(keyNames, "|"))
				return
			}
			for count := len(lastResults); count > 0; count-- {
				rt.stack[start] = rt.stack[len(rt.stack)-count]
				start++
			}
			status = statusNormal
		} else {
			return
		}
	}
	rt.stack = rt.stack[:start]
	if rt.cost <= 0 {
		rt.vm.logger.WithFields(log.Fields{"type": consts.VMError}).Warn("runtime cost limit overflow")
		err = fmt.Errorf(`runtime cost limit overflow`)
	}
	return
}

// Run executes CodeBlock with the specified parameters and extended variables and functions
func (rt *RunTime) Run(block *CodeBlock, params []interface{}, extend *map[string]interface{}) (ret []interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			//rt.vm.logger.WithFields(log.Fields{"type": consts.PanicRecoveredError, "error_info": r, "stack": string(debug.Stack())}).Error("runtime panic error")
			err = fmt.Errorf(`runtime panic error,%v`, r)
		}
	}()
	info := block.Info.FuncInfo()
	rt.extend = extend
	var (
		genBlock bool
		timer    *time.Timer
	)
	if gen, ok := (*extend)[`gen_block`]; ok {
		genBlock = gen.(bool)
	}
	timeOver := func() {
		rt.timeLimit = true
	}
	if genBlock {
		timer = time.AfterFunc(time.Millisecond*time.Duration((*extend)[`time_limit`].(int64)), timeOver)
	}
	if _, err = rt.RunCode(block); err == nil {
		off := len(rt.stack) - len(info.Results)
		for i := 0; i < len(info.Results); i++ {
			ret = append(ret, rt.stack[off+i])
		}
	}
	if genBlock {
		timer.Stop()
	}
	return
}
