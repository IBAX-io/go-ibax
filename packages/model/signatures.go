/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

// Signature is model
type Signature struct {
	tableName  string
	Name       string `gorm:"primary_key;not null;size:255"`
	Value      string `gorm:"not null;type:jsonb"`
	Conditions string `gorm:"not null"`
}

// SetTablePrefix is setting table prefix
func (s *Signature) SetTablePrefix(prefix string) {
	s.tableName = prefix + "_signatures"
}

// TableName returns name of table
func (s *Signature) TableName() string {
	return s.tableName
}

