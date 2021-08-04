/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import "github.com/IBAX-io/go-ibax/packages/converter"

// StateParameter is model
	if sp.ecosystem == 0 {
		sp.ecosystem = 1
	}
	return `1_parameters`
}

// SetTablePrefix is setting table prefix
func (sp *StateParameter) SetTablePrefix(prefix string) *StateParameter {
	sp.ecosystem = converter.StrToInt64(prefix)
	return sp
}

// Get is retrieving model from database
func (sp *StateParameter) Get(transaction *DbTransaction, name string) (bool, error) {
	return isFound(GetDB(transaction).Where("ecosystem = ? and name = ?", sp.ecosystem, name).First(sp))
}

// GetAllStateParameters is returning all state parameters
func (sp *StateParameter) GetAllStateParameters() ([]StateParameter, error) {
	parameters := make([]StateParameter, 0)
	err := DBConn.Table(sp.TableName()).Where(`ecosystem = ?`, sp.ecosystem).Find(&parameters).Error
	if err != nil {
		return nil, err
	}
	return parameters, nil
}
