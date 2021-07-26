/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script
	cmdAssign                // assign
	cmdLabel                 // label for continue
	cmdContinue              // continue from label
	cmdWhile                 // while
	cmdBreak                 // break
	cmdIndex                 // get index []
	cmdSetIndex              // set index []
	cmdFuncName              // set func name Func(...).Name(...)
	cmdUnwrapArr             // unwrap array to stack
	cmdMapInit               // map initialization
	cmdArrayInit             // array initialization
	cmdError                 // error command
)

// the commands for operations in expressions are listed below
const (
	cmdNot = iota | 0x0100
	cmdSign
)

const (
	cmdAdd = iota | 0x0200
	cmdSub
	cmdMul
	cmdDiv
	cmdAnd
	cmdOr
	cmdEqual
	cmdNotEq
	cmdLess
	cmdNotLess
	cmdGreat
	cmdNotGreat

	cmdSys          = 0xff
	cmdUnary uint16 = 50
)
