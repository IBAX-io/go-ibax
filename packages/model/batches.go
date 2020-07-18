package model

import (
	"errors"

	log "github.com/sirupsen/logrus"
	"github.com/IBAX-io/go-ibax/packages/consts"

	"gorm.io/gorm"
)

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

type Batcher interface {
	BatchFindByHash(*DbTransaction, ArrHashes) error
}
type ArrHashes [][]byte
type logTxser []LogTransaction
type txser []Transaction
type queueser []QueueTx

func (l logTxser) BatchFindByHash(tr *DbTransaction, hs ArrHashes) error {
	if result := GetDB(tr).Model(&LogTransaction{}).Select("hash").Where("hash IN ?", hs).FindInBatches(&l, len(hs), func(tx *gorm.DB, batch int) error {
		if tx.RowsAffected > 0 {
			return errors.New("duplicated transaction at log_transactions")
			return errors.New("duplicated transaction at transactions")
		}
		return nil
	}); result.Error != nil {
		return result.Error
	}
	return nil
}

func (l queueser) BatchFindByHash(tr *DbTransaction, hs ArrHashes) error {
	if result := GetDB(tr).Model(&QueueTx{}).Select("hash").Where("hash IN ?", hs).FindInBatches(&l, len(hs), func(tx *gorm.DB, batch int) error {
		if tx.RowsAffected > 0 {
			return errors.New("duplicated transaction at queue_tx")
		}
		return nil
	}); result.Error != nil {
		return result.Error
	}
	return nil
}

func CheckDupTx(transaction *DbTransaction, hs ArrHashes) error {
	var (
		logTxs = new(logTxser)
		txs    = new(txser)
		queues = new(queueser) //old not check that
		batch  []Batcher
	)
	batch = append(batch, logTxs, txs, queues)
	for _, d := range batch {
		err := d.BatchFindByHash(transaction, hs)
		if err != nil {
			return err
		}
	}
	return nil
}
