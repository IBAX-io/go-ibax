/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/pkg/errors"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

var (
	ErrDuplicatedTx = errors.New("Duplicated transaction")
	ErrNotComeTime  = errors.New("Transaction processing time has not come")
	ErrExpiredTime  = errors.New("Transaction processing time is expired")
	ErrEarlyTime    = utils.WithBan(errors.New("Early transaction time"))
	ErrEmptyKey     = utils.WithBan(errors.New("KeyID is empty"))
)

// InsertInLogTx is inserting tx in log
//func InsertInLogTx(t *Transaction, blockID int64) error {
//	ltx := &sqldb.LogTransaction{Hash: t.TxHash, Block: blockID}
//	if err := ltx.Create(t.DbTransaction); err != nil {
//		log.WithFields(log.Fields{"error": err, "type": consts.DBError}).Error("insert logged transaction")
//		return utils.ErrInfo(err)
//	}
//	return nil
//}

// CheckLogTx checks if this transaction exists
// And it would have successfully passed a frontal test
func CheckLogTx(txHash []byte, logger *log.Entry) error {
	logTx := &sqldb.LogTransaction{}
	found, err := logTx.GetByHash(nil, txHash)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting log transaction by hash")
		return err
	}
	if found {
		logger.WithFields(log.Fields{"tx_hash": txHash, "type": consts.DuplicateObject}).Warning("double tx in log transactions")
		return ErrDuplicatedTx
	}
	return nil
}

// DeleteQueueTx deletes a transaction from the queue
func DeleteQueueTx(dbTx *sqldb.DbTransaction, hash []byte) error {
	delQueueTx := &sqldb.QueueTx{Hash: hash}
	err := delQueueTx.DeleteTx(dbTx)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Debug("deleting transaction from queue")
		return err
	}
	// Because we process transactions with verified=0 in queue_parser_tx, after processing we need to delete them
	err = sqldb.DeleteTransactionByHash(dbTx, hash)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Debug("deleting transaction if unused")
		return err
	}
	//err = sqldb.DeleteTransactionsAttemptsByHash(dbTx, hash)
	//if err != nil {
	//	log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Debug("deleting DeleteTransactionsAttemptsByHash")
	//	return err
	//}
	return nil
}

func MarkTransactionBad(hash []byte, errText string) error {
	if hash == nil {
		return nil
	}
	if len(errText) > 255 {
		errText = errText[:255] + "..."
	}
	log.WithFields(log.Fields{"type": consts.BadTxError, "tx_hash": hash, "error": errText}).Debug("tx marked as bad")

	return sqldb.NewDbTransaction(sqldb.DBConn).Connection().Transaction(func(tx *gorm.DB) error {
		// looks like there is no hash in queue_tx at this moment
		qtx := &sqldb.QueueTx{}
		_, err := qtx.GetByHash(sqldb.NewDbTransaction(tx), hash)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Debug("getting tx by hash from queue")
			return err
		}

		if qtx.FromGate == 0 {
			m := &sqldb.TransactionStatus{}
			err = m.SetError(sqldb.NewDbTransaction(tx), errText, hash)
			if err != nil {
				log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Debug("setting transaction status error")
				return err
			}
		}
		err = DeleteQueueTx(sqldb.NewDbTransaction(tx), hash)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Debug("deleting transaction from queue")
			return err
		}
		return nil
	})
}

// ProcessQueueTransaction writes transactions into the queue
//func ProcessQueueTransaction(dbTx *sqldb.DbTransaction, hash, binaryTx []byte, myTx bool) error {
//	t, err := UnmarshallTransaction(bytes.NewBuffer(binaryTx), true)
//	if err != nil {
//		return err
//	}
//
//	if err = t.Check(time.Now().Unix(), true); err != nil {
//		if err != ErrEarlyTime {
//			return err
//		}
//		return nil
//	}
//
//	if t.TxKeyID == 0 {
//		errStr := "undefined keyID"
//		return errors.New(errStr)
//	}
//	var found bool
//	tx := &sqldb.Transaction{}
//	found, err = tx.Get(hash)
//	if err != nil {
//		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting transaction by hash")
//		return utils.ErrInfo(err)
//	}
//	if found {
//		err = sqldb.DeleteTransactionByHash(dbTx, hash)
//		if err != nil {
//			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting transaction by hash")
//			return utils.ErrInfo(err)
//		}
//	}
//	// put with verified=1
//	var expedite decimal.Decimal
//	if len(t.TxSmart.GetExpedite) > 0 {
//		expedite, err = decimal.NewFromString(t.TxSmart.GetExpedite)
//		if err != nil {
//			return utils.ErrInfo(err)
//		}
//	}
//	newTx := &sqldb.Transaction{
//		Hash:     hash,
//		Data:     binaryTx,
//		Type:     int8(t.TxType),
//		KeyID:    t.TxKeyID,
//		GetExpedite: expedite,
//		Time:     t.TxTime,
//		Verified: 1,
//	}
//	err = newTx.Create()
//	if err != nil {
//		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating new transaction")
//		return utils.ErrInfo(err)
//	}
//
//	delQueueTx := &sqldb.QueueTx{Hash: hash}
//	if err = delQueueTx.DeleteTx(dbTx); err != nil {
//		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting transaction from queue")
//		return utils.ErrInfo(err)
//	}
//
//	return nil
//}

// ProcessTransactionsQueue parses new transactions
func ProcessTransactionsQueue(dbTx *sqldb.DbTransaction) error {
	all, err := sqldb.GetAllUnverifiedAndUnusedTransactions(dbTx, syspar.GetMaxTxCount())
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting all unverified and unused transactions")
		return err
	}
	//for i := 0; i < len(all); i++ {
	//	err := ProcessQueueTransaction(dbTx, all[i].Hash, all[i].Data, false)
	//	if err != nil {
	//		MarkTransactionBad(dbTx, all[i].Hash, err.Error())
	//		return utils.ErrInfo(err)
	//	}
	//	log.Debug("transaction parsed successfully")
	//}
	return ProcessQueueTransactionBatches(dbTx, all)
}

// AllTxParser parses new transactions
func ProcessTransactionsAttempt(dbTx *sqldb.DbTransaction) error {
	all, err := sqldb.FindTxAttemptCount(dbTx, consts.MaxTXAttempt)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting all  transactions attempt > consts.MaxTXAttempt")
		return err
	}
	for _, data := range all {
		err := MarkTransactionBad(data.Hash, fmt.Sprintf("The limit of %d attempts has been reached", consts.MaxTXAttempt))
		if err != nil {
			return utils.ErrInfo(err)
		}
		log.Debug("transaction attempt deal successfully")
	}
	return nil
}
