/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/types"

	log "github.com/sirupsen/logrus"
)

// operPrior contains command and its priority
type operPrior struct {
	Cmd      uint16 // identifier of the command
	Priority uint16 // priority of the command
}

// State contains a new state and a handle function
type compileState struct {
	NewState   stateTypes  // a new state
	FuncFlag   int         // a handle flag
	FuncHandle compileFunc // a handle function
}

const (
	mapConst = iota
	mapVar
	mapMap
	mapExtend
	mapArray

	mustKey
	mustColon
	mustComma
	mustValue
)

type mapItem struct {
	Type  int
	Value interface{}
}

// The compiler converts the sequence of lexemes into the bytecodes using a finite state machine the same as
// it was implemented in lexical analysis. The difference lays in that we do not convert the list of
// states and transitions to the intermediate array.

const (
	// Errors of compilation
	//	errNoError    = iota
	errUnknownCmd = iota + 1 // unknown command
	errMustName              // must be the name
	errMustLCurly            // must be '{'
	errMustRCurly            // must be '}'
	errParams                // wrong parameters
	errVars                  // wrong variables
	errVarType               // must be type
	errAssign                // must be '='
	errStrNum                // must be number or string
)

var (
	// Array of operations and their priority
	opers = map[uint32]operPrior{
		isOr:       {Cmd: cmdOr, Priority: 10},
		isAnd:      {Cmd: cmdAnd, Priority: 15},
		isEqEq:     {Cmd: cmdEqual, Priority: 20},
		isNotEq:    {Cmd: cmdNotEq, Priority: 20},
		isLess:     {Cmd: cmdLess, Priority: 22},
		isGrEq:     {Cmd: cmdNotLess, Priority: 22},
		isGreat:    {Cmd: cmdGreat, Priority: 22},
		isLessEq:   {Cmd: cmdNotGreat, Priority: 22},
		isPlus:     {Cmd: cmdAdd, Priority: 25},
		isMinus:    {Cmd: cmdSub, Priority: 25},
		isAsterisk: {Cmd: cmdMul, Priority: 30},
		isSolidus:  {Cmd: cmdDiv, Priority: 30},
		isSign:     {Cmd: cmdSign, Priority: cmdUnary},
		isNot:      {Cmd: cmdNot, Priority: cmdUnary},
		isLPar:     {Cmd: cmdSys, Priority: 0xff},
		isRPar:     {Cmd: cmdSys, Priority: 0},
	}
)

// StateName checks the name of the contract and modifies it to @[state]name if it is necessary.
func StateName(state uint32, name string) string {
	if !strings.HasPrefix(name, `@`) {
		return fmt.Sprintf(`@%d%s`, state, name)
	} else if len(name) > 1 && (name[1] < '0' || name[1] > '9') {
		name = `@1` + name[1:]
	}
	return name
}

