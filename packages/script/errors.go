/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import "errors"

const (
	eContractLoop    = `there is loop in %s contract`
	eSysVar          = `system variable $%s cannot be changed`
	eTypeParam       = `parameter %d has wrong type`
	eUndefinedParam  = `%s is not defined`
	eUnknownContract = `unknown contract %s`
	eWrongParams     = `function %s must have %d parameters`
	errMaxArrayIndex   = errors.New(`The index is out of range`)
	errMaxMapCount     = errors.New(`The maxumim length of map`)
	errRecursion       = errors.New(`The contract can't call itself recursively`)
	errUnclosedArray   = errors.New(`unclosed array initialization`)
	errUnclosedMap     = errors.New(`unclosed map initialization`)
	errUnexpKey        = errors.New(`unexpected lexem; expecting string key`)
	errUnexpColon      = errors.New(`unexpected lexem; expecting colon`)
	errUnexpComma      = errors.New(`unexpected lexem; expecting comma`)
	errUnexpValue      = errors.New(`unexpected lexem; expecting string, int value or variable`)
	errCondWrite       = errors.New(`'conditions' cannot call contracts or functions which can modify the blockchain database.`)
	errMultiIndex      = errors.New(`multi-index is not supported`)
	errSelfAssignment  = errors.New(`self assignment`)
	errEndExp          = errors.New(`unexpected end of the expression`)
	errOper            = errors.New(`unexpected operator; expecting operand`)
)
