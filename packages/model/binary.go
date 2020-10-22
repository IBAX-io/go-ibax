/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import (
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

const BinaryTableSuffix = "_binaries"

// Binary represents record of {prefix}_binaries table
type Binary struct {
	ecosystem int64
	ID        int64
	Name      string
	Data      []byte
	Hash      string
	MimeType  string
}

// SetTablePrefix is setting table prefix
func (b *Binary) SetTablePrefix(prefix string) {
	b.ecosystem = converter.StrToInt64(prefix)
}

// SetTableName sets name of table
func (b *Binary) SetTableName(tableName string) {
	ecosystem, _ := converter.ParseName(tableName)
	b.ecosystem = ecosystem
}

// TableName returns name of table
func (b *Binary) TableName() string {
	if b.ecosystem == 0 {
		b.ecosystem = 1
	}
	return `1_binaries`
}

// Get is retrieving model from database
func (b *Binary) Get(appID int64, account, name string) (bool, error) {
	return isFound(DBConn.Where("id=?", id).First(b))
}
