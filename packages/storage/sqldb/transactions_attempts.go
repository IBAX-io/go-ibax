/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

// TransactionsAttempts is model
type TransactionsAttempts struct {
	Hash    []byte `gorm:"primary_key;not null"`
	Attempt int8   `gorm:"not null"`
}

// TableName returns name of table
func (m TransactionsAttempts) TableName() string {
	return `transactions_attempts`
}

// GetByHash returns TransactionsAttempts existence by hash
func (ta *TransactionsAttempts) GetByHash(dbTx *DbTransaction, hash []byte) (bool, error) {
	return isFound(GetDB(dbTx).Where("hash = ?", hash).First(&ta))
}

// IncrementTxAttemptCount increases attempt column
func IncrementTxAttemptCount(dbTx *DbTransaction, transactionHash []byte) (int64, error) {
	ta := &TransactionsAttempts{}
	found, err := ta.GetByHash(dbTx, transactionHash)
	if err != nil {
		return 0, err
	}
	if found {
		if ta.Attempt > 125 {
			return int64(ta.Attempt), nil
		}
		err = GetDB(dbTx).Exec("update transactions_attempts set attempt=attempt+1 where hash = ?",
			transactionHash).Error
		if err != nil {
			return 0, err
		}
		ta.Attempt++
	} else {
		ta.Hash = transactionHash
		ta.Attempt = 1
		if err = GetDB(dbTx).Create(ta).Error; err != nil {
			return 0, err
		}
	}
	return int64(ta.Attempt), nil
}

func DecrementTxAttemptCount(dbTx *DbTransaction, transactionHash []byte) error {
	return GetDB(dbTx).Exec("update transactions_attempts set attempt=attempt-1 where hash = ?",
		transactionHash).Error
}

func FindTxAttemptCount(dbTx *DbTransaction, count int) ([]*TransactionsAttempts, error) {
	var rs []*TransactionsAttempts
	if err := GetDB(dbTx).Where("attempt > ?", count).Find(&rs).Error; err != nil {
		return rs, err
	}
	return rs, nil
}

// GetByHash returns TransactionsAttempts existence by hash
func DeleteTransactionsAttemptsByHash(dbTx *DbTransaction, hash []byte) error {
	return GetDB(dbTx).Table("transactions_attempts").Delete(&TransactionsAttempts{}, hash).Error
}
