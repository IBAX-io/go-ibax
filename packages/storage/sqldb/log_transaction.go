/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"slices"

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
	return dbTx.Exec(`INSERT INTO "log_transactions" ("hash", "block", "timestamp", "address", "ecosystem_id", "status", "contract_name")
		SELECT * FROM unnest(?::bytea[], ?::bigint[], ?::bigint[], ?::bigint[], ?::bigint[], ?::bigint[], ?::varchar[])`,
		column(lts, func(r *LogTransaction) []byte { return r.Hash }),
		column(lts, func(r *LogTransaction) int64 { return r.Block }),
		column(lts, func(r *LogTransaction) int64 { return r.Timestamp }),
		column(lts, func(r *LogTransaction) int64 { return r.Address }),
		column(lts, func(r *LogTransaction) int64 { return r.EcosystemID }),
		column(lts, func(r *LogTransaction) int64 { return r.Status }),
		column(lts, func(r *LogTransaction) string { return r.ContractName })).Error
}

// DeleteLogTransactionsByHash is deleting record by hash, with what outlives it
func DeleteLogTransactionsByHash(dbTx *DbTransaction, hash []byte) (int64, error) {
	var deleted []LogTransaction
	if err := GetDB(dbTx).Raw(`DELETE FROM "log_transactions" WHERE "hash" = ? RETURNING "ecosystem_id", "status"`, hash).
		Scan(&deleted).Error; err != nil {
		return 0, err
	}
	for _, lt := range deleted {
		if lt.Status != 0 {
			continue
		}
		if err := GetDB(dbTx).Exec(`UPDATE "log_transaction_counts" SET "count" = "count" - 1 WHERE "ecosystem_id" = ?`,
			lt.EcosystemID).Error; err != nil {
			return 0, err
		}
	}
	if err := GetDB(dbTx).Exec(`DELETE FROM "stop_network_txs" WHERE "hash" = ?`, hash).Error; err != nil {
		return 0, err
	}
	return int64(len(deleted)), nil
}

// CountLogTransactions adds the successful transactions of the block to the counts of their
// ecosystems, which the retention of log_transactions leaves
func CountLogTransactions(dbTx *gorm.DB, lts []*LogTransaction) error {
	counts := make(map[int64]int64)
	for _, lt := range lts {
		if lt.Status == 0 {
			counts[lt.EcosystemID]++
		}
	}
	ecosystems := make([]int64, 0, len(counts))
	for eco := range counts {
		ecosystems = append(ecosystems, eco)
	}
	slices.Sort(ecosystems)
	for _, eco := range ecosystems {
		if err := dbTx.Exec(`INSERT INTO "log_transaction_counts" ("ecosystem_id", "count") VALUES (?, ?)
			ON CONFLICT ("ecosystem_id") DO UPDATE SET "count" = "log_transaction_counts"."count" + EXCLUDED."count"`,
			eco, counts[eco]).Error; err != nil {
			return err
		}
	}
	return nil
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

// GetLogTxCount is the count of the successful transactions of the ecosystem since the chain began
func GetLogTxCount(dbTx *DbTransaction, ecosystemID int64) (int64, error) {
	var count []int64
	if err := GetDB(dbTx).Raw(`SELECT "count" FROM "log_transaction_counts" WHERE "ecosystem_id" = ?`, ecosystemID).
		Scan(&count).Error; err != nil {
		return -1, err
	}
	if len(count) == 0 {
		return 0, nil
	}
	return count[0], nil
}
