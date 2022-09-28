package script

import (
	"fmt"
	"reflect"
	"regexp"

	"github.com/IBAX-io/go-ibax/packages/consts"
	log "github.com/sirupsen/logrus"
)

type compileFunc func(*CodeBlocks, stateTypes, *Lexeme) error

const (
	// This is a list of identifiers for functions that will generate a bytecode for the corresponding cases.
	// Indexes of handle functions funcHandles = compileFunc[]
	cfNothing = iota
	cfError
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

// VarRegexp letter { letter | unicode_digit }
var VarRegexp = `^[a-zA-Z][a-zA-Z0-9_]*$`

var (
	// The array of functions corresponding to the constants cf...
	funcHandles = map[int]compileFunc{
		cfNothing:    nil,
		cfError:      fError,
		cfNameBlock:  fNameBlock,
		cfFResult:    fFuncResult,
		cfReturn:     fReturn,
		cfIf:         fIf,
		cfElse:       fElse,
		cfFParam:     fFparam,
		cfFType:      fFtype,
		cfFTail:      fFtail,
		cfFNameParam: fFNameParam,
		cfAssignVar:  fAssignVar,
		cfAssign:     fAssign,
		cfTX:         fTx,
		cfSettings:   fSettings,
		cfConstName:  fConstName,
		cfConstValue: fConstValue,
		cfField:      fField,
		cfFieldType:  fFieldType,
		cfFieldTag:   fFieldTag,
		cfFields:     fFields,
		cfFieldComma: fFieldComma,
		cfFieldLine:  fFieldLine,
		cfWhile:      fWhile,
		cfContinue:   fContinue,
		cfBreak:      fBreak,
		cfCmdError:   fCmdError,
	}
)

func fError(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
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
	logger := lexeme.GetLogger()
	if lexeme.Type == lexNewLine {
		logger.WithFields(log.Fields{"error": errors[state], "lex_value": lexeme.Value, "type": consts.ParseError}).Error("unexpected new line")
		return fmt.Errorf(`%s (unexpected new line) [Ln:%d]`, errors[state], lexeme.Line-1)
	}
	logger.WithFields(log.Fields{"error": errors[state], "lex_value": lexeme.Value, "type": consts.ParseError}).Error("parsing error")
	return fmt.Errorf(`%s %x %v [Ln:%d Col:%d]`, errors[state], lexeme.Type, lexeme.Value, lexeme.Line, lexeme.Column)
}

func fNameBlock(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	var itype ObjectType
	prev := (*buf)[len(*buf)-2]
	fblock := buf.peek()
	name := lexeme.Value.(string)
	switch state {
	case stateBlock:
		itype = ObjectType_Contract
		name = StateName((*buf)[0].Owner.StateID, name)
		fblock.Info = &ContractInfo{ID: uint32(len(prev.Children) - 1), Name: name,
			Owner: (*buf)[0].Owner}
	default:
		itype = ObjectType_Func
		fblock.Info = &FuncInfo{Name: name}
	}
	fblock.Type = itype
	if _, ok := prev.Objects[name]; ok {
		lexeme.GetLogger().WithFields(log.Fields{"type": consts.ParseError, "contract": prev.GetContractInfo().Name, "lex_value": name}).Errorf("%s redeclared in this contract", itype)
		return fmt.Errorf("%s '%s' redeclared in this contract '%s'", itype, name, prev.GetContractInfo().Name)
	}
	prev.Objects[name] = &ObjInfo{Type: itype, Value: fblock}
	return nil
}

func fFuncResult(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	fblock := buf.peek().GetFuncInfo()
	(*fblock).Results = append((*fblock).Results, lexeme.Value.(reflect.Type))
	return nil
}

func fReturn(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	buf.peek().Code.push(newByteCode(cmdReturn, lexeme.Line, 0))
	return nil
}

func fCmdError(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	buf.peek().Code.push(newByteCode(cmdError, lexeme.Line, lexeme.Value))
	return nil
}

func fFparam(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	block := buf.peek()
	if block.Type == ObjectType_Func && (state == stateFParam || state == stateFParamTYPE) {
		fblock := block.GetFuncInfo()
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
	if !regexp.MustCompile(VarRegexp).MatchString(lexeme.Value.(string)) {
		var val = lexeme.Value.(string)
		if len(val) > 20 {
			val = val[:20] + "..."
		}
		return fmt.Errorf("identifier expected, got '%s'", val)
	}
	if _, ok := block.Objects[lexeme.Value.(string)]; ok {
		if state == stateFParamTYPE {
			return fmt.Errorf("duplicate argument '%s'", lexeme.Value.(string))
		} else if state == stateVarType {
			return fmt.Errorf("'%s' redeclared in this code block", lexeme.Value.(string))
		}
	}
	block.Objects[lexeme.Value.(string)] = &ObjInfo{Type: ObjectType_Var, Value: &ObjInfo_Variable{Name: lexeme.Value.(string), Index: len(block.Vars)}}
	block.Vars = append(block.Vars, reflect.TypeOf(nil))
	return nil
}

func fFtype(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	block := buf.peek()
	if block.Type == ObjectType_Func && state == stateFParam {
		fblock := block.GetFuncInfo()
		if fblock.Names == nil {
			for pkey, param := range fblock.Params {
				if param == reflect.TypeOf(nil) {
					fblock.Params[pkey] = lexeme.Value.(reflect.Type)
				}
			}
		} else {
			for key := range *fblock.Names {
				if key[0] == '_' {
					for pkey, param := range (*fblock.Names)[key[1:]].Params {
						if param == reflect.TypeOf(nil) {
							(*fblock.Names)[key[1:]].Params[pkey] = lexeme.Value.(reflect.Type)
						}
					}
					break
				}
			}
		}
	}
	for vkey, ivar := range block.Vars {
		if ivar == reflect.TypeOf(nil) {
			block.Vars[vkey] = lexeme.Value.(reflect.Type)
		}
	}
	return nil
}

func fFtail(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	var used bool
	block := buf.peek()

	fblock := block.GetFuncInfo()
	if fblock.Names == nil {
		for pkey, param := range fblock.Params {
			if param == reflect.TypeOf(nil) {
				if used {
					return fmt.Errorf(`... parameter must be one`)
				}
				fblock.Params[pkey] = reflect.TypeOf([]any{})
				used = true
			}
		}
		block.GetFuncInfo().Variadic = true
	} else {
		for key := range *fblock.Names {
			if key[0] == '_' {
				name := key[1:]
				for pkey, param := range (*fblock.Names)[name].Params {
					if param == reflect.TypeOf(nil) {
						if used {
							return fmt.Errorf(`... parameter must be one`)
						}
						(*fblock.Names)[name].Params[pkey] = reflect.TypeOf([]any{})
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
			block.Vars[vkey] = reflect.TypeOf([]any{})
		}
	}
	return nil
}

func fFNameParam(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	block := buf.peek()

	fblock := block.GetFuncInfo()
	if fblock.Names == nil {
		names := make(map[string]FuncName)
		fblock.Names = &names
	}
	for key := range *fblock.Names {
		if key[0] == '_' {
			delete(*fblock.Names, key)
		}
	}
	(*fblock.Names)[`_`+lexeme.Value.(string)] = FuncName{}
	return nil
}

func fIf(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	buf.get(len(*buf) - 2).Code.push(newByteCode(cmdIf, lexeme.Line, buf.peek()))
	return nil
}

func fWhile(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	buf.get(len(*buf) - 2).Code.push(newByteCode(cmdWhile, lexeme.Line, buf.peek()))
	buf.get(len(*buf) - 2).Code.push(newByteCode(cmdContinue, lexeme.Line, 0))
	return nil
}

func fContinue(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	buf.peek().Code.push(newByteCode(cmdContinue, lexeme.Line, 0))
	return nil
}

func fBreak(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	buf.peek().Code.push(newByteCode(cmdBreak, lexeme.Line, 0))
	return nil
}

func fAssignVar(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	block := buf.peek()
	var (
		prev []*VarInfo
		ivar VarInfo
	)
	if lexeme.Type == lexExtend {
		if isSysVar(lexeme.Value.(string)) {
			lexeme.GetLogger().WithFields(log.Fields{"type": consts.ParseError, "lex_value": lexeme.Value}).Error("modifying system variable")
			return fmt.Errorf(eSysVar, lexeme.Value.(string))
		}
		ivar = VarInfo{Obj: &ObjInfo{Type: ObjectType_ExtVar, Value: &ObjInfo_ExtendVariable{Name: lexeme.Value.(string)}}, Owner: nil}
	} else {
		objInfo, tobj := findVar(lexeme.Value.(string), buf)
		if objInfo == nil || objInfo.Type != ObjectType_Var {
			lexeme.GetLogger().WithFields(log.Fields{"type": consts.ParseError, "lex_value": lexeme.Value}).Error("unknown variable")
			return fmt.Errorf(`unknown variable '%s'`, lexeme.Value.(string))
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
		block.Code.push(newByteCode(cmdAssignVar, lexeme.Line, prev))
	} else {
		block.Code[len(block.Code)-1] = newByteCode(cmdAssignVar, lexeme.Line, prev)
	}
	return nil
}

func fAssign(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	buf.peek().Code.push(newByteCode(cmdAssign, lexeme.Line, 0))
	return nil
}

func fTx(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	contract := buf.peek()
	logger := lexeme.GetLogger()
	if contract.Type != ObjectType_Contract {
		logger.WithFields(log.Fields{"type": consts.ParseError, "contract_type": contract.Type, "lex_value": lexeme.Value}).Error("data can only be in contract")
		return fmt.Errorf(`data can only be in contract`)
	}
	(*contract).GetContractInfo().Tx = new([]*FieldInfo)
	return nil
}

func fSettings(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	contract := buf.peek()
	if contract.Type != ObjectType_Contract {
		logger := lexeme.GetLogger()
		logger.WithFields(log.Fields{"type": consts.ParseError, "contract_type": contract.Type, "lex_value": lexeme.Value}).Error("data can only be in contract")
		return fmt.Errorf(`settings can only be in contract`)
	}
	(*contract).GetContractInfo().Settings = make(map[string]any)
	return nil
}

func fConstName(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	sets := buf.peek().GetContractInfo().Settings
	sets[lexeme.Value.(string)] = nil
	return nil
}

func fConstValue(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	sets := buf.peek().GetContractInfo().Settings
	for key, val := range sets {
		if val == nil {
			sets[key] = lexeme.Value
			break
		}
	}
	return nil
}

func fField(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	info := buf.peek().GetContractInfo()
	tx := info.Tx
	if len(*tx) > 0 && (*tx)[len(*tx)-1].Type == reflect.TypeOf(nil) &&
		(*tx)[len(*tx)-1].Tags != `_` {
		return fmt.Errorf(eDataType, lexeme.Line, lexeme.Column)
	}

	if !regexp.MustCompile(VarRegexp).MatchString(lexeme.Value.(string)) {
		var val = lexeme.Value.(string)
		if len(val) > 20 {
			val = val[:20] + "..."
		}
		return fmt.Errorf("identifier expected, got '%s'", val)
	}

	if isSysVar(lexeme.Value.(string)) {
		lexeme.GetLogger().WithFields(log.Fields{"type": consts.ParseError, "contract": info.Name, "lex_value": lexeme.Value.(string)}).Error("param variable in the data section of the contract collides with the 'builtin' variable")
		return fmt.Errorf(eDataParamVarCollides, lexeme.Value.(string), info.Name)
	}
	*tx = append(*tx, &FieldInfo{Name: lexeme.Value.(string), Type: reflect.TypeOf(nil)})
	return nil
}

func fFields(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	tx := buf.peek().GetContractInfo().Tx
	if len(*tx) > 0 && (*tx)[len(*tx)-1].Type == nil {
		return fmt.Errorf(eDataType, lexeme.Line, lexeme.Column)
	}
	return nil
}

func fFieldComma(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	tx := buf.peek().GetContractInfo().Tx
	if len(*tx) == 0 || (*tx)[len(*tx)-1].Type != nil {
		return fmt.Errorf(eDataName, lexeme.Line, lexeme.Column)
	}
	(*tx)[len(*tx)-1].Tags = `_`
	return nil
}

func fFieldLine(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	tx := buf.peek().GetContractInfo().Tx
	if len(*tx) > 0 && (*tx)[len(*tx)-1].Type == nil {
		return fmt.Errorf(eDataType, lexeme.Line, lexeme.Column)
	}
	for i, field := range *tx {
		if field.Tags == `_` {
			(*tx)[i].Tags = ``
		}
	}
	return nil
}

func fFieldType(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	tx := buf.peek().GetContractInfo().Tx
	if len(*tx) == 0 || (*tx)[len(*tx)-1].Type != nil {
		return fmt.Errorf(eDataName, lexeme.Line, lexeme.Column)
	}
	for i, field := range *tx {
		if field.Type == reflect.TypeOf(nil) {
			(*tx)[i].Type = lexeme.Value.(reflect.Type)
			(*tx)[i].Original = lexeme.Ext
		}
	}
	return nil
}

func fFieldTag(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	tx := buf.peek().GetContractInfo().Tx
	if len(*tx) == 0 || (*tx)[len(*tx)-1].Type == nil || len((*tx)[len(*tx)-1].Tags) != 0 {
		return fmt.Errorf(eDataTag, lexeme.Line, lexeme.Column)
	}
	for i := len(*tx) - 1; i >= 0; i-- {
		if i == len(*tx)-1 || (*tx)[i].Tags == `_` {
			(*tx)[i].Tags = lexeme.Value.(string)
			continue
		}
		break
	}
	return nil
}

func fElse(buf *CodeBlocks, state stateTypes, lexeme *Lexeme) error {
	if buf.get(len(*buf)-2).Code.peek().Cmd != cmdIf {
		return fmt.Errorf(`there is not if before %v [Ln:%d Col:%d]`, lexeme.Type, lexeme.Line, lexeme.Column)
	}
	buf.get(len(*buf) - 2).Code.push(newByteCode(cmdElse, lexeme.Line, buf.peek()))
	return nil
}
