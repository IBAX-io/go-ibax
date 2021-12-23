/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"strings"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

// ExternalBlockchain represents a txinfo table
type ExternalBlockchain struct {
	Id               int64  `gorm:"primary_key;not null"`
	Value            string `gorm:"not null"`
	ExternalContract string `gorm:"not null"`
	ResultContract   string `gorm:"not null"`
	Url              string `gorm:"not null"`
	Uid              string `gorm:"not null"`
	TxTime           int64  `gorm:"not null"`
	Sent             int64  `gorm:"not null"`
	Hash             []byte `gorm:"not null"`
	Attempts         int64  `gorm:"not null"`
}

// GetExternalList returns the list of network tx
func GetExternalList() (list []ExternalBlockchain, err error) {
	err = DBConn.Table("external_blockchain").
		Order("id desc").Scan(&list).Error
	return
}

// DelExternalList deletes sent tx
func DelExternalList(list []int64) error {
	slist := make([]string, len(list))
	for i, v := range list {
		slist[i] = converter.Int64ToStr(v)
	}
	return DBConn.Exec("delete from external_blockchain where id in (" +
		strings.Join(slist, `,`) + ")").Error
}

func HashExternalTx(id int64, hash []byte) error {
	return DBConn.Exec("update external_blockchain set hash=?, sent = 1 where id = ?", hash, id).Error
}

func IncExternalAttempt(id int64) error {
	return DBConn.Exec("update external_blockchain set attempts=attempts+1 where id = ?", id).Error
}
