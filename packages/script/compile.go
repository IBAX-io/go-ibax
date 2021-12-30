/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import (
	"fmt"
	"reflect"
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
	NewState int // a new state
	Func     int // a handle function
}

type stateLine map[int]compileState

// The list of compile states
type compileStates []stateLine

type compileFunc func(*Blocks, int, *Lexem) error

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

/* Byte code could be described as a tree where functions and contracts are on the top level and
nesting goes further according to nesting of bracketed brackets. Tree nodes are structures of
'Block' type. For instance,
 func a {
	 if b {
		 while d {

		 }
	 }
	 if c {
	 }
 }
 will be compiled into Block(a) which will have two child blocks Block (b) and Block (c) that
 are responsible for executing bytecode inside if. Block (b) will have a child Block (d) with
 a cycle.
*/

const (
	// The list of state types
	stateRoot = iota
	stateBody
	stateBlock
	stateContract
	stateFunc
	stateFParams
	stateFParam
	stateFParamTYPE
	stateFTail
	stateFResult
	stateFDot
	stateVar
	stateVarType
	stateAssignEval
	stateAssign
	stateTX
	stateSettings
	stateConsts
	stateConstsAssign
	stateConstsValue
	stateFields
	stateEval

	// The list of state flags
	statePush     = 0x0100
	statePop      = 0x0200
	stateStay     = 0x0400
	stateToBlock  = 0x0800
	stateToBody   = 0x1000
	stateFork     = 0x2000
	stateToFork   = 0x4000
	stateLabel    = 0x8000
	stateMustEval = 0x010000

	flushMark = 0x100000
)

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