// CompileBlock compile the source code into the CodeBlock structure with a byte-code
func (vm *VM) CompileBlock(input []rune, owner *OwnerInfo) (*CodeBlock, error) {
	root := &CodeBlock{Info: newCodeBlockInfo(owner.StateID), Owner: owner}
	lexems, err := lexParser(input)
	if err != nil {
		return nil, err
	}
	if len(lexems) == 0 {
		return root, nil
	}
	curState := stateTypes(0)
	stack := make([]stateTypes, 0, 64)
	blockstack := make(CodeBlocks, 1, 64)
	blockstack[0] = root
	fork := 0

	for i := 0; i < len(lexems); i++ {
		var (
			newState compileState
			ok       bool
		)
		lexem := lexems[i]
		if newState, ok = states[curState][int(lexem.Type)]; !ok {
			newState = states[curState][0]
		}
		nextState := newState.NewState & 0xff
		if (newState.NewState & stateFork) > 0 {
			fork = i
		}
		if (newState.NewState & stateToFork) > 0 {
			i = fork
			fork = 0
			lexem = lexems[i]
		}

		if (newState.NewState & stateStay) > 0 {
			curState = nextState
			i--
			continue
		}
		if nextState == stateEval {
			if newState.NewState&stateLabel > 0 {
				blockstack.peek().Code.push(newByteCode(cmdLabel, lexem.Line, 0))
			}
			curlen := len(blockstack.peek().Code)
			if err := vm.compileEval(&lexems, &i, &blockstack); err != nil {
				return nil, err
			}
			if (newState.NewState&stateMustEval) > 0 && curlen == len(blockstack.peek().Code) {
				log.WithFields(log.Fields{"type": consts.ParseError}).Error("there is not eval expression")
				return nil, fmt.Errorf("there is not eval expression")
			}
			nextState = curState
		}
		if (newState.NewState & statePush) > 0 {
			stack = append(stack, curState)
			top := blockstack.peek()
			if top.Objects == nil {
				top.Objects = make(map[string]*ObjInfo)
			}
			block := &CodeBlock{Parent: top}
			top.Children.push(block)
			blockstack.push(block)
		}
		if (newState.NewState & statePop) > 0 {
			if len(stack) == 0 {
				return nil, fError(&blockstack, errMustLCurly, lexem)
			}
			nextState = stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			if len(blockstack) >= 2 {
				prev := blockstack.get(len(blockstack) - 2)
				if len(prev.Code) > 0 && (*prev).Code[len((*prev).Code)-1].Cmd == cmdContinue {
					(*prev).Code = (*prev).Code[:len((*prev).Code)-1]
					prev = blockstack.peek()
					(*prev).Code.push(newByteCode(cmdContinue, lexem.Line, 0))
				}
			}
			blockstack = blockstack[:len(blockstack)-1]
		}
		if (newState.NewState & stateToBlock) > 0 {
			nextState = stateBlock
		}
		if (newState.NewState & stateToBody) > 0 {
			nextState = stateBody
		}
		if newState.FuncFlag > 0 {
			if err := funcHandles[newState.FuncFlag](&blockstack, nextState, lexem); err != nil {
				return nil, err
			}
		}
		curState = nextState
	}
	if len(stack) > 0 {
		return nil, fError(&blockstack, errMustRCurly, lexems[len(lexems)-1])
	}
	for _, item := range root.Objects {
		if item.Type == ObjectType_Contract {
			if cond, ok := item.Value.CodeBlock().Objects[`conditions`]; ok {
				if cond.Type == ObjectType_Func && cond.Value.CodeBlock().Info.FuncInfo().CanWrite {
					return nil, errCondWrite
				}
			}
		}
	}
	return root, nil
}

// FlushBlock loads the compiled CodeBlock into the virtual machine
func (vm *VM) FlushBlock(root *CodeBlock) {
	shift := len(vm.Children)
	for key, item := range root.Objects {
		if cur, ok := vm.Objects[key]; ok {
			switch item.Type {
			case ObjectType_Contract:
				root.Objects[key].Value.CodeBlock().Info.ContractInfo().ID = cur.Value.CodeBlock().Info.ContractInfo().ID + flushMark
			case ObjectType_Func:
				root.Objects[key].Value.CodeBlock().Info.FuncInfo().ID = cur.Value.CodeBlock().Info.FuncInfo().ID + flushMark
				vm.Objects[key].Value = root.Objects[key].Value
			}
		}
		vm.Objects[key] = item
	}
	for _, item := range root.Children {
		switch item.Type {
		case ObjectType_Contract:
			if item.Info.ContractInfo().ID > flushMark {
				item.Info.ContractInfo().ID -= flushMark
				vm.Children[item.Info.ContractInfo().ID] = item
				shift--
				continue
			}
			item.Parent = vm.CodeBlock
			item.Info.ContractInfo().ID += uint32(shift)
		case ObjectType_Func:
			if item.Info.FuncInfo().ID > flushMark {
				item.Info.FuncInfo().ID -= flushMark
				vm.Children[item.Info.FuncInfo().ID] = item
				shift--
				continue
			}
			item.Parent = vm.CodeBlock
			item.Info.FuncInfo().ID += uint32(shift)
		}
		vm.Children = append(vm.Children, item)
	}
}

// FlushExtern switches off the extern mode of the compilation
func (vm *VM) FlushExtern() {
	vm.Extern = false
	return
}

// Compile compiles a source code and loads the byte-code into the virtual machine
func (vm *VM) Compile(input []rune, owner *OwnerInfo) error {
	root, err := vm.CompileBlock(input, owner)
	if err == nil {
		vm.FlushBlock(root)
	}
	return err
}

