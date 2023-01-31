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
	sysVars_block:               {},
	sysVars_block_key_id:        {},
	sysVars_block_time:          {},
	sysVars_data:                {},
	sysVars_ecosystem_id:        {},
	sysVars_key_id:              {},
	sysVars_account_id:          {},
	sysVars_node_position:       {},
	sysVars_parent:              {},
	sysVars_original_contract:   {},
	sysVars_sc:                  {},
	sysVars_contract:            {},
	sysVars_stack:               {},
	sysVars_this_contract:       {},
	sysVars_time:                {},
	sysVars_type:                {},
	sysVars_txcost:              {},
	sysVars_txhash:              {},
	sysVars_guest_key:           {},
	sysVars_guest_account:       {},
	sysVars_black_hole_key:      {},
	sysVars_black_hole_account:  {},
	sysVars_white_hole_key:      {},
	sysVars_white_hole_account:  {},
	sysVars_gen_block:           {},
	sysVars_time_limit:          {},
	sysVars_pre_block_data_hash: {},
}

var (
	ErrMemoryLimit = errors.New("Memory limit exceeded")
	//ErrVMTimeLimit returns when the time limit exceeded
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
	stack     []any
	blocks    []*blockStack
	vars      []any
	extend    map[string]any
	vm        *VM
	cost      int64
	err       error
	unwrap    bool
	timeLimit bool
	callDepth uint16
	mem       int64
	memVars   map[any]int64
	errInfo   ErrInfo
}

// NewRunTime creates a new RunTime for the virtual machine
func NewRunTime(vm *VM, cost int64) *RunTime {
	return &RunTime{
		stack:   make([]any, 0, 1024),
		vm:      vm,
		cost:    cost,
		memVars: make(map[any]int64),
	}
}

func isSysVar(name string) bool {
	if _, ok := sysVars[name]; ok || strings.HasPrefix(name, Extend_loop) {
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

	size := rt.len()
	in = obj.getInParams()
	if rt.unwrap && cmd == cmdCallVariadic && size > 1 &&
		reflect.TypeOf(rt.stack[size-2]).String() == `[]interface {}` {
		count = rt.getStack(size - 1).(int)
		arr := rt.getStack(size - 2).([]any)
		rt.resetByIdx(size - 2)
		for _, item := range arr {
			rt.push(item)
		}
		rt.push(count - 1 + len(arr))
		size = rt.len()
	}
	rt.unwrap = false
	if cmd == cmdCallVariadic {
		count = rt.getStack(size - 1).(int)
		size--
	} else {
		count = in
	}
	if obj.Type == ObjectType_Func {
		var imap map[string][]any
		finfo := obj.GetCodeBlock().GetFuncInfo()
		if finfo.Names != nil {
			if rt.getStack(size-1) != nil {
				imap = rt.getStack(size - 1).(map[string][]any)
			}
			rt.resetByIdx(size - 1)
			size = rt.len()
		}
		if cmd == cmdCallVariadic {
			parcount := count + 1 - in
			if parcount < 0 {
				log.WithFields(log.Fields{"type": consts.VMError}).Error(errWrongCountPars)
				return errWrongCountPars
			}
			pars := make([]any, parcount)
			shift := size - parcount
			for i := parcount; i > 0; i-- {
				pars[i-1] = rt.stack[size+i-parcount-1]
			}
			rt.resetByIdx(shift)
			rt.push(pars)
		}
		if rt.len() < len(finfo.Params) {
			log.WithFields(log.Fields{"type": consts.VMError}).Error(errWrongCountPars)
			return errWrongCountPars
		}
		for i, v := range finfo.Params {
			switch v.Kind() {
			case reflect.String, reflect.Int64:
				offset := rt.len() - in + i
				if v.Kind() == reflect.Int64 {
					rv := reflect.ValueOf(rt.stack[offset])
					switch rv.Kind() {
					case reflect.Float64:
						val, _ := converter.ValueToInt(rt.stack[offset])
						rt.stack[offset] = val
					}
				}
				if reflect.TypeOf(rt.stack[offset]) != v {
					log.WithFields(log.Fields{"type": consts.VMError}).Error(fmt.Sprintf(eTypeParam, i+1))
					return fmt.Errorf(eTypeParam, i+1)
				}
			}
		}
		if finfo.Names != nil {
			rt.push(imap)
		}
		_, err = rt.RunCode(obj.GetCodeBlock())
		return
	}

	var (
		stack  Stacker
		ok     bool
		result []reflect.Value
		limit  = 0
		finfo  = obj.GetExtFuncInfo()
		foo    = reflect.ValueOf(finfo.Func)
		pars   = make([]reflect.Value, in)
	)
	if stack, ok = rt.extend[Extend_sc].(Stacker); ok {
		if err := stack.AppendStack(finfo.Name); err != nil {
			return err
		}
	}
	rt.extend[Extend_rt] = rt
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
			pars[count-i] = reflect.ValueOf(rt.extend[finfo.Auto[count-i]])
			auto--
		} else {
			pars[count-i] = reflect.ValueOf(rt.stack[size-i+auto])
		}
		if !pars[count-i].IsValid() {
			pars[count-i] = reflect.Zero(reflect.TypeOf(``))
		}
	}
	if i > 0 && size-i >= 0 {
		pars[in-1] = reflect.ValueOf(rt.stack[size-i : size])
	} else {
		if !pars[in-1].IsValid() {
			pars[in-1] = reflect.Zero(finfo.Params[in-1])
		}
	}
	if finfo.Name == `ExecContract` && (pars[2].Kind() != reflect.String || !pars[3].IsValid()) {
		return fmt.Errorf(`unknown function %v`, pars[1])
	}
	if finfo.Variadic {
		result = foo.CallSlice(pars)
	} else {
		result = foo.Call(pars)
	}
	if shift < 0 {
		shift = 0
	}
	rt.resetByIdx(shift)
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
					rt.vm.logger.Error("paid CPU resource is over")
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
			rt.push(iret.Interface())
		}
	}
	return
}