const (
	// This is a list of identifiers for functions that will generate a bytecode for
	// the corresponding cases
	// Indexes of handle functions funcs = CompileFunc[]
	//	cfNothing = iota
	cfError = iota + 1
	cfNameBlock
	cfFResult
	cfReturn
	cfIf
	cfElse
	cfFParam
	cfFType
	cfFTail
	cfFNameParam
	cfAssignVar
	cfAssign
	cfTX
	cfSettings
	cfConstName
	cfConstValue
	cfField
	cfFieldType
	cfFieldTag
	cfFields
	cfFieldComma
	cfFieldLine
	cfWhile
	cfContinue
	cfBreak
	cfCmdError

	//	cfEval
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
	// The array of functions corresponding to the constants cf...
	funcs = []compileFunc{nil,
		fError,
		fNameBlock,
		fFuncResult,
		fReturn,
		fIf,
		fElse,
		fFparam,
		fFtype,
		fFtail,
		fFNameParam,
		fAssignVar,
		fAssign,
		fTx,
		fSettings,
		fConstName,
		fConstValue,
		fField,
		fFieldType,
		fFieldTag,
		fFields,
		fFieldComma,
		fFieldLine,
		fWhile,
		fContinue,
		fBreak,
		fCmdError,
	}

	// 'states' describes a finite machine with states on the base of which a bytecode will be generated
	states = compileStates{
		{ // stateRoot
			lexNewLine:                      {NewState: stateRoot, Func: 0},
			lexKeyword | (keyContract << 8): {NewState: stateContract | statePush, Func: 0},
			lexKeyword | (keyFunc << 8):     {NewState: stateFunc | statePush, Func: 0},
			0:                               {NewState: errUnknownCmd, Func: cfError},
		},
		{ // stateBody
			lexNewLine:                      {NewState: stateBody, Func: 0},
			lexKeyword | (keyFunc << 8):     {NewState: stateFunc | statePush, Func: 0},
			lexKeyword | (keyReturn << 8):   {NewState: stateEval, Func: cfReturn},
			lexKeyword | (keyContinue << 8): {NewState: stateBody, Func: cfContinue},
			lexKeyword | (keyBreak << 8):    {NewState: stateBody, Func: cfBreak},
			lexKeyword | (keyIf << 8):       {NewState: stateEval | statePush | stateToBlock | stateMustEval, Func: cfIf},
			lexKeyword | (keyWhile << 8):    {NewState: stateEval | statePush | stateToBlock | stateLabel | stateMustEval, Func: cfWhile},
			lexKeyword | (keyElse << 8):     {NewState: stateBlock | statePush, Func: cfElse},
			lexKeyword | (keyVar << 8):      {NewState: stateVar, Func: 0},
			lexKeyword | (keyTX << 8):       {NewState: stateTX, Func: cfTX},
			lexKeyword | (keySettings << 8): {NewState: stateSettings, Func: cfSettings},
			lexKeyword | (keyError << 8):    {NewState: stateEval, Func: cfCmdError},
			lexKeyword | (keyWarning << 8):  {NewState: stateEval, Func: cfCmdError},
			lexKeyword | (keyInfo << 8):     {NewState: stateEval, Func: cfCmdError},
			lexIdent:                        {NewState: stateAssignEval | stateFork, Func: 0},
			lexExtend:                       {NewState: stateAssignEval | stateFork, Func: 0},
			isRCurly:                        {NewState: statePop, Func: 0},
			0:                               {NewState: errMustRCurly, Func: cfError},
		},
		{ // stateBlock
			lexNewLine: {NewState: stateBlock, Func: 0},
			isLCurly:   {NewState: stateBody, Func: 0},
			0:          {NewState: errMustLCurly, Func: cfError},
		},
		{ // stateContract
			lexNewLine: {NewState: stateContract, Func: 0},
			lexIdent:   {NewState: stateBlock, Func: cfNameBlock},
			0:          {NewState: errMustName, Func: cfError},
		},
		{ // stateFunc
			lexNewLine: {NewState: stateFunc, Func: 0},
			lexIdent:   {NewState: stateFParams, Func: cfNameBlock},
			0:          {NewState: errMustName, Func: cfError},
		},
		{ // stateFParams
			lexNewLine: {NewState: stateFParams, Func: 0},
			isLPar:     {NewState: stateFParam, Func: 0},
			0:          {NewState: stateFResult | stateStay, Func: 0},
		},
		{ // stateFParam
			lexNewLine: {NewState: stateFParam, Func: 0},
			lexIdent:   {NewState: stateFParamTYPE, Func: cfFParam},
			// lexType:  {NewState: stateFParam, Func: cfFType},
			isComma: {NewState: stateFParam, Func: 0},
			isRPar:  {NewState: stateFResult, Func: 0},
			0:       {NewState: errParams, Func: cfError},
		},
		{ // stateFParamTYPE
			lexIdent:                    {NewState: stateFParamTYPE, Func: cfFParam},
			lexType:                     {NewState: stateFParam, Func: cfFType},
			lexKeyword | (keyTail << 8): {NewState: stateFTail, Func: cfFTail},
			isComma:                     {NewState: stateFParamTYPE, Func: 0},
			//			isRPar:  {NewState: stateFResult, Func: 0},
			0: {NewState: errVarType, Func: cfError},
		},
		{ // stateFTail
			lexNewLine: {NewState: stateFTail, Func: 0},
			isRPar:     {NewState: stateFResult, Func: 0},
			0:          {NewState: errParams, Func: cfError},
		},
		{ // stateFResult
			lexNewLine: {NewState: stateFResult, Func: 0},
			isDot:      {NewState: stateFDot, Func: 0},
			lexType:    {NewState: stateFResult, Func: cfFResult},
			isComma:    {NewState: stateFResult, Func: 0},
			0:          {NewState: stateBlock | stateStay, Func: 0},
		},
		{ // stateFDot
			lexNewLine: {NewState: stateFDot, Func: 0},
			lexIdent:   {NewState: stateFParams, Func: cfFNameParam},
			0:          {NewState: errMustName, Func: cfError},
		},
		{ // stateVar
			lexNewLine: {NewState: stateBody, Func: 0},
			lexIdent:   {NewState: stateVarType, Func: cfFParam},
			isRCurly:   {NewState: stateBody | stateStay, Func: 0},
			isComma:    {NewState: stateVar, Func: 0},
			0:          {NewState: errVars, Func: cfError},
		},
		{ // stateVarType
			lexIdent: {NewState: stateVarType, Func: cfFParam},
			lexType:  {NewState: stateVar, Func: cfFType},
			isComma:  {NewState: stateVarType, Func: 0},
			0:        {NewState: errVarType, Func: cfError},
		},
		{ // stateAssignEval
			isLPar:   {NewState: stateEval | stateToFork | stateToBody, Func: 0},
			isLBrack: {NewState: stateEval | stateToFork | stateToBody, Func: 0},
			0:        {NewState: stateAssign | stateToFork | stateStay, Func: 0},
		},
		{ // stateAssign
			isComma:   {NewState: stateAssign, Func: 0},
			lexIdent:  {NewState: stateAssign, Func: cfAssignVar},
			lexExtend: {NewState: stateAssign, Func: cfAssignVar},
			isEq:      {NewState: stateEval | stateToBody, Func: cfAssign},
			0:         {NewState: errAssign, Func: cfError},
		},
		{ // stateTX
			lexNewLine: {NewState: stateTX, Func: 0},
			isLCurly:   {NewState: stateFields, Func: 0},
			0:          {NewState: errMustLCurly, Func: cfError},
		},
		{ // stateSettings
			lexNewLine: {NewState: stateSettings, Func: 0},
			isLCurly:   {NewState: stateConsts, Func: 0},
			0:          {NewState: errMustLCurly, Func: cfError},
		},
		{ // stateConsts
			lexNewLine: {NewState: stateConsts, Func: 0},
			isComma:    {NewState: stateConsts, Func: 0},
			lexIdent:   {NewState: stateConstsAssign, Func: cfConstName},
			isRCurly:   {NewState: stateToBody, Func: 0},
			0:          {NewState: errMustRCurly, Func: cfError},
		},
		{ // stateConstsAssign
			isEq: {NewState: stateConstsValue, Func: 0},
			0:    {NewState: errAssign, Func: cfError},
		},
		{ // stateConstsValue
			lexString: {NewState: stateConsts, Func: cfConstValue},
			lexNumber: {NewState: stateConsts, Func: cfConstValue},
			0:         {NewState: errStrNum, Func: cfError},
		},
		{ // stateFields
			lexNewLine: {NewState: stateFields, Func: cfFieldLine},
			isComma:    {NewState: stateFields, Func: cfFieldComma},
			lexIdent:   {NewState: stateFields, Func: cfField},
			lexType:    {NewState: stateFields, Func: cfFieldType},
			lexString:  {NewState: stateFields, Func: cfFieldTag},
			isRCurly:   {NewState: stateToBody, Func: cfFields},
			0:          {NewState: errMustRCurly, Func: cfError},
		},
	}
)

