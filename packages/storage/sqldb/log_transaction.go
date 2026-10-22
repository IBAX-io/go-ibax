/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"gorm.io/gorm"
)

// LogTransaction is model
type LogTransaction struct {
	Hash  []byte `gorm:"primary_key;not null"`
	Block int64  `gorm:"not null"`
	//TxData       []byte `gorm:"not null"`
	Timestamp    int64  `gorm:"not null"`
	Address      int64  `gorm:"not null"`
	EcosystemID  int64  `gorm:"not null"`
	Status       int64  `gorm:"not null"`
	ContractName string `gorm:"not null"`
}

// GetByHash returns LogTransactions existence by hash, for the API: archived ones included
func (lt *LogTransaction) GetByHash(hash []byte) (bool, error) {
	return isFound(archived("log_transactions").Where("hash = ?", hash).First(lt))
}

// KnownLogTransactions are the hashes of those in a block, in one query
func KnownLogTransactions(dbTx *DbTransaction, hashes [][]byte) (map[string]bool, error) {
	known := make(map[string]bool)
	if len(hashes) == 0 {
		return known, nil
	}
	var found [][]byte
	if err := GetDB(dbTx).Raw(`SELECT "hash" FROM "log_transactions" WHERE "hash" IN ?
		UNION ALL SELECT "hash" FROM "stop_network_txs" WHERE "hash" IN ?`, hashes, hashes).
		Scan(&found).Error; err != nil {
		return nil, err
	}
	for _, h := range found {
		known[string(h)] = true
	}
	return known, nil
}

// Create is creating record of model
func (lt *LogTransaction) Create(dbTx *DbTransaction) error {
	return GetDB(dbTx).Create(lt).Error
}

func CreateLogTransactionBatches(dbTx *gorm.DB, lts []*LogTransaction) error {
	if len(lts) == 0 {
		return nil
	}
	return dbTx.Model(&LogTransaction{}).Create(&lts).Error
}

// DeleteLogTransactionsByHash is deleting record by hash
func DeleteLogTransactionsByHash(dbTx *DbTransaction, hash []byte) (int64, error) {
	query := GetDB(dbTx).Exec("DELETE FROM log_transactions WHERE hash = ?", hash)
	if query.Error != nil {
		return 0, query.Error
	}
	if err := GetDB(dbTx).Exec(`DELETE FROM "stop_network_txs" WHERE "hash" = ?`, hash).Error; err != nil {
		return 0, err
	}
	return query.RowsAffected, nil
}

// CreateStopNetworkTxs keeps the hashes of the StopNetwork transactions of the block
func CreateStopNetworkTxs(dbTx *gorm.DB, hashes [][]byte, blockID int64) error {
	for _, h := range hashes {
		if err := dbTx.Exec(`INSERT INTO "stop_network_txs" ("hash", "block") VALUES (?, ?)`, h, blockID).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetLogTransactionsCount count records by transaction hash
func GetLogTransactionsCount(hash []byte) (int64, error) {
	var rowsCount int64
	if err := DBConn.Table("log_transactions").Where("hash = ?", hash).Count(&rowsCount).Error; err != nil {
		return -1, err
	}
	return rowsCount, nil
}

// GetLogTxCount count records by ecosystemID
func GetLogTxCount(dbTx *DbTransaction, ecosystemID int64) (int64, error) {
	var rowsCount int64
	if err := GetDB(dbTx).Table("log_transactions").Where("ecosystem_id = ? and status = 0", ecosystemID).Count(&rowsCount).Error; err != nil {
		return -1, err
	}
	return rowsCount, nil
}
