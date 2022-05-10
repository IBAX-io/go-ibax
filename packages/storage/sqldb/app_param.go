/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package sqldb

import (
	"github.com/IBAX-io/go-ibax/packages/converter"
)

// AppParam is model
type AppParam struct {
	ecosystem  int64
	ID         int64  `gorm:"primary_key;not null"`
	AppID      int64  `gorm:"not null"`
	Name       string `gorm:"not null;size:100"`
	Value      string `gorm:"not null"`
	Conditions string `gorm:"not null"`
}

// TableName returns name of table
func (sp *AppParam) TableName() string {
	if sp.ecosystem == 0 {
		sp.ecosystem = 1
	}
	return `1_app_params`
}

// SetTablePrefix is setting table prefix
func (sp *AppParam) SetTablePrefix(tablePrefix string) {
	sp.ecosystem = converter.StrToInt64(tablePrefix)
}

// Get is retrieving model from database
func (sp *AppParam) Get(dbTx *DbTransaction, app int64, name string) (bool, error) {
	return isFound(GetDB(dbTx).Where("ecosystem=? and app_id=? and name = ?",
		sp.ecosystem, app, name).First(sp))
}

// GetAllAppParameters is returning all state parameters
func (sp *AppParam) GetAllAppParameters(app int64) ([]AppParam, error) {
	parameters := make([]AppParam, 0)
	err := DBConn.Table(sp.TableName()).Where(`ecosystem = ?`, sp.ecosystem).Where(`app_id = ?`, app).Find(&parameters).Error
	if err != nil {
		return nil, err
	}
	return parameters, nil
}