func fError(buf *Blocks, state int, lexem *Lexem) error {
	errors := []string{`no error`,
		`unknown command`,          // errUnknownCmd
		`must be the name`,         // errMustName
		`must be '{'`,              // errMustLCurly
		`must be '}'`,              // errMustRCurly
		`wrong parameters`,         // errParams
		`wrong variables`,          // errVars
		`must be type`,             // errVarType
		`must be '='`,              // errAssign
		`must be number or string`, // errStrNum
	}
	logger := lexem.GetLogger()
	if lexem.Type == lexNewLine {
		logger.WithFields(log.Fields{"error": errors[state], "lex_value": lexem.Value, "type": consts.ParseError}).Error("unexpected new line")
		return fmt.Errorf(`%s (unexpected new line) [Ln:%d]`, errors[state], lexem.Line-1)
	}
	logger.WithFields(log.Fields{"error": errors[state], "lex_value": lexem.Value, "type": consts.ParseError}).Error("parsing error")
	return fmt.Errorf(`%s %x %v [Ln:%d Col:%d]`, errors[state], lexem.Type, lexem.Value, lexem.Line, lexem.Column)
}

func fFuncResult(buf *Blocks, state int, lexem *Lexem) error {
	fblock := buf.peek().Info.(*FuncInfo)
	(*fblock).Results = append((*fblock).Results, lexem.Value.(reflect.Type))
	return nil
}

