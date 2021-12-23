package sqldb

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	log "github.com/sirupsen/logrus"

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

func AfterPlayTxs(dbTx *DbTransaction, blockID int64, playTx AfterTxs, logger *log.Entry) error {
	return GetDB(dbTx).Transaction(func(tx *gorm.DB) error {
		if err := DeleteTransactions(tx, playTx.UsedTx); err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("batches delete used transactions")
			return err
		}
		if err := CreateLogTransactionBatches(tx, playTx.Lts); err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("batches insert log_transactions")
			return err
		}
		if err := CreateBatchesRollbackTx(tx, playTx.Rts); err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("batches insert rollback tx")
			return err
		}
		if err := UpdateBlockMsgBatches(tx, blockID); err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("batches update block msg transaction status")
			return err
		}

		return nil
	})
}
