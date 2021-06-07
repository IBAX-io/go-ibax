/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model
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
