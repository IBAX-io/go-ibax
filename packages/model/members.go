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

// SetTablePrefix is setting table prefix
func (m *Member) SetTablePrefix(prefix string) {
	m.ecosystem = converter.StrToInt64(prefix)
}

// TableName returns name of table
func (m *Member) TableName() string {
	if m.ecosystem == 0 {
		m.ecosystem = 1
}
