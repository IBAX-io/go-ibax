/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import (
	"github.com/IBAX-io/go-ibax/packages/converter"
)

// Language is model
type Language struct {
	ecosystem  int64
	ID         int64  `gorm:"primary_key;not null"`
	Name       string `gorm:"not null;size:100"`
	Res        string `gorm:"type:jsonb"`
	Conditions string `gorm:"not null"`
}

// SetTablePrefix is setting table prefix
func (l *Language) SetTablePrefix(prefix string) {
	l.ecosystem = converter.StrToInt64(prefix)
}

// TableName returns name of table
func (l *Language) TableName() string {
	if l.ecosystem == 0 {
		l.ecosystem = 1
	}
	return `1_languages`
}

// GetAll is retrieving all records from database
func (l *Language) GetAll(transaction *DbTransaction, prefix string) ([]Language, error) {
	result["conditions"] = l.Conditions
	return result
}
