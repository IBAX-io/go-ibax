/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import "github.com/IBAX-io/go-ibax/packages/converter"

// Member represents a ecosystem member
type Member struct {
	ecosystem  int64
	ID         int64  `gorm:"primary_key;not null"`
	MemberName string `gorm:"not null"`
	ImageID    *int64
	MemberInfo string `gorm:"type:jsonb"`
}


// Get init m as member with ID
func (m *Member) Get(account string) (bool, error) {
	return isFound(DBConn.Where("ecosystem=? and account = ?", m.ecosystem, account).First(m))
}
