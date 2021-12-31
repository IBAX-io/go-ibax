package script

type (
	stateTypes int
	stateLine  map[int]compileState
	// The list of compile states
	compileStates []stateLine
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
		{ // stateRoot
			lexNewLine:                      {NewState: stateRoot, FuncFlag: cfNothing},
			lexKeyword | (keyContract << 8): {NewState: stateContract | statePush, FuncFlag: cfNothing},
			lexKeyword | (keyFunc << 8):     {NewState: stateFunc | statePush, FuncFlag: cfNothing},
			0:                               {NewState: errUnknownCmd, FuncFlag: cfError},
		},
		{ // stateBody
			lexNewLine:                      {NewState: stateBody, FuncFlag: cfNothing},
			lexKeyword | (keyFunc << 8):     {NewState: stateFunc | statePush, FuncFlag: cfNothing},
			lexKeyword | (keyReturn << 8):   {NewState: stateEval, FuncFlag: cfReturn},
			lexKeyword | (keyContinue << 8): {NewState: stateBody, FuncFlag: cfContinue},
			lexKeyword | (keyBreak << 8):    {NewState: stateBody, FuncFlag: cfBreak},
			lexKeyword | (keyIf << 8):       {NewState: stateEval | statePush | stateToBlock | stateMustEval, FuncFlag: cfIf},
			lexKeyword | (keyWhile << 8):    {NewState: stateEval | statePush | stateToBlock | stateLabel | stateMustEval, FuncFlag: cfWhile},
			lexKeyword | (keyElse << 8):     {NewState: stateBlock | statePush, FuncFlag: cfElse},
			lexKeyword | (keyVar << 8):      {NewState: stateVar, FuncFlag: cfNothing},
			lexKeyword | (keyTX << 8):       {NewState: stateTX, FuncFlag: cfTX},
			lexKeyword | (keySettings << 8): {NewState: stateSettings, FuncFlag: cfSettings},
			lexKeyword | (keyError << 8):    {NewState: stateEval, FuncFlag: cfCmdError},
			lexKeyword | (keyWarning << 8):  {NewState: stateEval, FuncFlag: cfCmdError},
			lexKeyword | (keyInfo << 8):     {NewState: stateEval, FuncFlag: cfCmdError},
			lexIdent:                        {NewState: stateAssignEval | stateFork, FuncFlag: cfNothing},
			lexExtend:                       {NewState: stateAssignEval | stateFork, FuncFlag: cfNothing},
			isRCurly:                        {NewState: statePop, FuncFlag: cfNothing},
			0:                               {NewState: errMustRCurly, FuncFlag: cfError},
		},
		{ // stateBlock
			lexNewLine: {NewState: stateBlock, FuncFlag: cfNothing},
			isLCurly:   {NewState: stateBody, FuncFlag: cfNothing},
			0:          {NewState: errMustLCurly, FuncFlag: cfError},
		},
		{ // stateContract
			lexNewLine: {NewState: stateContract, FuncFlag: cfNothing},
			lexIdent:   {NewState: stateBlock, FuncFlag: cfNameBlock},
			0:          {NewState: errMustName, FuncFlag: cfError},
		},
		{ // stateFunc
			lexNewLine: {NewState: stateFunc, FuncFlag: cfNothing},
			lexIdent:   {NewState: stateFParams, FuncFlag: cfNameBlock},
			0:          {NewState: errMustName, FuncFlag: cfError},
		},
		{ // stateFParams
			lexNewLine: {NewState: stateFParams, FuncFlag: cfNothing},
			isLPar:     {NewState: stateFParam, FuncFlag: cfNothing},
			0:          {NewState: stateFResult | stateStay, FuncFlag: cfNothing},
		},
		{ // stateFParam
			lexNewLine: {NewState: stateFParam, FuncFlag: cfNothing},
			lexIdent:   {NewState: stateFParamTYPE, FuncFlag: cfFParam},
			// lexType:  {NewState: stateFParam, Func: cfFType},
			isComma: {NewState: stateFParam, FuncFlag: cfNothing},
			isRPar:  {NewState: stateFResult, FuncFlag: cfNothing},
			0:       {NewState: errParams, FuncFlag: cfError},
		},
		{ // stateFParamTYPE
			lexIdent:                    {NewState: stateFParamTYPE, FuncFlag: cfFParam},
			lexType:                     {NewState: stateFParam, FuncFlag: cfFType},
			lexKeyword | (keyTail << 8): {NewState: stateFTail, FuncFlag: cfFTail},
			isComma:                     {NewState: stateFParamTYPE, FuncFlag: cfNothing},
			//			isRPar:  {NewState: stateFResult, Func: cfNothing},
			0: {NewState: errVarType, FuncFlag: cfError},
		},
		{ // stateFTail
			lexNewLine: {NewState: stateFTail, FuncFlag: cfNothing},
			isRPar:     {NewState: stateFResult, FuncFlag: cfNothing},
			0:          {NewState: errParams, FuncFlag: cfError},
		},
		{ // stateFResult
			lexNewLine: {NewState: stateFResult, FuncFlag: cfNothing},
			isDot:      {NewState: stateFDot, FuncFlag: cfNothing},
			lexType:    {NewState: stateFResult, FuncFlag: cfFResult},
			isComma:    {NewState: stateFResult, FuncFlag: cfNothing},
			0:          {NewState: stateBlock | stateStay, FuncFlag: cfNothing},
		},
		{ // stateFDot
			lexNewLine: {NewState: stateFDot, FuncFlag: cfNothing},
			lexIdent:   {NewState: stateFParams, FuncFlag: cfFNameParam},
			0:          {NewState: errMustName, FuncFlag: cfError},
		},
		{ // stateVar
			lexNewLine: {NewState: stateBody, FuncFlag: cfNothing},
			lexIdent:   {NewState: stateVarType, FuncFlag: cfFParam},
			isRCurly:   {NewState: stateBody | stateStay, FuncFlag: cfNothing},
			isComma:    {NewState: stateVar, FuncFlag: cfNothing},
			0:          {NewState: errVars, FuncFlag: cfError},
		},
		{ // stateVarType
			lexIdent: {NewState: stateVarType, FuncFlag: cfFParam},
			lexType:  {NewState: stateVar, FuncFlag: cfFType},
			isComma:  {NewState: stateVarType, FuncFlag: cfNothing},
			0:        {NewState: errVarType, FuncFlag: cfError},
		},
		{ // stateAssignEval
			isLPar:   {NewState: stateEval | stateToFork | stateToBody, FuncFlag: cfNothing},
			isLBrack: {NewState: stateEval | stateToFork | stateToBody, FuncFlag: cfNothing},
			0:        {NewState: stateAssign | stateToFork | stateStay, FuncFlag: cfNothing},
		},
		{ // stateAssign
			isComma:   {NewState: stateAssign, FuncFlag: cfNothing},
			lexIdent:  {NewState: stateAssign, FuncFlag: cfAssignVar},
			lexExtend: {NewState: stateAssign, FuncFlag: cfAssignVar},
			isEq:      {NewState: stateEval | stateToBody, FuncFlag: cfAssign},
			0:         {NewState: errAssign, FuncFlag: cfError},
		},
		{ // stateTX
			lexNewLine: {NewState: stateTX, FuncFlag: cfNothing},
			isLCurly:   {NewState: stateFields, FuncFlag: cfNothing},
			0:          {NewState: errMustLCurly, FuncFlag: cfError},
		},
		{ // stateSettings
			lexNewLine: {NewState: stateSettings, FuncFlag: cfNothing},
			isLCurly:   {NewState: stateConsts, FuncFlag: cfNothing},
			0:          {NewState: errMustLCurly, FuncFlag: cfError},
		},
		{ // stateConsts
			lexNewLine: {NewState: stateConsts, FuncFlag: cfNothing},
			isComma:    {NewState: stateConsts, FuncFlag: cfNothing},
			lexIdent:   {NewState: stateConstsAssign, FuncFlag: cfConstName},
			isRCurly:   {NewState: stateToBody, FuncFlag: cfNothing},
			0:          {NewState: errMustRCurly, FuncFlag: cfError},
		},
		{ // stateConstsAssign
			isEq: {NewState: stateConstsValue, FuncFlag: cfNothing},
			0:    {NewState: errAssign, FuncFlag: cfError},
		},
		{ // stateConstsValue
			lexString: {NewState: stateConsts, FuncFlag: cfConstValue},
			lexNumber: {NewState: stateConsts, FuncFlag: cfConstValue},
			0:         {NewState: errStrNum, FuncFlag: cfError},
		},
		{ // stateFields
			lexNewLine: {NewState: stateFields, FuncFlag: cfFieldLine},
			isComma:    {NewState: stateFields, FuncFlag: cfFieldComma},
			lexIdent:   {NewState: stateFields, FuncFlag: cfField},
			lexType:    {NewState: stateFields, FuncFlag: cfFieldType},
			lexString:  {NewState: stateFields, FuncFlag: cfFieldTag},
			isRCurly:   {NewState: stateToBody, FuncFlag: cfFields},
			0:          {NewState: errMustRCurly, FuncFlag: cfError},
		},
	}
)
