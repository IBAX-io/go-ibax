/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// UpsertBlockStatusBatches records the status of each transaction of a block, on every node that
// plays it: a node that did not take the transaction through /sendTx answers /txstatus for it too,
// so a client that moved to another node follows what it sent
func UpsertBlockStatusBatches(dbTx *gorm.DB, statuses []*TransactionStatus) error {
	rows := presentStatuses(statuses)
	if len(rows) == 0 {
		return nil
	}
	return dbTx.Clauses(upsertBlockStatusClause).Create(&rows).Error
}

// A transaction status the node has, as the one that took it through /sendTx has, takes the block's
var upsertBlockStatusClause = clause.OnConflict{
	Columns:   []clause.Column{{Name: "hash"}},
	DoUpdates: clause.AssignmentColumns([]string{"block_id", "error", "penalty"}),
}

func presentStatuses(statuses []*TransactionStatus) []*TransactionStatus {
	rows := make([]*TransactionStatus, 0, len(statuses))
	for _, s := range statuses {
		if s != nil {
			rows = append(rows, s)
		}
	}
	return rows
}

// SetError is updating transaction status error. The status of a transaction in a block stays: a copy
// sent again is refused as a duplicate, which says nothing of the one in the block
func (ts *TransactionStatus) SetError(dbTx *DbTransaction, errorText string, transactionHash []byte) error {
	return GetDB(dbTx).Model(&TransactionStatus{}).Where("hash = ? AND block_id = 0", transactionHash).Update("error", errorText).Error
}

// UpdatePenalty is updating penalty
func (ts *TransactionStatus) UpdatePenalty(dbTx *DbTransaction, transactionHash []byte) error {
	return GetDB(dbTx).Model(&TransactionStatus{}).Where("hash = ? AND penalty = 0", transactionHash).Update("penalty", int64(pbgo.TxInvokeStatusCode_PENALTY)).Error
}