func fReturn(buf *Blocks, state int, lexem *Lexem) error {
	buf.peek().Code.push(newByteCode(cmdReturn, lexem.Line, 0))
	return nil
}

func fCmdError(buf *Blocks, state int, lexem *Lexem) error {
	buf.peek().Code.push(newByteCode(cmdError, lexem.Line, lexem.Value))
	return nil
}

func fFparam(buf *Blocks, state int, lexem *Lexem) error {
	block := buf.peek()
	if block.Type == ObjectType_Func && (state == stateFParam || state == stateFParamTYPE) {
		fblock := block.Info.(*FuncInfo)
		if fblock.Names == nil {
			fblock.Params = append(fblock.Params, reflect.TypeOf(nil))
		} else {
			for key := range *fblock.Names {
				if key[0] == '_' {
					name := key[1:]
					params := append((*fblock.Names)[name].Params, reflect.TypeOf(nil))
					offset := append((*fblock.Names)[name].Offset, len(block.Vars))
					(*fblock.Names)[name] = FuncName{Params: params, Offset: offset}
					break
				}
			}
		}
	}
	if block.Objects == nil {
		block.Objects = make(map[string]*ObjInfo)
	}
	block.Objects[lexem.Value.(string)] = &ObjInfo{Type: ObjectType_Var, Value: len(block.Vars)}
	block.Vars = append(block.Vars, reflect.TypeOf(nil))
	return nil
}

func fFtype(buf *Blocks, state int, lexem *Lexem) error {
	block := buf.peek()
	if block.Type == ObjectType_Func && state == stateFParam {
		fblock := block.Info.(*FuncInfo)
		if fblock.Names == nil {
			for pkey, param := range fblock.Params {
				if param == reflect.TypeOf(nil) {
					fblock.Params[pkey] = lexem.Value.(reflect.Type)
				}
			}
		} else {
			for key := range *fblock.Names {
				if key[0] == '_' {
					for pkey, param := range (*fblock.Names)[key[1:]].Params {
						if param == reflect.TypeOf(nil) {
							(*fblock.Names)[key[1:]].Params[pkey] = lexem.Value.(reflect.Type)
						}
					}
					break
				}
			}
		}
	}
	for vkey, ivar := range block.Vars {
		if ivar == reflect.TypeOf(nil) {
			block.Vars[vkey] = lexem.Value.(reflect.Type)
		}
	}
	return nil
}

func fFtail(buf *Blocks, state int, lexem *Lexem) error {
	var used bool
	block := buf.peek()

	fblock := block.Info.(*FuncInfo)
	if fblock.Names == nil {
		for pkey, param := range fblock.Params {
			if param == reflect.TypeOf(nil) {
				if used {
					return fmt.Errorf(`... parameter must be one`)
				}
				fblock.Params[pkey] = reflect.TypeOf([]interface{}{})
				used = true
			}
		}
		block.Info.(*FuncInfo).Variadic = true
	} else {
		for key := range *fblock.Names {
			if key[0] == '_' {
				name := key[1:]
				for pkey, param := range (*fblock.Names)[name].Params {
					if param == reflect.TypeOf(nil) {
						if used {
							return fmt.Errorf(`... parameter must be one`)
						}
						(*fblock.Names)[name].Params[pkey] = reflect.TypeOf([]interface{}{})
						used = true
					}
				}
				offset := append((*fblock.Names)[name].Offset, len(block.Vars))
				(*fblock.Names)[name] = FuncName{Params: (*fblock.Names)[name].Params,
					Offset: offset, Variadic: true}
				break
			}
		}
	}
	for vkey, ivar := range block.Vars {
		if ivar == reflect.TypeOf(nil) {
			block.Vars[vkey] = reflect.TypeOf([]interface{}{})
		}
	}
	return nil
}

