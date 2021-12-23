/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package rollback

import (
	"bytes"
	"errors"
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	log "github.com/sirupsen/logrus"
)

var (
	ErrLastBlock = errors.New("block is not the last")
)

// RollbackBlock is blocking rollback
func RollbackBlock(data []byte) error {
	bl, err := block.UnmarshallBlock(bytes.NewBuffer(data), true)
	if err != nil {
		return err
	}

	b := &sqldb.BlockChain{}
	if _, err = b.GetMaxBlock(); err != nil {
		return err
	}

	if b.ID != bl.Header.BlockID {
		return ErrLastBlock
	}

	dbTransaction, err := sqldb.StartTransaction()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("starting transaction")
		return err
	}

	err = rollbackBlock(dbTransaction, bl)
	if err != nil {
		dbTransaction.Rollback()
		return err
	}

	if err = b.DeleteById(dbTransaction, bl.Header.BlockID); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting block by id")
		dbTransaction.Rollback()
		return err
	}

	b = &sqldb.BlockChain{}
	if _, err = b.Get(bl.Header.BlockID - 1); err != nil {
		dbTransaction.Rollback()
		return err
	}

	bl, err = block.UnmarshallBlock(bytes.NewBuffer(b.Data), false)
	if err != nil {
		dbTransaction.Rollback()
		return err
	}

	ib := &sqldb.InfoBlock{
		Hash:           b.Hash,
		RollbacksHash:  b.RollbacksHash,
		BlockID:        b.ID,
		NodePosition:   strconv.Itoa(int(b.NodePosition)),
		KeyID:          b.KeyID,
		Time:           b.Time,
		CurrentVersion: strconv.Itoa(bl.Header.Version),
	}
	err = ib.Update(dbTransaction)
	if err != nil {
		dbTransaction.Rollback()
		return err
	}

	return dbTransaction.Commit()
}

func rollbackBlock(dbTransaction *sqldb.DbTransaction, block *block.Block) error {
	// rollback transactions in reverse order
	logger := block.GetLogger()
	for i := len(block.Transactions) - 1; i >= 0; i-- {
		t := block.Transactions[i]
		t.DbTransaction = dbTransaction

		_, err := sqldb.MarkTransactionUnusedAndUnverified(dbTransaction, t.TxHash())
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("starting transaction")
			return err
		}
		_, err = sqldb.DeleteLogTransactionsByHash(dbTransaction, t.TxHash())
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting log transactions by hash")
			return err
		}

		ts := &sqldb.TransactionStatus{}
		err = ts.UpdateBlockID(dbTransaction, 0, t.TxHash())
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating block id in transaction status")
			return err
		}

		_, err = sqldb.DeleteQueueTxByHash(dbTransaction, t.TxHash())
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting transacion from queue by hash")
			return err
		}

		switch t.Inner.(type) {
		case *transaction.SmartContractTransaction:
			if err = rollbackTransaction(t.TxHash(), t.DbTransaction, logger); err != nil {
				return err
			}
		}
		err = t.Inner.TxRollback()
		if err != nil {
			return err
		}
	}

	return nil
}
