/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import "github.com/IBAX-io/go-ibax/packages/converter"

// SetTablePrefix is setting table prefix
func (bi *BlockInterface) SetTablePrefix(prefix string) {
	bi.ecosystem = converter.StrToInt64(prefix)
}

// TableName returns name of table
func (bi BlockInterface) TableName() string {
	if bi.ecosystem == 0 {
		bi.ecosystem = 1
	}
	return `1_blocks`
}

// Get is retrieving model from database
func (bi *BlockInterface) Get(name string) (bool, error) {
	return isFound(DBConn.Where("ecosystem=? and name = ?", bi.ecosystem, name).First(bi))
}

// GetByApp returns all interface blocks belonging to selected app
func (bi *BlockInterface) GetByApp(appID int64, ecosystemID int64) ([]BlockInterface, error) {
	var result []BlockInterface
	err := DBConn.Select("id, name").Where("app_id = ? and ecosystem = ?", appID, ecosystemID).Find(&result).Error
	return result, err
}
