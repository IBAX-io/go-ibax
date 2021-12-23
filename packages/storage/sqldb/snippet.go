/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import "github.com/IBAX-io/go-ibax/packages/converter"

// Snippet is code snippet
type Snippet struct {
	ecosystem  int64
	ID         int64  `gorm:"primary_key;not null" json:"id,omitempty"`
	Name       string `gorm:"not null" json:"name,omitempty"`
	Value      string `gorm:"not null" json:"value,omitempty"`
	Conditions string `gorm:"not null" json:"conditions,omitempty"`
}

// SetTablePrefix is setting table prefix
func (bi *Snippet) SetTablePrefix(prefix string) {
	bi.ecosystem = converter.StrToInt64(prefix)
}

// TableName returns name of table
func (bi *Snippet) TableName() string {
	if bi.ecosystem == 0 {
		bi.ecosystem = 1
	}
	return `1_snippets`
}

// Get is retrieving model from database
func (bi *Snippet) Get(name string) (bool, error) {
	return isFound(DBConn.Where("ecosystem=? and name = ?", bi.ecosystem, name).First(bi))
}

// GetByApp returns all interface blocks belonging to selected app
func (bi *Snippet) GetByApp(appID int64, ecosystemID int64) ([]Snippet, error) {
	var result []Snippet
	err := DBConn.Select("id, name").Where("app_id = ? and ecosystem = ?", appID, ecosystemID).Find(&result).Error
	return result, err
}
