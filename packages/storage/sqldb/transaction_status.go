/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/IBAX-io/go-ibax/packages/conf"
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
func (ts *TransactionStatus) UpdateBlockID(transaction *DbTransaction, newBlockID int64, transactionHash []byte) error {
	return GetDB(transaction).Model(&TransactionStatus{}).Where("hash = ?", transactionHash).Update("block_id", newBlockID).Error
}

type updateBlockMsg struct {
	Hash []byte
	Msg  string
}

var updBlockMsg []updateBlockMsg

// SetTransactionStatusBlockMsg is updating block msg
func SetTransactionStatusBlockMsg(transaction *DbTransaction, newBlockID int64, msg string, transactionHash []byte) error {
	if len(msg) > 255 {
		msg = msg[:255]
	}
	if !conf.Config.IsCLBMaster() {
		updBlockMsg = append(updBlockMsg, updateBlockMsg{Msg: msg, Hash: transactionHash})
		return nil
	}

	return GetDB(transaction).Model(&TransactionStatus{}).Where("hash = ?", transactionHash).Updates(
		map[string]interface{}{"block_id": newBlockID, "error": msg}).Error
}

func UpdateBlockMsgBatches(dbTx *gorm.DB, newBlockID int64) error {
	defer func() {
		updBlockMsg = []updateBlockMsg{}
	}()
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
func (ts *TransactionStatus) SetError(transaction *DbTransaction, errorText string, transactionHash []byte) error {
	return GetDB(transaction).Model(&TransactionStatus{}).Where("hash = ?", transactionHash).Update("error", errorText).Error
}

// UpdatePenalty is updating penalty
func (ts *TransactionStatus) UpdatePenalty(transaction *DbTransaction, transactionHash []byte) error {
	return GetDB(transaction).Model(&TransactionStatus{}).Where("hash = ? AND penalty = 0", transactionHash).Update("penalty", 1).Error
}
