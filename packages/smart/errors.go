/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"errors"
	"fmt"
)

const (
	eContractLoop          = `there is loop in %s contract`
	eContractExist         = `contract %s already exists`
	eLatin                 = `Name %s must only contain latin, digit and '_', '-' characters`
	eAccessContract        = `%s can only be called with condition: %s`
	eColumnExist           = `column %s exists`
	eColumnNotExist        = `column %s doesn't exist`
	eColumnType            = `Type '%s' of columns is not supported`
	eNotCustomTable        = `%s is not a custom table`
	eEmptyCond             = `%v condition is empty`
	eIncorrectSignature    = `incorrect signature %s`
	eItemNotFound          = `item %d has not been found`
	eManyColumns           = `Too many columns. Limit is %d`
	eNotCondition          = `There is not %s in parameters`
	eParamNotFound         = `Parameter %s has not been found`
	eRecordNotFound        = `Record %s has not been found`
	eTableExists           = `table %s exists`
	eTableNotFound         = `table %s has not been found`
	eTypeJSON              = `Type %T doesn't support json marshalling`
	eUnknownContract       = `Unknown contract %s`
	eUnsupportedType       = "Unsupported type %T"
	eWrongRandom           = `wrong random parameters min: %v, max: %v`
	eGreaterThan           = `%s must be greater than 0`
	eTableNotEmpty         = `Table %s is not empty`
	eColumnNotDeleted      = `Column %s cannot be deleted`
	eRollbackContract      = `Wrong rollback of the latest contract %d != %d`
	eExternalNet           = `External network %s is not defined`
	eKeyNotFound           = `sender %s has not been found`
	eEcoKeyNotFound        = `sender %s has not been found in ecosystem %d`
	eEcoKeyDisable         = `%s disable in ecosystem %d`
	eEcoFuelRate           = `fuel rate must be greater than 0 or empty in ecosystem %d`
	eEcoCurrentBalance     = `account %s current balance is not enough in ecosystem %d`
	eEcoCurrentBalanceDiff = eEcoCurrentBalance + `, at least [%s] difference`
)

var (
	errDelayedContract   = errors.New(`incorrect delayed contract`)
	errAccessDenied      = errors.New(`access denied`)
	errConditionEmpty    = errors.New(`conditions is empty`)
	errContractNotFound  = errors.New(`contract has not been found`)
	errTaxes             = errors.New("not enough money to pay the taxes fee")
	errEmptyColumn       = errors.New(`column name is empty`)
	errWrongColumn       = errors.New(`column name cannot begin with digit`)
	errNotFound          = errors.New(`record has not been found`)
	errContractChange    = errors.New(`contract cannot be removed or inserted`)
	errDeletedKey        = errors.New(`the key is deleted`)
	errDiffKeys          = errors.New(`contract and user public keys are different`)
	errEmpty             = errors.New(`empty value and condition`)
	errEmptyCond         = errors.New(`the condition is empty`)
	errEmptyContract     = errors.New(`empty contract name in ContractConditions`)
	errEmptyPublicKey    = errors.New(`empty public key`)
	errFounderAccount    = errors.New(`unknown founder account`)
	errKeyIDAccount      = errors.New(`unknown address account`)
	errFuelRate          = errors.New(`fuel rate must be greater than 0`)
	errIncorrectSign     = errors.New(`incorrect sign`)
	errIncorrectType     = errors.New(`incorrect type`)
	errInvalidValue      = errors.New(`invalid value`)
	errNameChange        = errors.New(`contracts or functions names cannot be changed`)
	errNegPrice          = errors.New(`price value is negative`)
	errOneContract       = errors.New(`only one contract must be in the record`)
	errPermEmpty         = errors.New(`permissions are empty`)
	errRecursion         = errors.New("recursion detected")
	errSameColumns       = errors.New(`there are the same columns`)
	errTableName         = errors.New(`the name of the table cannot begin with @`)
	errTableEmptyName    = errors.New(`the table name cannot be empty`)
	errUndefColumns      = errors.New(`columns are undefined`)
	errUpdNotExistRecord = errors.New(`update for not existing record`)
	errWrongSignature    = errors.New(`wrong signature`)
	errParseTransaction  = errors.New(`parse transaction`)
	errWhereUpdate       = errors.New(`there is not Where in Update request`)
	errNotValidUTF       = errors.New(`result is not valid utf-8 string`)
	errFloat             = errors.New(`incorrect float value`)
	errFloatResult       = errors.New(`incorrect float result`)

	errMaxPrice = fmt.Errorf(`price value is more than %d`, MaxPrice)
)