func fFNameParam(buf *Blocks, state int, lexem *Lexem) error {
	block := buf.peek()

	fblock := block.Info.(*FuncInfo)
	if fblock.Names == nil {
		names := make(map[string]FuncName)
		fblock.Names = &names
	}
	for key := range *fblock.Names {
		if key[0] == '_' {
			delete(*fblock.Names, key)
		}
	}
	(*fblock.Names)[`_`+lexem.Value.(string)] = FuncName{}

	return nil
}

func fIf(buf *Blocks, state int, lexem *Lexem) error {
	buf.get(len(*buf) - 2).Code.push(newByteCode(cmdIf, lexem.Line, buf.peek()))
	return nil
}

func fWhile(buf *Blocks, state int, lexem *Lexem) error {
	buf.get(len(*buf) - 2).Code.push(newByteCode(cmdWhile, lexem.Line, buf.peek()))
	buf.get(len(*buf) - 2).Code.push(newByteCode(cmdContinue, lexem.Line, 0))
	return nil
}

func fContinue(buf *Blocks, state int, lexem *Lexem) error {
	buf.peek().Code.push(newByteCode(cmdContinue, lexem.Line, 0))
	return nil
}

func fBreak(buf *Blocks, state int, lexem *Lexem) error {
	buf.peek().Code.push(newByteCode(cmdBreak, lexem.Line, 0))
	return nil
}

func fAssignVar(buf *Blocks, state int, lexem *Lexem) error {
	block := buf.peek()
	var (
		prev []*VarInfo
		ivar VarInfo
	)
	if lexem.Type == lexExtend {
		if isSysVar(lexem.Value.(string)) {
			lexem.GetLogger().WithFields(log.Fields{"type": consts.ParseError, "lex_value": lexem.Value.(string)}).Error("modifying system variable")
			return fmt.Errorf(eSysVar, lexem.Value.(string))
		}
		ivar = VarInfo{Obj: &ObjInfo{Type: ObjectType_Extend, Value: lexem.Value.(string)}, Owner: nil}
	} else {
		objInfo, tobj := findVar(lexem.Value.(string), buf)
		if objInfo == nil || objInfo.Type != ObjectType_Var {
			logger := lexem.GetLogger()
			logger.WithFields(log.Fields{"type": consts.ParseError, "lex_value": lexem.Value.(string)}).Error("unknown variable")
			return fmt.Errorf(`unknown variable %s`, lexem.Value.(string))
		}
		ivar = VarInfo{Obj: objInfo, Owner: tobj}
	}
	if len(block.Code) > 0 {
		if block.Code[len(block.Code)-1].Cmd == cmdAssignVar {
			prev = block.Code[len(block.Code)-1].Value.([]*VarInfo)
		}
	}
	prev = append(prev, &ivar)
	if len(prev) == 1 {
		block.Code.push(newByteCode(cmdAssignVar, lexem.Line, prev))
	} else {
		block.Code[len(block.Code)-1] = newByteCode(cmdAssignVar, lexem.Line, prev)
	}
	return nil
}

func fAssign(buf *Blocks, state int, lexem *Lexem) error {
	buf.peek().Code.push(newByteCode(cmdAssign, lexem.Line, 0))
	return nil
}

func fTx(buf *Blocks, state int, lexem *Lexem) error {
	contract := buf.peek()
	logger := lexem.GetLogger()
	if contract.Type != ObjectType_Contract {
		logger.WithFields(log.Fields{"type": consts.ParseError, "contract_type": contract.Type, "lex_value": lexem.Value}).Error("data can only be in contract")
		return fmt.Errorf(`data can only be in contract`)
	}
	(*contract).Info.(*ContractInfo).Tx = new([]*FieldInfo)
	return nil
}

