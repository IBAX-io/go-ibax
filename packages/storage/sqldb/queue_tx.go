/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"github.com/shopspring/decimal"
)

// QueueTx is model
type QueueTx struct {
	Hash     []byte          `gorm:"primary_key;not null"`
	Data     []byte          `gorm:"not null"`
	FromGate int             `gorm:"not null"`
	Expedite decimal.Decimal `gorm:"not null"`
	Time     int64           `gorm:"not null"`
}

// TableName returns name of table
func (qt *QueueTx) TableName() string {
	return "queue_tx"
}

// DeleteTx is deleting tx
func (qt *QueueTx) DeleteTx(dbTx *DbTransaction) error {
	return GetDB(dbTx).Delete(qt).Error
}

// Save is saving model
func (qt *QueueTx) Save(dbTx *DbTransaction) error {
	return GetDB(dbTx).Save(qt).Error
}

// Create is creating record of model
func (qt *QueueTx) Create() error {
	return DBConn.Create(qt).Error
}

// GetByHash is retrieving model from database by hash
func (qt *QueueTx) GetByHash(dbTx *DbTransaction, hash []byte) (bool, error) {
	return isFound(GetDB(dbTx).Where("hash = ?", hash).First(qt))
}

// DeleteQueueTxByHash is deleting queue tx by hash
func DeleteQueueTxByHash(dbTx *DbTransaction, hash []byte) (int64, error) {
	query := GetDB(dbTx).Exec("DELETE FROM queue_tx WHERE hash = ?", hash)
	return query.RowsAffected, query.Error
}

// GetQueuedTransactionsCount counting queued transactions
func GetQueuedTransactionsCount(hash []byte) (int64, error) {
	var rowsCount int64
	err := DBConn.Table("queue_tx").Where("hash = ?", hash).Count(&rowsCount).Error
	return rowsCount, err
}

// GetAllUnverifiedAndUnusedTransactions is returns all unverified and unused transaction
func GetAllUnverifiedAndUnusedTransactions(dbTx *DbTransaction, limit int) ([]*QueueTx, error) {
	query := `SELECT *
		  FROM (
	              SELECT data,
	                     hash,expedite,time
	              FROM queue_tx
		      UNION
		      SELECT data,
			     hash,expedite,time
		      FROM transactions
		      WHERE verified = 0 AND used = 0
			)  AS x ORDER BY expedite DESC,time ASC limit ?`
	var result []*QueueTx
	err := GetDB(dbTx).Raw(query, limit).Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}

func DeleteQueueTxs(dbTx *DbTransaction, hs [][]byte) error {
	if len(hs) == 0 {
		return nil
	}
	return GetDB(dbTx).Delete(&QueueTx{}, hs).Error
}
