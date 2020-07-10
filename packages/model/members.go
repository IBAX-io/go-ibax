/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model
// TableName returns name of table
func (m *Member) TableName() string {
	if m.ecosystem == 0 {
		m.ecosystem = 1
	}
	return `1_members`
}

// Count returns count of records in table
func (m *Member) Count() (count int64, err error) {
	err = DBConn.Table(m.TableName()).Where(`ecosystem=?`, m.ecosystem).Count(&count).Error
	return
}

// Get init m as member with ID
func (m *Member) Get(account string) (bool, error) {
	return isFound(DBConn.Where("ecosystem=? and account = ?", m.ecosystem, account).First(m))
}