func findVar(name string, block *CodeBlocks) (ret *ObjInfo, owner *CodeBlock) {
	var ok bool
	i := len(*block) - 1
	for ; i >= 0; i-- {
		ret, ok = (*block)[i].Objects[name]
		if ok {
			return ret, (*block)[i]
		}
	}
	return nil, nil
}

func (vm *VM) findObj(name string, block *CodeBlocks) (ret *ObjInfo, owner *CodeBlock) {
	sname := StateName((*block)[0].Info.Uint32(), name)
	ret, owner = findVar(name, block)
	if ret != nil {
		return
	} else if len(sname) > 0 {
		if ret, owner = findVar(sname, block); ret != nil {
			return
		}
	}
	if ret = vm.getObjByName(name); ret == nil && len(sname) > 0 {
		ret = vm.getObjByName(sname)
	}
	return
}

func (vm *VM) getInitValue(lexems *Lexems, ind *int, block *CodeBlocks) (value mapItem, err error) {
	var (
		subArr []mapItem
		subMap *types.Map
	)
	i := *ind
	lexem := (*lexems)[i]

	switch lexem.Type {
	case isLBrack:
		subArr, err = vm.getInitArray(lexems, &i, block)
		if err == nil {
			value = mapItem{Type: mapArray, Value: subArr}
		}
	case isLCurly:
		subMap, err = vm.getInitMap(lexems, &i, block, false)
		if err == nil {
			value = mapItem{Type: mapMap, Value: subMap}
		}
	case lexExtend:
		value = mapItem{Type: mapExtend, Value: lexem.Value}
	case lexIdent:
		objInfo, tobj := vm.findObj(lexem.Value.(string), block)
		if objInfo == nil {
			err = fmt.Errorf(eUnknownIdent, lexem.Value.(string))
		} else {
			value = mapItem{Type: mapVar, Value: &VarInfo{Obj: objInfo, Owner: tobj}}
		}
	case lexNumber, lexString:
		value = mapItem{Type: mapConst, Value: lexem.Value}
	default:
		err = errUnexpValue
	}
	*ind = i
	return
}

func (vm *VM) getInitMap(lexems *Lexems, ind *int, block *CodeBlocks, oneItem bool) (*types.Map, error) {
	var next int
	if !oneItem {
		next = 1
	}
	i := *ind + next
	key := ``
	ret := types.NewMap()
	state := mustKey
main:
	for ; i < len(*lexems); i++ {
		lexem := (*lexems)[i]
		switch lexem.Type {
		case lexNewLine:
			continue
		case isRCurly:
			break main
		case isComma, isRBrack:
			if oneItem {
				*ind = i - 1
				return ret, nil
			}
		}
		switch state {
		case mustComma:
			if lexem.Type != isComma {
				return nil, errUnexpComma
			}
			state = mustKey
		case mustColon:
			if lexem.Type != isColon {
				return nil, errUnexpColon
			}
			state = mustValue
		case mustKey:
			switch lexem.Type & 0xff {
			case lexIdent:
				key = lexem.Value.(string)
			case lexExtend:
				key = `$` + lexem.Value.(string)
			case lexString:
				key = lexem.Value.(string)
			case lexKeyword:
				for ikey, v := range keywords {
					if fmt.Sprint(v) == fmt.Sprint(lexem.Value) {
						key = ikey
						if v == keyFunc && i < len(*lexems)-1 && (*lexems)[i+1].Type&0xff == lexIdent {
							continue main
						}
						break
					}
				}
			default:
				return nil, errUnexpKey
			}
			state = mustColon
		case mustValue:
			mapi, err := vm.getInitValue(lexems, &i, block)
			if err != nil {
				return nil, err
			}
			ret.Set(key, mapi)
			state = mustComma
		}
	}
	if ret.IsEmpty() && state == mustKey {
		return nil, errUnexpKey
	}
	if i == len(*lexems) {
		return nil, errUnclosedMap
	}
	*ind = i
	return ret, nil
}

