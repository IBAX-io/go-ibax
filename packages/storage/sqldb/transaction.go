/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This constants contains values of transactions priority
const (
	TransactionRateOnBlock transactionRate = iota + 1
	TransactionRateApiContract
	TransactionRateStopNetwork
)
const expediteOrder = `high_rate,expedite DESC,time ASC`

type transactionRate int8

// Transaction is model
type Transaction struct {
	Hash     []byte          `gorm:"private_key;not null"`
	Data     []byte          `gorm:"not null"`
	Used     int8            `gorm:"not null"`
	HighRate transactionRate `gorm:"not null"`
	Expedite decimal.Decimal `gorm:"not null"`
	Type     int8            `gorm:"not null"`
	KeyID    int64           `gorm:"not null"`
	Sent     int8            `gorm:"not null"`
	Verified int8            `gorm:"not null"`
	Time     int64           `gorm:"not null"`
}

// GetAllUnusedTransactions is retrieving all unused transactions
func GetAllUnusedTransactions(dbTx *DbTransaction, limit int) ([]*Transaction, error) {
	var transactions []*Transaction

	query := GetDB(dbTx).Where("used = ?", "0").Order(expediteOrder)
	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&transactions).Error; err != nil {
		return nil, err
	}
	return transactions, nil
}

// GetAllUnsentTransactions is retrieving all unset transactions
func GetAllUnsentTransactions(limit int) (*[]Transaction, error) {
	transactions := new([]Transaction)
	query := DBConn.Where("sent = ?", "0").Order(expediteOrder)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&transactions).Error; err != nil {
		return nil, err
	}
	return transactions, nil
}

// GetTransactionCountAll count all transactions
func GetTransactionCountAll() (int64, error) {
	var rowsCount int64
	if err := DBConn.Table("transactions").Count(&rowsCount).Error; err != nil {
		return -1, err
	}
	return rowsCount, nil
}

// GetTransactionsCount count all transactions by hash
func GetTransactionsCount(hash []byte) (int64, error) {
	var rowsCount int64
	if err := DBConn.Table("transactions").Where("hash = ?", hash).Count(&rowsCount).Error; err != nil {
		return -1, err
	}
	return rowsCount, nil
}

// DeleteTransactionByHash deleting transaction by hash
func DeleteTransactionByHash(dbTx *DbTransaction, hash []byte) error {
	return GetDB(dbTx).Where("hash = ?", hash).Delete(&Transaction{}).Error
}

// DeleteUsedTransactions deleting used transaction
func DeleteUsedTransactions(dbTx *DbTransaction) (int64, error) {
	query := GetDB(dbTx).Exec("DELETE FROM transactions WHERE used = 1")
	return query.RowsAffected, query.Error
}

// DeleteTransactionIfUnused deleting unused transaction
func DeleteTransactionIfUnused(dbTx *DbTransaction, transactionHash []byte) (int64, error) {
	query := GetDB(dbTx).Exec("DELETE FROM transactions WHERE hash = ? and used = 0 and verified = 0", transactionHash)
	return query.RowsAffected, query.Error
}

// MarkTransactionSent is marking transaction as sent
func MarkTransactionSent(transactionHash []byte) (int64, error) {
	query := DBConn.Exec("UPDATE transactions SET sent = 1 WHERE hash = ?", transactionHash)
	return query.RowsAffected, query.Error
}

// MarkTransactionSentBatches is marking transaction as sent
func MarkTransactionSentBatches(hashArr [][]byte) error {
	return DBConn.Exec("UPDATE transactions SET sent  = 1 WHERE hash in(?)", hashArr).Error
}

// MarkTransactionUsed is marking transaction as used
func MarkTransactionUsed(dbTx *DbTransaction, transactionHash []byte) (int64, error) {
	query := GetDB(dbTx).Exec("UPDATE transactions SET used = 1 WHERE hash = ?", transactionHash)
	return query.RowsAffected, query.Error
}

// MarkTransactionUnusedAndUnverified is marking transaction unused and unverified
func MarkTransactionUnusedAndUnverified(dbTx *DbTransaction, transactionHash []byte) (int64, error) {
	query := GetDB(dbTx).Exec("UPDATE transactions SET used = 0, verified = 0 WHERE hash = ?", transactionHash)
	return query.RowsAffected, query.Error
}

// MarkVerifiedAndNotUsedTransactionsUnverified is marking verified and unused transaction as unverified
func MarkVerifiedAndNotUsedTransactionsUnverified() (int64, error) {
	query := DBConn.Exec("UPDATE transactions SET verified = 0 WHERE verified = 1 AND used = 0")
	return query.RowsAffected, query.Error
}

// Read is checking transaction existence by hash
func (t *Transaction) Read(hash []byte) (bool, error) {
	return isFound(DBConn.Where("hash = ?", hash).First(t))
}

// Get is retrieving model from database
func (t *Transaction) Get(transactionHash []byte) (bool, error) {
	return isFound(DBConn.Where("hash = ?", transactionHash).First(t))
}

// GetVerified is checking transaction verification by hash
func (t *Transaction) GetVerified(transactionHash []byte) (bool, error) {
	return isFound(DBConn.Where("hash = ? AND verified = 1", transactionHash).First(t))
}

func (t *Transaction) BeforeCreate(db *gorm.DB) error {
	if t.HighRate == 0 {
		t.HighRate = GetTxRateByTxType(t.Type)
	}
	return nil
}

// Create is creating record of model
func (t *Transaction) Create(db *DbTransaction) error {
	return GetDB(db).Create(&t).Error
}

// CreateTransactionBatches is creating record of model
func CreateTransactionBatches(db *DbTransaction, trs []*Transaction) error {
	return GetDB(db).Clauses(clause.OnConflict{DoNothing: true}).Create(&trs).Error
}

func (t *Transaction) BeforeUpdate(db *gorm.DB) error {
	return db.Where("hash = ?", t.Hash).FirstOrCreate(&t).Error
}

func (t *Transaction) Update(db *DbTransaction) error {
	return GetDB(db).Where("hash = ?", t.Hash).Updates(&t).Error
}

func GetTxRateByTxType(txType int8) transactionRate {
	switch txType {
	case types.StopNetworkTxType:
		return TransactionRateStopNetwork
	default:
		return TransactionRateApiContract
	}
}

func GetManyTransactions(dbtx *DbTransaction, hashes [][]byte) ([]Transaction, error) {
	txes := []Transaction{}
	query := GetDB(dbtx).Where("hash in (?)", hashes).Find(&txes)
	if err := query.Error; err != nil {
		return nil, err
	}

	return txes, nil
}

func (t *Transaction) GetStopNetwork() (bool, error) {
	return isFound(DBConn.Where("type = ?", types.StopNetworkTxType).First(t))
}

func (t *Transaction) GetTransactionRateStopNetwork() bool {
	return t.HighRate == TransactionRateStopNetwork
}

func DeleteTransactions(dbTx *gorm.DB, hs [][]byte) error {
	if len(hs) == 0 {
		return nil
	}
	return dbTx.Delete(&Transaction{}, hs).Error
}
