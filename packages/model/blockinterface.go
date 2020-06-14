/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import "github.com/IBAX-io/go-ibax/packages/converter"

// BlockInterface is model
type BlockInterface struct {
	ecosystem  int64
	ID         int64  `gorm:"primary_key;not null" json:"id,omitempty"`
	Name       string `gorm:"not null" json:"name,omitempty"`
	Value      string `gorm:"not null" json:"value,omitempty"`
	Conditions string `gorm:"not null" json:"conditions,omitempty"`
}

// SetTablePrefix is setting table prefix
func (bi *BlockInterface) SetTablePrefix(prefix string) {
	bi.ecosystem = converter.StrToInt64(prefix)
}

// TableName returns name of table
func (bi BlockInterface) TableName() string {
	if bi.ecosystem == 0 {
	var result []BlockInterface
	err := DBConn.Select("id, name").Where("app_id = ? and ecosystem = ?", appID, ecosystemID).Find(&result).Error
	return result, err
}