func fSettings(buf *Blocks, state int, lexem *Lexem) error {
	contract := buf.peek()
	if contract.Type != ObjectType_Contract {
		logger := lexem.GetLogger()
		logger.WithFields(log.Fields{"type": consts.ParseError, "contract_type": contract.Type, "lex_value": lexem.Value}).Error("data can only be in contract")
		return fmt.Errorf(`data can only be in contract`)
	}
	(*contract).Info.(*ContractInfo).Settings = make(map[string]interface{})
	return nil
}

func fConstName(buf *Blocks, state int, lexem *Lexem) error {
	sets := buf.peek().Info.(*ContractInfo).Settings
	sets[lexem.Value.(string)] = nil
	return nil
}

func fConstValue(buf *Blocks, state int, lexem *Lexem) error {
	sets := buf.peek().Info.(*ContractInfo).Settings
	for key, val := range sets {
		if val == nil {
			sets[key] = lexem.Value
			break
		}
	}
	return nil
}

func fField(buf *Blocks, state int, lexem *Lexem) error {
	tx := buf.peek().Info.(*ContractInfo).Tx
	if len(*tx) > 0 && (*tx)[len(*tx)-1].Type == reflect.TypeOf(nil) &&
		(*tx)[len(*tx)-1].Tags != `_` {
		return fmt.Errorf(eDataType, lexem.Line, lexem.Column)
	}
	*tx = append(*tx, &FieldInfo{Name: lexem.Value.(string), Type: reflect.TypeOf(nil)})
	return nil
}

func fFields(buf *Blocks, state int, lexem *Lexem) error {
	tx := buf.peek().Info.(*ContractInfo).Tx
	if len(*tx) > 0 && (*tx)[len(*tx)-1].Type == nil {
		return fmt.Errorf(eDataType, lexem.Line, lexem.Column)
	}
	return nil
}

func fFieldComma(buf *Blocks, state int, lexem *Lexem) error {
	tx := buf.peek().Info.(*ContractInfo).Tx
	if len(*tx) == 0 || (*tx)[len(*tx)-1].Type != nil {
		return fmt.Errorf(eDataName, lexem.Line, lexem.Column)
	}
	(*tx)[len(*tx)-1].Tags = `_`
	return nil
}

func fFieldLine(buf *Blocks, state int, lexem *Lexem) error {
	tx := buf.peek().Info.(*ContractInfo).Tx
	if len(*tx) > 0 && (*tx)[len(*tx)-1].Type == nil {
		return fmt.Errorf(eDataType, lexem.Line, lexem.Column)
	}
	for i, field := range *tx {
		if field.Tags == `_` {
			(*tx)[i].Tags = ``
		}
	}
	return nil
}

func fFieldType(buf *Blocks, state int, lexem *Lexem) error {
	tx := buf.peek().Info.(*ContractInfo).Tx
	if len(*tx) == 0 || (*tx)[len(*tx)-1].Type != nil {
		return fmt.Errorf(eDataName, lexem.Line, lexem.Column)
	}
	for i, field := range *tx {
		if field.Type == reflect.TypeOf(nil) {
			(*tx)[i].Type = lexem.Value.(reflect.Type)
			(*tx)[i].Original = lexem.Ext
		}
	}
	return nil
}

func fFieldTag(buf *Blocks, state int, lexem *Lexem) error {
	tx := buf.peek().Info.(*ContractInfo).Tx
	if len(*tx) == 0 || (*tx)[len(*tx)-1].Type == nil || len((*tx)[len(*tx)-1].Tags) != 0 {
		return fmt.Errorf(eDataTag, lexem.Line, lexem.Column)
	}
	for i := len(*tx) - 1; i >= 0; i-- {
		if i == len(*tx)-1 || (*tx)[i].Tags == `_` {
			(*tx)[i].Tags = lexem.Value.(string)
			continue
		}
		break
	}
	return nil
}

func fElse(buf *Blocks, state int, lexem *Lexem) error {
	code := buf.get(len(*buf) - 2).Code
	if code.peek().Cmd != cmdIf {
		return fmt.Errorf(`there is not if before %v [Ln:%d Col:%d]`, lexem.Type, lexem.Line, lexem.Column)
	}
	code.push(newByteCode(cmdElse, lexem.Line, buf.peek()))
	return nil
}