func (vm *VM) getInitArray(lexems *Lexems, ind *int, block *CodeBlocks) ([]mapItem, error) {
	i := *ind + 1
	ret := make([]mapItem, 0)
	state := mustValue
main:
	for ; i < len(*lexems); i++ {
		lexem := (*lexems)[i]
		switch lexem.Type {
		case lexNewLine:
			continue
		case isRBrack:
			break main
		}
		switch state {
		case mustComma:
			if lexem.Type != isComma {
				return nil, errUnexpComma
			}
			state = mustValue
		case mustValue:
			if i+1 < len(*lexems) && (*lexems)[i+1].Type == isColon {
				subMap, err := vm.getInitMap(lexems, &i, block, true)
				if err != nil {
					return nil, err
				}
				ret = append(ret, mapItem{Type: mapMap, Value: subMap})
			} else {
				arri, err := vm.getInitValue(lexems, &i, block)
				if err != nil {
					return nil, err
				}
				ret = append(ret, arri)
			}
			state = mustComma
		}
	}
	if len(ret) > 0 && state == mustValue {
		return nil, errUnexpValue
	}
	if i == len(*lexems) {
		return nil, errUnclosedArray
	}
	*ind = i
	return ret, nil
}

// This function is responsible for the compilation of expressions
func (vm *VM) compileEval(lexems *Lexems, ind *int, block *CodeBlocks) error {
	var indexInfo *IndexInfo

	i := *ind
	curBlock := (*block)[len(*block)-1]

	buffer := make(ByteCodes, 0, 20)
	bytecode := make(ByteCodes, 0, 100)
	parcount := make([]int, 0, 20)
	setIndex := false
	noMap := false
	prevLex := uint32(0)
main:
	for ; i < len(*lexems); i++ {
		var cmd *ByteCode
		var call bool
		lexem := (*lexems)[i]
		logger := lexem.GetLogger()
		if !noMap {
			if lexem.Type == isLCurly {
				pMap, err := vm.getInitMap(lexems, &i, block, false)
				if err != nil {
					return err
				}
				bytecode.push(newByteCode(cmdMapInit, lexem.Line, pMap))
				continue
			}
			if lexem.Type == isLBrack {
				pArray, err := vm.getInitArray(lexems, &i, block)
				if err != nil {
					return err
				}
				bytecode.push(newByteCode(cmdArrayInit, lexem.Line, pArray))
				continue
			}
		}
		noMap = false

		switch lexem.Type {
		case isRCurly, isLCurly:
			i--
			if prevLex == isComma || prevLex == lexOper {
				return errEndExp
			}
			break main
		case lexNewLine:
			if i > 0 && ((*lexems)[i-1].Type == isComma || (*lexems)[i-1].Type == lexOper) {
				continue main
			}
			for k := len(buffer) - 1; k >= 0; k-- {
				if buffer[k].Cmd == cmdSys {
					continue main
				}
			}
			break main
		case isLPar:
			buffer.push(newByteCode(cmdSys, lexem.Line, uint16(0xff)))
		case isLBrack:
			buffer.push(newByteCode(cmdSys, lexem.Line, uint16(0xff)))
		case isComma:
			if len(parcount) > 0 {
				parcount[len(parcount)-1]++
			}
			for len(buffer) > 0 {
				prev := buffer[len(buffer)-1]
				if prev.Cmd == cmdSys && prev.Value.(uint16) == 0xff {
					break
				} else {
					bytecode.push(prev)
					buffer = buffer[:len(buffer)-1]
				}
			}
		case isRPar:
			noMap = true
			for {
				if len(buffer) == 0 {
					logger.WithFields(log.Fields{"lex_value": lexem.Value.(string), "type": consts.ParseError}).Error("there is not pair")
					return fmt.Errorf(`there is not pair`)
				}
				prev := buffer[len(buffer)-1]
				buffer = buffer[:len(buffer)-1]
				if prev.Value.(uint16) == 0xff {
					break
				} else {
					bytecode.push(prev)
				}
			}
			if len(buffer) > 0 {
				if prev := buffer[len(buffer)-1]; prev.Cmd == cmdFuncName {
					buffer = buffer[:len(buffer)-1]
					(*prev).Value = FuncNameCmd{Name: prev.Value.(FuncNameCmd).Name,
						Count: parcount[len(parcount)-1]}
					parcount = parcount[:len(parcount)-1]
					bytecode.push(prev)
				}
				var tail *ByteCode
				if prev := buffer[len(buffer)-1]; prev.Cmd == cmdCall || prev.Cmd == cmdCallVari {
					objInfo := prev.Value.(*ObjInfo)
					if (objInfo.Type == ObjectType_Func && objInfo.Value.CodeBlock().Info.FuncInfo().CanWrite) ||
						(objInfo.Type == ObjectType_ExtFunc && objInfo.Value.ExtFuncInfo().CanWrite) {
						setWritable(block)
					}
					if objInfo.Type == ObjectType_Func && objInfo.Value.CodeBlock().Info.FuncInfo().Names != nil {
						if len(bytecode) == 0 || bytecode[len(bytecode)-1].Cmd != cmdFuncName {
							bytecode.push(newByteCode(cmdPush, lexem.Line, nil))
						}
						if i < len(*lexems)-4 && (*lexems)[i+1].Type == isDot {
							if (*lexems)[i+2].Type != lexIdent {
								log.WithFields(log.Fields{"type": consts.ParseError}).Error("must be the name of the tail")
								return fmt.Errorf(`must be the name of the tail`)
							}
							names := prev.Value.(*ObjInfo).Value.CodeBlock().Info.FuncInfo().Names
							if _, ok := (*names)[(*lexems)[i+2].Value.(string)]; !ok {

								if i < len(*lexems)-5 && (*lexems)[i+3].Type == isLPar {
									objInfo, _ := vm.findObj((*lexems)[i+2].Value.(string), block)
									if objInfo != nil && objInfo.Type == ObjectType_Func || objInfo.Type == ObjectType_ExtFunc {
										tail = newByteCode(uint16(cmdCall), lexem.Line, objInfo)
									}
								}
								if tail == nil {
									log.WithFields(log.Fields{"type": consts.ParseError, "tail": (*lexems)[i+2].Value.(string)}).Error("unknown function tail")
									return fmt.Errorf(`unknown function tail %s`, (*lexems)[i+2].Value.(string))
								}
							}
							if tail == nil {
								buffer.push(newByteCode(cmdFuncName, lexem.Line, FuncNameCmd{Name: (*lexems)[i+2].Value.(string)}))
								count := 0
								if (*lexems)[i+3].Type != isRPar {
									count++
								}
								parcount = append(parcount, count)
								i += 2
								break
							}
						}
					}
					count := parcount[len(parcount)-1]
					parcount = parcount[:len(parcount)-1]
					if prev.Value.(*ObjInfo).Type == ObjectType_ExtFunc {
						var errtext string
						extinfo := prev.Value.(*ObjInfo).Value.ExtFuncInfo()
						wantlen := len(extinfo.Params)
						for _, v := range extinfo.Auto {
							if len(v) > 0 {
								wantlen--
							}
						}
						if count != wantlen && (!extinfo.Variadic || count < wantlen) {
							errtext = fmt.Sprintf(eWrongParams, extinfo.Name, wantlen)
							logger.WithFields(log.Fields{"error": errtext, "type": consts.ParseError}).Error(errtext)
							return fmt.Errorf(errtext)
						}
					}
					if prev.Cmd == cmdCallVari {
						bytecode.push(newByteCode(cmdPush, lexem.Line, count))
					}
					buffer = buffer[:len(buffer)-1]
					bytecode.push(prev)
					if tail != nil {
						buffer.push(tail)
						parcount = append(parcount, 1)
						i += 2
					}
				}
			}
		case isRBrack:
			noMap = true
			for {
				if len(buffer) == 0 {
					logger.WithFields(log.Fields{"lex_value": lexem.Value.(string), "type": consts.ParseError}).Error("there is not pair")
					return fmt.Errorf(`there is not pair`)
				}
				prev := buffer[len(buffer)-1]
				buffer = buffer[:len(buffer)-1]
				if prev.Value.(uint16) == 0xff {
					break
				} else {
					bytecode.push(prev)
				}
			}
			if len(buffer) > 0 {
				if prev := buffer[len(buffer)-1]; prev.Cmd == cmdIndex {
					buffer = buffer[:len(buffer)-1]
					if i < len(*lexems)-1 && (*lexems)[i+1].Type == isEq {
						i++
						setIndex = true
						indexInfo = prev.Value.(*IndexInfo)
						noMap = false
						continue
					}
					bytecode.push(prev)
				}
			}
			if (*lexems)[i+1].Type == isLBrack {
				return errMultiIndex
			}
		case lexOper:
			if oper, ok := opers[lexem.Value.(uint32)]; ok {
				var prevType uint32
				if i > 0 {
					prevType = (*lexems)[i-1].Type
				}
				if oper.Cmd == cmdSub && (i == 0 || (prevType != lexNumber && prevType != lexIdent &&
					prevType != lexExtend && prevType != lexString && prevType != isRCurly &&
					prevType != isRBrack && prevType != isRPar)) {
					oper.Cmd = cmdSign
					oper.Priority = cmdUnary
				} else if prevLex == lexOper && oper.Priority != cmdUnary {
					return errOper
				}
				byteOper := newByteCode(oper.Cmd, lexem.Line, oper.Priority)

				for {
					if len(buffer) == 0 {
						buffer.push(byteOper)
						break
					} else {
						prev := buffer[len(buffer)-1]
						if prev.Value.(uint16) >= oper.Priority && oper.Priority != cmdUnary && prev.Cmd != cmdSys {
							if prev.Value.(uint16) == cmdUnary { // Right to left
								unar := len(buffer) - 1
								for ; unar > 0 && buffer[unar-1].Value.(uint16) == cmdUnary; unar-- {
								}
								bytecode = append(bytecode, buffer[unar:]...)
								buffer = buffer[:unar]
							} else {
								bytecode.push(prev)
								buffer = buffer[:len(buffer)-1]
							}
						} else {
							buffer.push(byteOper)
							break
						}
					}
				}
			} else {
				logger.WithFields(log.Fields{"lex_value": strconv.FormatUint(uint64(lexem.Value.(uint32)), 10), "type": consts.ParseError}).Error("unknown operator")
				return fmt.Errorf(`unknown operator %d`, lexem.Value.(uint32))
			}
		case lexNumber, lexString:
			noMap = true
			cmd = newByteCode(cmdPush, lexem.Line, lexem.Value)
		case lexExtend:
			noMap = true
			if i < len(*lexems)-2 {
				if (*lexems)[i+1].Type == isLPar {
					count := 0
					if (*lexems)[i+2].Type != isRPar {
						count++
					}
					parcount = append(parcount, count)
					buffer.push(newByteCode(cmdCallExtend, lexem.Line, lexem.Value.(string)))
					call = true
				}
			}
			if !call {
				cmd = newByteCode(cmdExtend, lexem.Line, lexem.Value.(string))
				if i < len(*lexems)-1 && (*lexems)[i+1].Type == isLBrack {
					buffer.push(newByteCode(cmdIndex, lexem.Line, &IndexInfo{Extend: lexem.Value.(string)}))
				}
			}
		case lexIdent:
			noMap = true
			objInfo, tobj := vm.findObj(lexem.Value.(string), block)
			if objInfo == nil && (!vm.Extern || i > *ind || i >= len(*lexems)-2 || (*lexems)[i+1].Type != isLPar) {
				logger.WithFields(log.Fields{"lex_value": lexem.Value.(string), "type": consts.ParseError}).Error("unknown identifier")
				return fmt.Errorf(eUnknownIdent, lexem.Value.(string))
			}
			if i < len(*lexems)-2 {
				if (*lexems)[i+1].Type == isLPar {
					var (
						isContract  bool
						objContract *CodeBlock
					)
					if vm.Extern && objInfo == nil {
						objInfo = &ObjInfo{Type: ObjectType_Contract}
					}
					if objInfo == nil || (objInfo.Type != ObjectType_ExtFunc && objInfo.Type != ObjectType_Func &&
						objInfo.Type != ObjectType_Contract) {
						logger.WithFields(log.Fields{"lex_value": lexem.Value.(string), "type": consts.ParseError}).Error("unknown function")
						return fmt.Errorf(`unknown function %s`, lexem.Value.(string))
					}
					if objInfo.Type == ObjectType_Contract {
						if objInfo.Value != nil {
							objContract = objInfo.Value.CodeBlock()
						}
						objInfo, tobj = vm.findObj(`ExecContract`, block)
						isContract = true
					}
					cmdCall := uint16(cmdCall)
					if (objInfo.Type == ObjectType_ExtFunc && objInfo.Value.ExtFuncInfo().Variadic) ||
						(objInfo.Type == ObjectType_Func && objInfo.Value.CodeBlock().Info.FuncInfo().Variadic) {
						cmdCall = cmdCallVari
					}
					count := 0
					if (*lexems)[i+2].Type != isRPar {
						count++
					}
					buffer.push(newByteCode(cmdCall, lexem.Line, objInfo))
					if isContract {
						name := StateName((*block)[0].Info.Uint32(), lexem.Value.(string))
						for j := len(*block) - 1; j >= 0; j-- {
							topblock := (*block)[j]
							if topblock.Type == ObjectType_Contract {
								if name == topblock.Info.ContractInfo().Name {
									return errRecursion
								}
								if topblock.Info.ContractInfo().Used == nil {
									topblock.Info.ContractInfo().Used = make(map[string]bool)
								}
								topblock.Info.ContractInfo().Used[name] = true
							}
						}
						if objContract != nil && objContract.Info.ContractInfo().CanWrite {
							setWritable(block)
						}
						bytecode.push(newByteCode(cmdPush, lexem.Line, name))
						if count == 0 {
							count = 2
							bytecode.push(newByteCode(cmdPush, lexem.Line, ""))
							bytecode.push(newByteCode(cmdPush, lexem.Line, ""))
						}
						count++
					}
					if lexem.Value.(string) == `CallContract` {
						count++
						bytecode.push(newByteCode(cmdPush, lexem.Line, (*block)[0].Info.Uint32()))
					}
					parcount = append(parcount, count)
					call = true
				}
				if (*lexems)[i+1].Type == isLBrack {
					if objInfo == nil || objInfo.Type != ObjectType_Var {
						logger.WithFields(log.Fields{"lex_value": lexem.Value.(string), "type": consts.ParseError}).Error("unknown variable")
						return fmt.Errorf(`unknown variable %s`, lexem.Value.(string))
					}
					buffer.push(newByteCode(cmdIndex, lexem.Line, &IndexInfo{VarOffset: objInfo.Value.Int(), Owner: tobj}))
				}
			}
			if !call {
				if objInfo.Type != ObjectType_Var {
					return fmt.Errorf(`unknown variable %s`, lexem.Value.(string))
				}
				cmd = newByteCode(cmdVar, lexem.Line, &VarInfo{Obj: objInfo, Owner: tobj})
			}
		}
		if lexem.Type != lexNewLine {
			prevLex = lexem.Type
		}
		if lexem.Type&0xff == lexKeyword {
			if lexem.Value.(uint32) == keyTail {
				cmd = newByteCode(cmdUnwrapArr, lexem.Line, 0)
			}
		}
		if cmd != nil {
			bytecode.push(cmd)
		}
	}
	*ind = i
	if prevLex == lexOper {
		return errEndExp
	}
	for i := len(buffer) - 1; i >= 0; i-- {
		if buffer[i].Cmd == cmdSys {
			log.WithFields(log.Fields{"type": consts.ParseError}).Error("there is not pair")
			return fmt.Errorf(`there is not pair`)
		}
		bytecode.push(buffer[i])
	}
	if setIndex {
		bytecode.push(newByteCode(cmdSetIndex, 0, indexInfo))
	}
	curBlock.Code = append(curBlock.Code, bytecode...)
	return nil
}

// ContractsList returns list of contracts names from source of code
func ContractsList(value string) ([]string, error) {
	names := make([]string, 0)
	lexems, err := lexParser([]rune(value))
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ParseError, "error": err}).Error("getting contract list")
		return names, err
	}
	var level int
	for i, lexem := range lexems {
		switch lexem.Type {
		case isLCurly:
			level++
		case isRCurly:
			level--
		case lexKeyword | (keyContract << 8), lexKeyword | (keyFunc << 8):
			if level == 0 && i+1 < len(lexems) && lexems[i+1].Type == lexIdent {
				names = append(names, lexems[i+1].Value.(string))
			}
		}
	}

	return names, nil
}
