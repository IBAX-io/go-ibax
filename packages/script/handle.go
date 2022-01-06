package script

import (
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/consts"
	log "github.com/sirupsen/logrus"
	"reflect"
)

type compileFunc func(*CodeBlocks, stateTypes, *Lexem) error

const (
	// This is a list of identifiers for functions that will generate a bytecode for
	// the corresponding cases
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

func fError(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
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

func fNameBlock(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	var itype ObjectType

	prev := (*buf)[len(*buf)-2]
	fblock := buf.peek()
	name := lexem.Value.(string)
	switch state {
	case stateBlock:
		itype = ObjectType_Contract
		name = StateName((*buf)[0].Info.Uint32(), name)
		fblock.Info = newCodeBlockInfo(&ContractInfo{ID: uint32(len(prev.Children) - 1), Name: name,
			Owner: (*buf)[0].Owner})
	default:
		itype = ObjectType_Func
		fblock.Info = newCodeBlockInfo(&FuncInfo{})
	}
	fblock.Type = itype
	prev.Objects[name] = &ObjInfo{Type: itype, Value: newObjInfoValue(fblock)}
	return nil
}

func fFuncResult(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	fblock := buf.peek().Info.FuncInfo()
	(*fblock).Results = append((*fblock).Results, lexem.Value.(reflect.Type))
	return nil
}

func fReturn(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	buf.peek().Code.push(newByteCode(cmdReturn, lexem.Line, 0))
	return nil
}

func fCmdError(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	buf.peek().Code.push(newByteCode(cmdError, lexem.Line, lexem.Value))
	return nil
}

func fFparam(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	block := buf.peek()
	if block.Type == ObjectType_Func && (state == stateFParam || state == stateFParamTYPE) {
		fblock := block.Info.FuncInfo()
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
	block.Objects[lexem.Value.(string)] = &ObjInfo{Type: ObjectType_Var, Value: newObjInfoValue(len(block.Vars))}
	block.Vars = append(block.Vars, reflect.TypeOf(nil))
	return nil
}

func fFtype(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	block := buf.peek()
	if block.Type == ObjectType_Func && state == stateFParam {
		fblock := block.Info.FuncInfo()
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

func fFtail(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	var used bool
	block := buf.peek()

	fblock := block.Info.FuncInfo()
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
		block.Info.FuncInfo().Variadic = true
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

func fFNameParam(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	block := buf.peek()

	fblock := block.Info.FuncInfo()
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

func fIf(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	buf.get(len(*buf) - 2).Code.push(newByteCode(cmdIf, lexem.Line, buf.peek()))
	return nil
}

func fWhile(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	buf.get(len(*buf) - 2).Code.push(newByteCode(cmdWhile, lexem.Line, buf.peek()))
	buf.get(len(*buf) - 2).Code.push(newByteCode(cmdContinue, lexem.Line, 0))
	return nil
}

func fContinue(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	buf.peek().Code.push(newByteCode(cmdContinue, lexem.Line, 0))
	return nil
}

func fBreak(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	buf.peek().Code.push(newByteCode(cmdBreak, lexem.Line, 0))
	return nil
}

func fAssignVar(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
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
		ivar = VarInfo{Obj: &ObjInfo{Type: ObjectType_Extend, Value: newObjInfoValue(lexem.Value.(string))}, Owner: nil}
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

func fAssign(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	buf.peek().Code.push(newByteCode(cmdAssign, lexem.Line, 0))
	return nil
}

func fTx(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	contract := buf.peek()
	logger := lexem.GetLogger()
	if contract.Type != ObjectType_Contract {
		logger.WithFields(log.Fields{"type": consts.ParseError, "contract_type": contract.Type, "lex_value": lexem.Value}).Error("data can only be in contract")
		return fmt.Errorf(`data can only be in contract`)
	}
	(*contract).Info.ContractInfo().Tx = new([]*FieldInfo)
	return nil
}

func fSettings(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	contract := buf.peek()
	if contract.Type != ObjectType_Contract {
		logger := lexem.GetLogger()
		logger.WithFields(log.Fields{"type": consts.ParseError, "contract_type": contract.Type, "lex_value": lexem.Value}).Error("data can only be in contract")
		return fmt.Errorf(`data can only be in contract`)
	}
	(*contract).Info.ContractInfo().Settings = make(map[string]interface{})
	return nil
}

func fConstName(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	sets := buf.peek().Info.ContractInfo().Settings
	sets[lexem.Value.(string)] = nil
	return nil
}

func fConstValue(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	sets := buf.peek().Info.ContractInfo().Settings
	for key, val := range sets {
		if val == nil {
			sets[key] = lexem.Value
			break
		}
	}
	return nil
}

func fField(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	tx := buf.peek().Info.ContractInfo().Tx
	if len(*tx) > 0 && (*tx)[len(*tx)-1].Type == reflect.TypeOf(nil) &&
		(*tx)[len(*tx)-1].Tags != `_` {
		return fmt.Errorf(eDataType, lexem.Line, lexem.Column)
	}
	*tx = append(*tx, &FieldInfo{Name: lexem.Value.(string), Type: reflect.TypeOf(nil)})
	return nil
}

func fFields(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	tx := buf.peek().Info.ContractInfo().Tx
	if len(*tx) > 0 && (*tx)[len(*tx)-1].Type == nil {
		return fmt.Errorf(eDataType, lexem.Line, lexem.Column)
	}
	return nil
}

func fFieldComma(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	tx := buf.peek().Info.ContractInfo().Tx
	if len(*tx) == 0 || (*tx)[len(*tx)-1].Type != nil {
		return fmt.Errorf(eDataName, lexem.Line, lexem.Column)
	}
	(*tx)[len(*tx)-1].Tags = `_`
	return nil
}

func fFieldLine(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	tx := buf.peek().Info.ContractInfo().Tx
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

func fFieldType(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	tx := buf.peek().Info.ContractInfo().Tx
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

func fFieldTag(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	tx := buf.peek().Info.ContractInfo().Tx
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

func fElse(buf *CodeBlocks, state stateTypes, lexem *Lexem) error {
	if buf.get(len(*buf)-2).Code.peek().Cmd != cmdIf {
		return fmt.Errorf(`there is not if before %v [Ln:%d Col:%d]`, lexem.Type, lexem.Line, lexem.Column)
	}
	buf.get(len(*buf) - 2).Code.push(newByteCode(cmdElse, lexem.Line, buf.peek()))
	return nil
}
