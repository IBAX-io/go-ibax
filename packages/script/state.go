package script

type (
	stateTypes int
	stateLine  map[int]compileState
	// The list of compile states
	compileStates map[stateTypes]stateLine
)

const (
	// The list of state types
	stateRoot stateTypes = iota
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

var (
	// 'states' describes a finite machine with states on the base of which a bytecode will be generated
	states = compileStates{
		stateRoot: { // stateRoot
			lexNewLine:                      newCompileState(stateRoot, cfNothing),
			lexKeyword | (keyContract << 8): newCompileState(stateContract|statePush, cfNothing),
			lexKeyword | (keyFunc << 8):     newCompileState(stateFunc|statePush, cfNothing),
			lexUnknown:                      newCompileState(errUnknownCmd, cfError),
		},
		stateBody: { // stateBody
			lexNewLine:                      newCompileState(stateBody, cfNothing),
			lexKeyword | (keyFunc << 8):     newCompileState(stateFunc|statePush, cfNothing),
			lexKeyword | (keyReturn << 8):   newCompileState(stateEval, cfReturn),
			lexKeyword | (keyContinue << 8): newCompileState(stateBody, cfContinue),
			lexKeyword | (keyBreak << 8):    newCompileState(stateBody, cfBreak),
			lexKeyword | (keyIf << 8):       newCompileState(stateEval|statePush|stateToBlock|stateMustEval, cfIf),
			lexKeyword | (keyWhile << 8):    newCompileState(stateEval|statePush|stateToBlock|stateLabel|stateMustEval, cfWhile),
			lexKeyword | (keyElse << 8):     newCompileState(stateBlock|statePush, cfElse),
			lexKeyword | (keyVar << 8):      newCompileState(stateVar, cfNothing),
			lexKeyword | (keyTX << 8):       newCompileState(stateTX, cfTX),
			lexKeyword | (keySettings << 8): newCompileState(stateSettings, cfSettings),
			lexKeyword | (keyError << 8):    newCompileState(stateEval, cfCmdError),
			lexKeyword | (keyWarning << 8):  newCompileState(stateEval, cfCmdError),
			lexKeyword | (keyInfo << 8):     newCompileState(stateEval, cfCmdError),
			lexIdent:                        newCompileState(stateAssignEval|stateFork, cfNothing),
			lexExtend:                       newCompileState(stateAssignEval|stateFork, cfNothing),
			isRCurly:                        newCompileState(statePop, cfNothing),
			lexUnknown:                      newCompileState(errMustRCurly, cfError),
		},
		stateBlock: { // stateBlock
			lexNewLine: newCompileState(stateBlock, cfNothing),
			isLCurly:   newCompileState(stateBody, cfNothing),
			lexUnknown: newCompileState(errMustLCurly, cfError),
		},
		stateContract: { // stateContract
			lexNewLine: newCompileState(stateContract, cfNothing),
			lexIdent:   newCompileState(stateBlock, cfNameBlock),
			lexUnknown: newCompileState(errMustName, cfError),
		},
		stateFunc: { // stateFunc
			lexNewLine: newCompileState(stateFunc, cfNothing),
			lexIdent:   newCompileState(stateFParams, cfNameBlock),
			lexUnknown: newCompileState(errMustName, cfError),
		},
		stateFParams: { // stateFParams
			lexNewLine: newCompileState(stateFParams, cfNothing),
			isLPar:     newCompileState(stateFParam, cfNothing),
			lexUnknown: newCompileState(stateFResult|stateStay, cfNothing),
		},
		stateFParam: { // stateFParam
			lexNewLine: newCompileState(stateFParam, cfNothing),
			lexIdent:   newCompileState(stateFParamTYPE, cfFParam),
			//lexType:  newCompileState(stateFParam, cfFType),
			isComma:    newCompileState(stateFParam, cfNothing),
			isRPar:     newCompileState(stateFResult, cfNothing),
			lexUnknown: newCompileState(errParams, cfError),
		},
		stateFParamTYPE: { // stateFParamTYPE
			lexIdent:                    newCompileState(stateFParamTYPE, cfFParam),
			lexType:                     newCompileState(stateFParam, cfFType),
			lexKeyword | (keyTail << 8): newCompileState(stateFTail, cfFTail),
			isComma:                     newCompileState(stateFParamTYPE, cfNothing),
			//			isRPar:  newCompileState(stateFResult, Func: cfNothing),
			lexUnknown: newCompileState(errVarType, cfError),
		},
		stateFTail: { // stateFTail
			lexNewLine: newCompileState(stateFTail, cfNothing),
			isRPar:     newCompileState(stateFResult, cfNothing),
			lexUnknown: newCompileState(errParams, cfError),
		},
		stateFResult: { // stateFResult
			lexNewLine: newCompileState(stateFResult, cfNothing),
			isDot:      newCompileState(stateFDot, cfNothing),
			lexType:    newCompileState(stateFResult, cfFResult),
			isComma:    newCompileState(stateFResult, cfNothing),
			lexUnknown: newCompileState(stateBlock|stateStay, cfNothing),
		},
		stateFDot: { // stateFDot
			lexNewLine: newCompileState(stateFDot, cfNothing),
			lexIdent:   newCompileState(stateFParams, cfFNameParam),
			lexUnknown: newCompileState(errMustName, cfError),
		},
		stateVar: { // stateVar
			lexNewLine: newCompileState(stateBody, cfNothing),
			lexIdent:   newCompileState(stateVarType, cfFParam),
			isRCurly:   newCompileState(stateBody|stateStay, cfNothing),
			isComma:    newCompileState(stateVar, cfNothing),
			lexUnknown: newCompileState(errVars, cfError),
		},
		stateVarType: { // stateVarType
			lexIdent:   newCompileState(stateVarType, cfFParam),
			lexType:    newCompileState(stateVar, cfFType),
			isComma:    newCompileState(stateVarType, cfNothing),
			lexUnknown: newCompileState(errVarType, cfError),
		},
		stateAssignEval: { // stateAssignEval
			isLPar:     newCompileState(stateEval|stateToFork|stateToBody, cfNothing),
			isLBrack:   newCompileState(stateEval|stateToFork|stateToBody, cfNothing),
			lexUnknown: newCompileState(stateAssign|stateToFork|stateStay, cfNothing),
		},
		stateAssign: { // stateAssign
			isComma:    newCompileState(stateAssign, cfNothing),
			lexIdent:   newCompileState(stateAssign, cfAssignVar),
			lexExtend:  newCompileState(stateAssign, cfAssignVar),
			isEq:       newCompileState(stateEval|stateToBody, cfAssign),
			lexUnknown: newCompileState(errAssign, cfError),
		},
		stateTX: { // stateTX
			lexNewLine: newCompileState(stateTX, cfNothing),
			isLCurly:   newCompileState(stateFields, cfNothing),
			//lexIdent:   newCompileState(stateAssign, cfTX), //todo
			lexExtend:  newCompileState(stateAssign, cfTX), //todo
			lexUnknown: newCompileState(errMustLCurly, cfError),
		},
		stateSettings: { // stateSettings
			lexNewLine: newCompileState(stateSettings, cfNothing),
			isLCurly:   newCompileState(stateConsts, cfNothing),
			lexUnknown: newCompileState(errMustLCurly, cfError),
		},
		stateConsts: { // stateConsts
			lexNewLine: newCompileState(stateConsts, cfNothing),
			isComma:    newCompileState(stateConsts, cfNothing),
			lexIdent:   newCompileState(stateConstsAssign, cfConstName),
			isRCurly:   newCompileState(stateToBody, cfNothing),
			lexUnknown: newCompileState(errMustRCurly, cfError),
		},
		stateConstsAssign: { // stateConstsAssign
			isEq:       newCompileState(stateConstsValue, cfNothing),
			lexUnknown: newCompileState(errAssign, cfError),
		},
		stateConstsValue: { // stateConstsValue
			lexString:  newCompileState(stateConsts, cfConstValue),
			lexNumber:  newCompileState(stateConsts, cfConstValue),
			lexUnknown: newCompileState(errStrNum, cfError),
		},
		stateFields: { // stateFields
			lexNewLine: newCompileState(stateFields, cfFieldLine),
			isComma:    newCompileState(stateFields, cfFieldComma),
			lexIdent:   newCompileState(stateFields, cfField),
			lexType:    newCompileState(stateFields, cfFieldType),
			lexString:  newCompileState(stateFields, cfFieldTag),
			isRCurly:   newCompileState(stateToBody, cfFields),
			lexUnknown: newCompileState(errMustRCurly, cfError),
		},
	}
)
