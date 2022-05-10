package sqldb

import (
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ArrHashes [][]byte

func DeleteQueueTxs(dbTx *DbTransaction, hs [][]byte) error {
	return GetDB(dbTx).Delete(&QueueTx{}, hs).Error
}

func DeleteTransactionsAttempts(dbTx *DbTransaction, hs [][]byte) error {
	return GetDB(dbTx).Delete(&TransactionsAttempts{}, hs).Error
}

func DeleteTransactions(dbTx *gorm.DB, hs [][]byte) error {
	if len(hs) == 0 {
		return nil
	}
	return dbTx.Delete(&Transaction{}, hs).Error
}

type AfterTxs struct {
	UsedTx         [][]byte
	Rts            []*RollbackTx
	Lts            []*LogTransaction
	UpdTxStatus    []*updateBlockMsg
	TxExecutionSql []string
}

func AfterPlayTxs(dbTx *DbTransaction, blockID int64, playTx AfterTxs, genBlock, firstBlock bool) error {
	return GetDB(dbTx).Transaction(func(tx *gorm.DB) error {
		if !genBlock && !firstBlock {
			//for i := 0; i < len(playTx.TxExecutionSql); i++ {
			//	if err := tx.Exec(playTx.TxExecutionSql[i]).Error; err != nil {
			//		return errors.Wrap(err, "batches exec sql for tx")
			//	}
			//}
		}

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
