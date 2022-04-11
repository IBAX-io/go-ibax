package sqldb

import (
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ArrHashes [][]byte

func DeleteQueueTxs(transaction *DbTransaction, hs [][]byte) error {
	return GetDB(transaction).Delete(&QueueTx{}, hs).Error
}

func DeleteTransactionsAttempts(transaction *DbTransaction, hs [][]byte) error {
	return GetDB(transaction).Delete(&TransactionsAttempts{}, hs).Error
}

func DeleteTransactions(dbTx *gorm.DB, hs [][]byte) error {
	if len(hs) == 0 {
		return nil
	}
	return dbTx.Delete(&Transaction{}, hs).Error
}

type AfterTxs struct {
	UsedTx      [][]byte
	Rts         []*RollbackTx
	Lts         []*LogTransaction
	UpdTxStatus []*updateBlockMsg
}

func AfterPlayTxs(dbTx *DbTransaction, blockID int64, playTx AfterTxs) error {
	return GetDB(dbTx).Transaction(func(tx *gorm.DB) error {
		if err := DeleteTransactions(tx, playTx.UsedTx); err != nil {
			return errors.Wrap(err, "batches delete used transactions")
		}
		if err := CreateLogTransactionBatches(tx, playTx.Lts); err != nil {
			return errors.Wrap(err, "batches insert log_transactions")
		}
		if err := CreateBatchesRollbackTx(tx, playTx.Rts); err != nil {
			return errors.Wrap(err, "batches insert rollback tx")
		}
		if err := UpdateBlockMsgBatches(tx, blockID); err != nil {
			return errors.Wrap(err, "batches update block msg transaction status")
		}

		return nil
	})
}
