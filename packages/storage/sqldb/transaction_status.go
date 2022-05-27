/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// TransactionStatus is model
type TransactionStatus struct {
	Hash     []byte `gorm:"primary_key;not null"`
	Time     int64  `gorm:"not null;"`
	Type     byte   `gorm:"not null"`
	WalletID int64  `gorm:"not null"`
	BlockID  int64  `gorm:"not null"`
	Error    string `gorm:"not null"`
	Penalty  int64  `gorm:"not null"`
}

// TableName returns name of table
func (ts *TransactionStatus) TableName() string {
	return "transactions_status"
}

// Create is creating record of model
func (ts *TransactionStatus) Create() error {
	return DBConn.Create(ts).Error
}

// Get is retrieving model from database
func (ts *TransactionStatus) Get(transactionHash []byte) (bool, error) {
	return isFound(DBConn.Where("hash = ?", transactionHash).First(ts))
}

// UpdateBlockID is updating block id
func (ts *TransactionStatus) UpdateBlockID(dbTx *DbTransaction, newBlockID int64, transactionHash []byte) error {
	return GetDB(dbTx).Model(&TransactionStatus{}).Where("hash = ?", transactionHash).Update("block_id", newBlockID).Error
}

type UpdateBlockMsg struct {
	Hash []byte
	Msg  string
}

func UpdateBlockMsgBatches(dbTx *gorm.DB, newBlockID int64, updBlockMsg []*UpdateBlockMsg) error {
	if len(updBlockMsg) == 0 {
		return nil
	}
	var (
		upStr   string
		hashArr [][]byte
	)
	for _, s := range updBlockMsg {
		hashArr = append(hashArr, s.Hash)
		upStr += fmt.Sprintf("WHEN decode('%x','hex') THEN '%s' ", s.Hash, strings.Replace(s.Msg, `'`, `''`, -1))
	}
	sqlStr := fmt.Sprintf("UPDATE transactions_status SET error = CASE hash %s END , block_id  = %d WHERE hash in(?)", upStr, newBlockID)
	return dbTx.Exec(sqlStr, hashArr).Error
}

// SetError is updating transaction status error
func (ts *TransactionStatus) SetError(dbTx *DbTransaction, errorText string, transactionHash []byte) error {
	return GetDB(dbTx).Model(&TransactionStatus{}).Where("hash = ?", transactionHash).Update("error", errorText).Error
}

// UpdatePenalty is updating penalty
func (ts *TransactionStatus) UpdatePenalty(dbTx *DbTransaction, transactionHash []byte) error {
	return GetDB(dbTx).Model(&TransactionStatus{}).Where("hash = ? AND penalty = 0", transactionHash).Update("penalty", 1).Error
}