// StateName checks the name of the contract and modifies it to @[state]name if it is necessary.
func StateName(state uint32, name string) string {
	if !strings.HasPrefix(name, `@`) {
		return fmt.Sprintf(`@%d%s`, state, name)
	} else if len(name) > 1 && (name[1] < '0' || name[1] > '9') {
		name = `@1` + name[1:]
	}
	return name
}

func fNameBlock(buf *Blocks, state int, lexem *Lexem) error {
	var itype ObjectType

	prev := (*buf)[len(*buf)-2]
	fblock := buf.peek()
	name := lexem.Value.(string)
	switch state {
	case stateBlock:
		itype = ObjectType_Contract
		name = StateName((*buf)[0].Info.(uint32), name)
		fblock.Info = &ContractInfo{ID: uint32(len(prev.Children) - 1), Name: name,
			Owner: (*buf)[0].Owner}
	default:
		itype = ObjectType_Func
		fblock.Info = &FuncInfo{}
	}
	fblock.Type = itype
	prev.Objects[name] = &ObjInfo{Type: itype, Value: fblock}
	return nil
}

// CompileBlock compile the source code into the Block structure with a byte-code
func (vm *VM) CompileBlock(input []rune, owner *OwnerInfo) (*Block, error) {
	root := &Block{Info: owner.StateID, Owner: owner}
	lexems, err := lexParser(input)
	if err != nil {
		return nil, err
	}
	if len(lexems) == 0 {
		return root, nil
	}
	curState := 0
	stack := make([]int, 0, 64)
	blockstack := make(Blocks, 1, 64)
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
			block := &Block{Parent: top}
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
		if newState.Func > 0 {
			if err := funcs[newState.Func](&blockstack, nextState, lexem); err != nil {
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
			if cond, ok := item.Value.(*Block).Objects[`conditions`]; ok {
				if cond.Type == ObjectType_Func && cond.Value.(*Block).Info.(*FuncInfo).CanWrite {
					return nil, errCondWrite
				}
			}
		}
	}
	return root, nil
}

