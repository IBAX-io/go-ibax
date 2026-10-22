/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
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

// DeleteTransactionStatus deletes the status of a transaction whose block is rolled back: the
// transaction goes back to the pool, which answers for it
func DeleteTransactionStatus(dbTx *DbTransaction, transactionHash []byte) error {
	return GetDB(dbTx).Where("hash = ?", transactionHash).Delete(&TransactionStatus{}).Error
}

// UpsertBlockStatusBatches records the status of each transaction of a block, on every node that
// plays it: a node that did not take the transaction through /sendTx answers /txstatus for it too,
// so a client that moved to another node follows what it sent
func UpsertBlockStatusBatches(dbTx *gorm.DB, statuses []*TransactionStatus) error {
	rows := presentStatuses(statuses)
	if len(rows) == 0 {
		return nil
	}
	// a transaction status the node has, as the one that took it through /sendTx has, takes the block's
	return dbTx.Exec(`INSERT INTO "transactions_status" ("hash", "time", "type", "wallet_id", "block_id", "error", "penalty")
		SELECT * FROM unnest(?::bytea[], ?::bigint[], ?::integer[], ?::bigint[], ?::bigint[], ?::varchar[], ?::bigint[])
		ON CONFLICT ("hash") DO UPDATE SET "block_id" = "excluded"."block_id", "error" = "excluded"."error",
			"penalty" = "excluded"."penalty"`,
		column(rows, func(r *TransactionStatus) []byte { return r.Hash }),
		column(rows, func(r *TransactionStatus) int64 { return r.Time }),
		column(rows, func(r *TransactionStatus) int32 { return int32(r.Type) }),
		column(rows, func(r *TransactionStatus) int64 { return r.WalletID }),
		column(rows, func(r *TransactionStatus) int64 { return r.BlockID }),
		column(rows, func(r *TransactionStatus) string { return r.Error }),
		column(rows, func(r *TransactionStatus) int64 { return r.Penalty })).Error
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
