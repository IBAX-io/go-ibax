/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

const (
	//	cmdUnknown = iota // error
	// here are described the commands of bytecode
	cmdPush         = iota + 1 // Push value to stack
	cmdVar                     // Push variable to stack
	cmdExtend                  // Push extend variable to stack
	cmdCallExtend              // Call extend function
	cmdPushStr                 // Push ident as string
	cmdCall                    // call a function
	cmdCallVariadic            // call a variadic function
	cmdReturn                  // return from function
	cmdIf                      // run block if Value is true
	cmdElse                    // run block if Value is false
	cmdAssignVar               // list of assigned var
	cmdAssign                  // assign
	cmdLabel                   // label for continue
	cmdContinue                // continue from label
	cmdWhile                   // while
	cmdBreak                   // break
	cmdIndex                   // get index []
	cmdSetIndex                // set index []
	cmdFuncName                // set func name Func(...).Name(...)
	cmdUnwrapArr               // unwrap array to stack
	cmdMapInit                 // map initialization
	cmdArrayInit               // array initialization
	cmdError                   // error command
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