// FlushBlock loads the compiled Block into the virtual machine
func (vm *VM) FlushBlock(root *Block) {
	shift := len(vm.Children)
	for key, item := range root.Objects {
		if cur, ok := vm.Objects[key]; ok {
			switch item.Type {
			case ObjectType_Contract:
				root.Objects[key].Value.(*Block).Info.(*ContractInfo).ID = cur.Value.(*Block).Info.(*ContractInfo).ID + flushMark
			case ObjectType_Func:
				root.Objects[key].Value.(*Block).Info.(*FuncInfo).ID = cur.Value.(*Block).Info.(*FuncInfo).ID + flushMark
				vm.Objects[key].Value = root.Objects[key].Value
			}
		}
		vm.Objects[key] = item
	}
	for _, item := range root.Children {
		switch item.Type {
		case ObjectType_Contract:
			if item.Info.(*ContractInfo).ID > flushMark {
				item.Info.(*ContractInfo).ID -= flushMark
				vm.Children[item.Info.(*ContractInfo).ID] = item
				shift--
				continue
			}
			item.Parent = &vm.Block
			item.Info.(*ContractInfo).ID += uint32(shift)
		case ObjectType_Func:
			if item.Info.(*FuncInfo).ID > flushMark {
				item.Info.(*FuncInfo).ID -= flushMark
				vm.Children[item.Info.(*FuncInfo).ID] = item
				shift--
				continue
			}
			item.Parent = &vm.Block
			item.Info.(*FuncInfo).ID += uint32(shift)
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

func findVar(name string, block *Blocks) (ret *ObjInfo, owner *Block) {
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

func (vm *VM) findObj(name string, block *Blocks) (ret *ObjInfo, owner *Block) {
	sname := StateName((*block)[0].Info.(uint32), name)
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

func (vm *VM) getInitValue(lexems *Lexems, ind *int, block *Blocks) (value mapItem, err error) {
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

func (vm *VM) getInitMap(lexems *Lexems, ind *int, block *Blocks, oneItem bool) (*types.Map, error) {
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

func (vm *VM) getInitArray(lexems *Lexems, ind *int, block *Blocks) ([]mapItem, error) {
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

func setWritable(block *Blocks) {
	for i := len(*block) - 1; i >= 0; i-- {
		blockItem := (*block)[i]
		if blockItem.Type == ObjectType_Func {
			blockItem.Info.(*FuncInfo).CanWrite = true
		}
		if blockItem.Type == ObjectType_Contract {
			blockItem.Info.(*ContractInfo).CanWrite = true
		}
	}
}

// This function is responsible for the compilation of expressions
func (vm *VM) compileEval(lexems *Lexems, ind *int, block *Blocks) error {
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
					if (objInfo.Type == ObjectType_Func && objInfo.Value.(*Block).Info.(*FuncInfo).CanWrite) ||
						(objInfo.Type == ObjectType_ExtFunc && objInfo.Value.(ExtFuncInfo).CanWrite) {
						setWritable(block)
					}
					if objInfo.Type == ObjectType_Func && objInfo.Value.(*Block).Info.(*FuncInfo).Names != nil {
						if len(bytecode) == 0 || bytecode[len(bytecode)-1].Cmd != cmdFuncName {
							bytecode.push(newByteCode(cmdPush, lexem.Line, nil))
						}
						if i < len(*lexems)-4 && (*lexems)[i+1].Type == isDot {
							if (*lexems)[i+2].Type != lexIdent {
								log.WithFields(log.Fields{"type": consts.ParseError}).Error("must be the name of the tail")
								return fmt.Errorf(`must be the name of the tail`)
							}
							names := prev.Value.(*ObjInfo).Value.(*Block).Info.(*FuncInfo).Names
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
						extinfo := prev.Value.(*ObjInfo).Value.(ExtFuncInfo)
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
						objContract *Block
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
							objContract = objInfo.Value.(*Block)
						}
						objInfo, tobj = vm.findObj(`ExecContract`, block)
						isContract = true
					}
					cmdCall := uint16(cmdCall)
					if (objInfo.Type == ObjectType_ExtFunc && objInfo.Value.(ExtFuncInfo).Variadic) ||
						(objInfo.Type == ObjectType_Func && objInfo.Value.(*Block).Info.(*FuncInfo).Variadic) {
						cmdCall = cmdCallVari
					}
					count := 0
					if (*lexems)[i+2].Type != isRPar {
						count++
					}
					buffer.push(newByteCode(cmdCall, lexem.Line, objInfo))
					if isContract {
						name := StateName((*block)[0].Info.(uint32), lexem.Value.(string))
						for j := len(*block) - 1; j >= 0; j-- {
							topblock := (*block)[j]
							if topblock.Type == ObjectType_Contract {
								if name == topblock.Info.(*ContractInfo).Name {
									return errRecursion
								}
								if topblock.Info.(*ContractInfo).Used == nil {
									topblock.Info.(*ContractInfo).Used = make(map[string]bool)
								}
								topblock.Info.(*ContractInfo).Used[name] = true
							}
						}
						if objContract != nil && objContract.Info.(*ContractInfo).CanWrite {
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
						bytecode.push(newByteCode(cmdPush, lexem.Line, (*block)[0].Info.(uint32)))
					}
					parcount = append(parcount, count)
					call = true
				}
				if (*lexems)[i+1].Type == isLBrack {
					if objInfo == nil || objInfo.Type != ObjectType_Var {
						logger.WithFields(log.Fields{"lex_value": lexem.Value.(string), "type": consts.ParseError}).Error("unknown variable")
						return fmt.Errorf(`unknown variable %s`, lexem.Value.(string))
					}
					buffer.push(newByteCode(cmdIndex, lexem.Line, &IndexInfo{VarOffset: objInfo.Value.(int), Owner: tobj}))
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
