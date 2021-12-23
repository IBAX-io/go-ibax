/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import "github.com/IBAX-io/go-ibax/packages/converter"

// Menu is model
type Menu struct {
	ecosystem  int64
	ID         int64  `gorm:"primary_key;not null" json:"id"`
	Name       string `gorm:"not null" json:"name"`
	Title      string `gorm:"not null" json:"title"`
	Value      string `gorm:"not null" json:"value"`
	Conditions string `gorm:"not null" json:"conditions"`
}

// SetTablePrefix is setting table prefix
func (m *Menu) SetTablePrefix(prefix string) {
	m.ecosystem = converter.StrToInt64(prefix)
}

// TableName returns name of table
func (m Menu) TableName() string {
	if m.ecosystem == 0 {
		m.ecosystem = 1
	}
	return `1_menu`
}

// Get is retrieving model from database
func (m *Menu) Get(name string) (bool, error) {
	return isFound(DBConn.Where("ecosystem=? and name = ?", m.ecosystem, name).First(m))
}
