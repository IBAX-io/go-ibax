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

	if b.ID != bl.Header.BlockId {
		return ErrLastBlock
	}

	dbTx, err := sqldb.StartTransaction()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("starting transaction")
		return err
	}

	err = rollbackBlock(dbTx, bl)
	if err != nil {
		dbTx.Rollback()
		return err
	}

	if err = b.DeleteById(dbTx, bl.Header.BlockId); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting block by id")
		dbTx.Rollback()
		return err
	}

	b = &sqldb.BlockChain{}
	if _, err = b.Get(bl.Header.BlockId - 1); err != nil {
		dbTx.Rollback()
		return err
	}

	bl, err = block.UnmarshallBlock(bytes.NewBuffer(b.Data), false)
	if err != nil {
		dbTx.Rollback()
		return err
	}

	ib := &sqldb.InfoBlock{
		Hash:           b.Hash,
		RollbacksHash:  b.RollbacksHash,
		BlockID:        b.ID,
		NodePosition:   strconv.Itoa(int(b.NodePosition)),
		KeyID:          b.KeyID,
		Time:           b.Time,
		CurrentVersion: strconv.Itoa(int(bl.Header.Version)),
	}
	err = ib.Update(dbTx)
	if err != nil {
		dbTx.Rollback()
		return err
	}

	return dbTx.Commit()
}

func rollbackBlock(dbTx *sqldb.DbTransaction, block *block.Block) error {
	// rollback transactions in reverse order
	logger := block.GetLogger()
	for i := len(block.Transactions) - 1; i >= 0; i-- {
		t := block.Transactions[i]
		t.DbTransaction = dbTx

		_, err := sqldb.MarkTransactionUnusedAndUnverified(dbTx, t.Hash())
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("starting transaction")
			return err
		}
		_, err = sqldb.DeleteLogTransactionsByHash(dbTx, t.Hash())
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting log transactions by hash")
			return err
		}

		ts := &sqldb.TransactionStatus{}
		err = ts.UpdateBlockID(dbTx, 0, t.Hash())
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating block id in transaction status")
			return err
		}

		_, err = sqldb.DeleteQueueTxByHash(dbTx, t.Hash())
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting transacion from queue by hash")
			return err
		}

		switch t.Inner.(type) {
		case *transaction.SmartTransactionParser:
			if err = rollbackTransaction(t.Hash(), t.DbTransaction, logger); err != nil {
				return err
			}
		}
		err = t.Inner.TxRollback()
		if err != nil {
			return err
		}
	}

	err := sqldb.RollbackOutputs(block.Header.BlockId, dbTx, logger)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating outputs by block id")
		return err
	}

	return nil
}