func (rt *RunTime) extendFunc(name string) error {
	var (
		ok bool
		f  any
	)
	if f, ok = rt.extend[name]; !ok || reflect.ValueOf(f).Kind() != reflect.Func {
		return fmt.Errorf(`unknown function %s`, name)
	}
	size := rt.len()
	foo := reflect.ValueOf(f)

	count := foo.Type().NumIn()
	pars := make([]reflect.Value, count)
	for i := count; i > 0; i-- {
		pars[count-i] = reflect.ValueOf(rt.stack[size-i])
	}
	result := foo.Call(pars)

	rt.resetByIdx(size - count)
	for i, iret := range result {
		if foo.Type().Out(i).String() == `error` {
			if iret.Interface() != nil {
				return iret.Interface().(error)
			}
		} else {
			rt.push(iret.Interface())
		}
	}
	return nil
}

func calcMem(v any) (mem int64) {
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

func (rt *RunTime) setExtendVar(k string, v any) {
	rt.extend[k] = v
	rt.recalcMemExtendVar(k)
}

func (rt *RunTime) recalcMemExtendVar(k string) {
	mem := calcMem(rt.extend[k])
	rt.mem += mem - rt.memVars[k]
	rt.memVars[k] = mem
}

func (rt *RunTime) addVar(v any) {
	rt.vars = append(rt.vars, v)
	mem := calcMem(v)
	rt.memVars[len(rt.vars)-1] = mem
	rt.mem += mem
}

func (rt *RunTime) setVar(k int, v any) {
	rt.vars[k] = v
	rt.recalcMemVar(k)
}

func (rt *RunTime) recalcMemVar(k int) {
	mem := calcMem(rt.vars[k])
	rt.mem += mem - rt.memVars[k]
	rt.memVars[k] = mem
}

func valueToBool(v any) bool {
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
	case []any:
		return val != nil && len(val) > 0
	case map[string]any:
		return val != nil && len(val) > 0
	case map[string]string:
		return val != nil && len(val) > 0
	case *types.Map:
		return val != nil && val.Size() > 0
	default:
		dec, _ := decimal.NewFromString(fmt.Sprintf(`%v`, val))
		return dec.Cmp(decimal.Zero) != 0
	}
	return false
}

// ValueToFloat converts interface (string, float64 or int64) to float64
func ValueToFloat(v any) (ret float64) {
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
	case decimal.Decimal:
		ret = val.InexactFloat64()
	}
	return
}

