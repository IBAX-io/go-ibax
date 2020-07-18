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
	eArrIndex        = `index of array cannot be type %s`
	eMapIndex        = `index of map cannot be type %s`
	eUnknownIdent    = `unknown identifier %s`
	eWrongVar        = `wrong var %v`
	eDataType        = `expecting type of the data field [Ln:%d Col:%d]`
	eDataName        = `expecting name of the data field [Ln:%d Col:%d]`
	eDataTag         = `unexpected tag [Ln:%d Col:%d]`
)
	errUnexpComma      = errors.New(`unexpected lexem; expecting comma`)
	errUnexpValue      = errors.New(`unexpected lexem; expecting string, int value or variable`)
	errCondWrite       = errors.New(`'conditions' cannot call contracts or functions which can modify the blockchain database.`)
	errMultiIndex      = errors.New(`multi-index is not supported`)
	errSelfAssignment  = errors.New(`self assignment`)
	errEndExp          = errors.New(`unexpected end of the expression`)
	errOper            = errors.New(`unexpected operator; expecting operand`)
)
