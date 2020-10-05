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
	return *result, err
}

// ToMap is converting model to map
func (l *Language) ToMap() map[string]string {
	result := make(map[string]string, 0)
	result["name"] = l.Name
	result["res"] = l.Res
	result["conditions"] = l.Conditions
	return result
}