// ValueToDecimal converts interface (string, float64, Decimal or int64) to Decimal
func ValueToDecimal(v any) (ret decimal.Decimal, err error) {
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
func SetVMError(eType string, eText any) error {
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

func (rt *RunTime) getResultValue(item mapItem) (value any, err error) {
	switch item.Type {
	case mapConst:
		value = item.Value
	case mapExtend:
		var ok bool
		value, ok = rt.extend[item.Value.(string)]
		if !ok {
			rt.vm.logger.WithFields(log.Fields{"cmd": item.Value}).Error("unknown extend identifier")
			err = fmt.Errorf(`unknown extend identifier %s`, item.Value)
		}
	case mapVar:
		ivar := item.Value.(*VarInfo)
		var i int
		for i = len(rt.blocks) - 1; i >= 0; i-- {
			if ivar.Owner == rt.blocks[i].Block {
				value = rt.vars[rt.blocks[i].Offset+ivar.Obj.GetVariable().Index]
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

func (rt *RunTime) getResultArray(cmd []mapItem) ([]any, error) {
	initArr := make([]any, 0)
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

func isSelfAssignment(dest, value any) bool {
	if _, ok := value.([]any); !ok {
		if _, ok = value.(*types.Map); !ok {
			return false
		}
	}
	if reflect.ValueOf(dest).Pointer() == reflect.ValueOf(value).Pointer() {
		return true
	}
	switch v := value.(type) {
	case []any:
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
			err = errors.Errorf(`runtime run code crashed: %v`, r)
		}
		if err != nil && !strings.HasPrefix(err.Error(), `{`) {
			var curContract, line string
			if block.isParentContract() {
				stack := block.Parent.GetContractInfo()
				curContract = stack.Name
			}
			if stack, ok := rt.extend[Extend_stack].([]any); ok {
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
	top := make([]any, 8)
	rt.blocks = append(rt.blocks, &blockStack{Block: block, Offset: len(rt.vars)})
	var namemap map[string][]any
	if block.Type == ObjectType_Func && block.GetFuncInfo().Names != nil {
		if rt.peek() != nil {
			namemap = rt.peek().(map[string][]any)
		}
		rt.resetByIdx(rt.len() - 1)
	}
	start := rt.len()
	varoff := len(rt.vars)
	for vkey, vpar := range block.Vars {
		rt.cost--
		var value any
		if block.Type == ObjectType_Func && vkey < len(block.GetFuncInfo().Params) {
			value = rt.stack[start-len(block.GetFuncInfo().Params)+vkey]
		} else {
			value = reflect.New(vpar).Elem().Interface()
			if vpar == reflect.TypeOf(&types.Map{}) {
				value = types.NewMap()
			} else if vpar == reflect.TypeOf([]any{}) {
				value = make([]any, 0, len(rt.vars)+1)
			}
		}
		rt.addVar(value)
	}
	if namemap != nil {
		for key, item := range namemap {
			params := (*block.GetFuncInfo().Names)[key]
			for i, value := range item {
				if params.Variadic && i >= len(params.Params)-1 {
					off := varoff + params.Offset[len(params.Params)-1]
					rt.setVar(off, append(rt.vars[off].([]any), value))
				} else {
					rt.setVar(varoff+params.Offset[i], value)
				}
			}
		}
	}
	if block.Type == ObjectType_Func {
		start -= len(block.GetFuncInfo().Params)
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
		var bin any
		size := rt.len()
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
			rt.push(cmd.Value)
		case cmdPushStr:
			rt.push(cmd.Value.(string))
		case cmdIf:
			if valueToBool(rt.peek()) {
				status, err = rt.RunCode(cmd.Value.(*CodeBlock))
			}
		case cmdElse:
			if !valueToBool(rt.peek()) {
				status, err = rt.RunCode(cmd.Value.(*CodeBlock))
			}
		case cmdWhile:
			val := rt.peek()
			rt.resetByIdx(rt.len() - 1)
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
				val := rt.stack[rt.len()-count+ivar]
				if item.Owner == nil {
					if item.Obj.Type == ObjectType_ExtVar {
						var n = item.Obj.GetExtendVariable().Name
						if isSysVar(n) {
							err = fmt.Errorf(eSysVar, n)
							rt.vm.logger.WithError(err).Error("modifying system variable")
							break main
						}
						if v, ok := rt.extend[n]; ok && v != nil && reflect.TypeOf(v) != reflect.TypeOf(val) {
							err = fmt.Errorf("$%s (type %s) cannot be represented by the type %s", n, reflect.TypeOf(val), reflect.TypeOf(v))
							break
						}
						rt.setExtendVar(n, val)
					}
				} else {
					for i := len(rt.blocks) - 1; i >= 0; i-- {
						if item.Owner == rt.blocks[i].Block {
							k := rt.blocks[i].Offset + item.Obj.GetVariable().Index
							switch v := rt.blocks[i].Block.Vars[item.Obj.GetVariable().Index]; v.String() {
							case Decimal:
								var v decimal.Decimal
								v, err = ValueToDecimal(val)
								if err != nil {
									break main
								}
								rt.setVar(k, v)
							default:
								if val != nil && v != reflect.TypeOf(val) {
									err = fmt.Errorf("variable '%v' (type %s) cannot be represented by the type %s", item.Obj.GetVariable().Name, reflect.TypeOf(val), v)
									break
								}
								rt.setVar(k, val)
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
			err = SetVMError(eType, rt.peek())
		case cmdFuncName:
			ifunc := cmd.Value.(FuncNameCmd)
			mapoff := rt.len() - 1 - ifunc.Count
			if rt.stack[mapoff] == nil {
				rt.stack[mapoff] = make(map[string][]any)
			}
			params := make([]any, 0, ifunc.Count)
			for i := 0; i < ifunc.Count; i++ {
				cur := rt.stack[mapoff+1+i]
				if i == ifunc.Count-1 && rt.unwrap &&
					reflect.TypeOf(cur).String() == `[]interface {}` {
					params = append(params, cur.([]any)...)
					rt.unwrap = false
				} else {
					params = append(params, cur)
				}
			}
			rt.stack[mapoff].(map[string][]any)[ifunc.Name] = params
			rt.resetByIdx(mapoff + 1)
			continue
		case cmdCallVariadic, cmdCall:
			if cmd.Value.(*ObjInfo).Type == ObjectType_ExtFunc {
				finfo := cmd.Value.(*ObjInfo).GetExtFuncInfo()
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
					rt.push(rt.vars[rt.blocks[i].Offset+ivar.Obj.GetVariable().Index])
					break
				}
			}
			if i < 0 {
				rt.vm.logger.WithFields(log.Fields{"var": ivar.Obj.Value}).Error("wrong var")
				err = fmt.Errorf(`wrong var %v`, ivar.Obj.Value)
				break main
			}
		case cmdExtend, cmdCallExtend:
			if val, ok := rt.extend[cmd.Value.(string)]; ok {
				rt.cost -= CostExtend
				if cmd.Cmd == cmdCallExtend {
					err = rt.extendFunc(cmd.Value.(string))
					if err != nil {
						rt.vm.logger.WithFields(log.Fields{"error": err, "cmd": cmd.Value.(string)}).Error("executing extended function")
						err = fmt.Errorf(`extend function %s %s`, cmd.Value.(string), err)
						break main
					}
				} else {
					switch varVal := val.(type) {
					case int:
						val = int64(varVal)
					}
					rt.push(val)
				}
			} else {
				rt.vm.logger.WithFields(log.Fields{"cmd": cmd.Value}).Error("unknown extend identifier")
				err = fmt.Errorf(`unknown extend identifier %s`, cmd.Value.(string))
			}
		case cmdIndex:
			rv := reflect.ValueOf(rt.stack[size-2])
			itype := reflect.TypeOf(rt.stack[size-2]).String()

			switch {
			case itype == `*types.Map`:
				if reflect.TypeOf(rt.getStack(size-1)).String() != `string` {
					err = fmt.Errorf(eMapIndex, reflect.TypeOf(rt.getStack(size-1)).String())
					break
				}
				v, found := rt.stack[size-2].(*types.Map).Get(rt.getStack(size - 1).(string))
				if found {
					rt.stack[size-2] = v
				} else {
					rt.stack[size-2] = nil
				}
				rt.resetByIdx(size - 1)
			case itype[:2] == brackets:
				if reflect.TypeOf(rt.getStack(size-1)).String() != `int64` {
					err = fmt.Errorf(eArrIndex, reflect.TypeOf(rt.getStack(size-1)).String())
					break
				}
				v := rv.Index(int(rt.getStack(size - 1).(int64)))
				if v.IsValid() {
					rt.stack[size-2] = v.Interface()
				} else {
					rt.stack[size-2] = nil
				}
				rt.resetByIdx(size - 1)
			default:
				itype := reflect.TypeOf(rt.stack[size-2]).String()
				rt.vm.logger.WithFields(log.Fields{"vm_type": itype}).Error("type does not support indexing")
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
			if isSelfAssignment(rt.stack[size-3], rt.getStack(size-1)) {
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
					reflect.ValueOf(rt.getStack(size-1)).Interface())
				rt.resetByIdx(size - 2)
			case itype[:2] == brackets:
				if reflect.TypeOf(rt.stack[size-2]).String() != `int64` {
					err = fmt.Errorf(eArrIndex, reflect.TypeOf(rt.stack[size-2]).String())
					break
				}
				ind := rt.stack[size-2].(int64)
				if strings.Contains(itype, Interface) {
					slice := rt.stack[size-3].([]any)
					if int(ind) >= len(slice) {
						if ind > maxArrayIndex {
							err = errMaxArrayIndex
							break
						}
						slice = append(slice, make([]any, int(ind)-len(slice)+1)...)
						indexInfo := cmd.Value.(*IndexInfo)
						if indexInfo.Owner == nil { // Extend variable $varname
							rt.extend[indexInfo.Extend] = slice
						} else {
							rt.vars[indexKey] = slice
						}
						rt.stack[size-3] = slice
					}
					slice[ind] = rt.getStack(size - 1)
				} else {
					slice := rt.getStack(size - 3).([]map[string]string)
					slice[ind] = rt.getStack(size - 1).(map[string]string)
				}
				rt.resetByIdx(size - 2)
			default:
				rt.vm.logger.WithFields(log.Fields{"vm_type": itype}).Error("type does not support indexing")
				err = fmt.Errorf(`type %s doesn't support indexing`, itype)
			}

			if indexInfo.Owner == nil {
				rt.recalcMemExtendVar(indexInfo.Extend)
			} else {
				rt.recalcMemVar(indexKey)
			}
		case cmdUnwrapArr:
			if reflect.TypeOf(rt.getStack(size-1)).String() == `[]interface {}` {
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
					if top[0].(decimal.Decimal).Cmp(decimal.Zero) == 0 {
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
			var initArray []any
			initArray, err = rt.getResultArray(cmd.Value.([]mapItem))
			if err != nil {
				break main
			}
			rt.push(initArray)
		case cmdMapInit:
			var initMap *types.Map
			initMap, err = rt.getResultMap(cmd.Value.(*types.Map))
			if err != nil {
				break main
			}
			rt.push(initMap)
		default:
			rt.vm.logger.WithFields(log.Fields{"vm_cmd": cmd.Cmd}).Error("Unknown command")
			err = fmt.Errorf(`unknown command %d`, cmd.Cmd)
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
			rt.resetByIdx(size - 1)
		}
	}
	last := rt.popBlock()
	if status == statusReturn {
		if last.Block.Type == ObjectType_Func {
			lastResults := last.Block.GetFuncInfo().Results
			if len(lastResults) > rt.len() {
				var keyNames []string
				for i := 0; i < len(lastResults); i++ {
					keyNames = append(keyNames, lastResults[i].String())
				}
				err = fmt.Errorf("func '%s' not enough arguments to return, need [%s]", last.Block.GetFuncInfo().Name, strings.Join(keyNames, "|"))
				return
			}
			stackCpy := make([]any, rt.len())
			copy(stackCpy, rt.stack)
			var index int
			for count := len(lastResults); count > 0; count-- {
				val := stackCpy[len(stackCpy)-1-index]
				if val != nil && lastResults[count-1] != reflect.TypeOf(val) {
					err = fmt.Errorf("function '%s' return index[%d] (type %s) cannot be represented by the type %s", last.Block.GetFuncInfo().Name, count-1, reflect.TypeOf(val), lastResults[count-1])
					return
				}
				rt.stack[start] = rt.stack[rt.len()-count]
				start++
				index++
			}
			status = statusNormal
		} else {
			return
		}
	}

	rt.resetByIdx(start)
	if rt.cost <= 0 {
		rt.vm.logger.WithFields(log.Fields{"type": consts.VMError}).Warn("runtime cost limit overflow")
		err = fmt.Errorf(`runtime cost limit overflow`)
	}
	return
}

// Run executes CodeBlock with the specified parameters and extended variables and functions
func (rt *RunTime) Run(block *CodeBlock, params []any, extend map[string]any) (ret []any, err error) {
	defer func() {
		if r := recover(); r != nil {
			//rt.vm.logger.WithFields(log.Fields{"type": consts.PanicRecoveredError, "error_info": r, "stack": string(debug.Stack())}).Error("runtime panic error")
			err = fmt.Errorf(`runtime panic: %v`, r)
		}
	}()
	info := block.GetFuncInfo()
	rt.extend = extend
	var (
		genBlock bool
		timer    *time.Timer
	)
	if gen, ok := extend[Extend_gen_block]; ok {
		genBlock = gen.(bool)
	}
	timeOver := func() {
		rt.timeLimit = true
	}
	if genBlock {
		timer = time.AfterFunc(time.Millisecond*time.Duration(extend[Extend_time_limit].(int64)), timeOver)
	}
	if _, err = rt.RunCode(block); err == nil {
		if rt.len() < len(info.Results) {
			var keyNames []string
			for i := 0; i < len(info.Results); i++ {
				keyNames = append(keyNames, info.Results[i].String())
			}
			err = fmt.Errorf("not enough arguments to return, need [%s]", strings.Join(keyNames, "|"))
		}
		off := rt.len() - len(info.Results)
		for i := 0; i < len(info.Results) && off >= 0; i++ {
			ret = append(ret, rt.stack[off+i])
		}
	}
	if genBlock {
		timer.Stop()
	}
	return
}
