/*----------------------------------------------------------------
- Copyright (c) IBAX. All rights reserved.
- See LICENSE in the project root for license information.
---------------------------------------------------------------*/

package block

import (
	"strings"

	"github.com/IBAX-io/go-ibax/packages/common/random"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/notificator"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	log "github.com/sirupsen/logrus"
)

// PlaySafe is inserting block safely
func (b *Block) PlaySafe() error {
	logger := b.GetLogger()
	dbTx, err := sqldb.StartTransaction()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("starting db transaction")
		return err
	}

	inputTx := b.Transactions[:]
	err = b.Play(dbTx)
	if err != nil {
		dbTx.Rollback()
		if b.GenBlock && len(b.Transactions) == 0 {
			if err == transaction.ErrLimitStop {
				err = script.ErrVMTimeLimit
			}
			if inputTx[0].IsSmartContract() {
				transaction.BadTxForBan(inputTx[0].KeyID())
			}
			if err := transaction.MarkTransactionBad(dbTx, inputTx[0].Hash(), err.Error()); err != nil {
				return err
			}
		}
		return err
	}

	if b.GenBlock {
		if len(b.Transactions) == 0 {
			dbTx.Commit()
			return ErrEmptyBlock
		}

		//if len(inputTx) != len(b.Transactions) {
		b.Header.RollbacksHash, err = b.GetRollbacksHash(dbTx)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("getting rollbacks hash")
			return err
		}
		if err = b.repeatMarshallBlock(); err != nil {
			dbTx.Rollback()
			return err
		}
		//}
	}
	if err := b.InsertIntoBlockchain(dbTx); err != nil {
		dbTx.Rollback()
		return err
	}

	err = dbTx.Commit()
	if err != nil {
		return err
	}

	for _, q := range b.Notifications {
		q.Send()
	}
	return nil
}

func (b *Block) Play(dbTx *sqldb.DbTransaction) (batchErr error) {
	var (
		playTxs sqldb.AfterTxs
	)
	logger := b.GetLogger()
	limits := transaction.NewLimits(b.limitMode())
	rand := random.NewRand(b.Header.Time)
	processedTx := make([]*transaction.Transaction, 0, len(b.Transactions))
	defer func() {
		if b.GenBlock {
			b.TxExecutionSql = playTxs.TxExecutionSql
			b.Transactions = processedTx
		}
		if err := sqldb.AfterPlayTxs(dbTx, b.Header.BlockID, playTxs, b.GenBlock, b.IsGenesis()); err != nil {
			batchErr = err
			return
		}
	}()
	for curTx := 0; curTx < len(b.Transactions); curTx++ {
		t := b.Transactions[curTx]
		err := dbTx.Savepoint(consts.SetSavePointMarkBlock(curTx))
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": t.Hash()}).Error("using savepoint")
			return err
		}

		t.Notifications = notificator.NewQueue()
		t.DbTransaction = dbTx
		t.DbTransaction.ExecutionSql.Reset()
		t.TxCheckLimits = limits
		t.BlockHeader = b.Header
		t.PreBlockHeader = b.PrevHeader
		t.GenBlock = b.GenBlock
		t.SqlDbSavePoint = curTx
		t.Rand = rand.BytesSeed(t.Hash())
		err = t.Play()
		if err != nil {
			if err == transaction.ErrNetworkStopping {
				// Set the node in a pause state
				node.PauseNodeActivity(node.PauseTypeStopingNetwork)
				return err
			}
			errRoll := t.DbTransaction.RollbackSavepoint(consts.SetSavePointMarkBlock(curTx))
			if errRoll != nil {
				t.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": t.Hash()}).Error("rolling back to previous savepoint")
				return errRoll
			}
			if b.GenBlock {
				if err == transaction.ErrLimitStop {
					if curTx == 0 {
						return err
					}
					break
				}
				if strings.Contains(err.Error(), script.ErrVMTimeLimit.Error()) {
					err = script.ErrVMTimeLimit
				}
			}
			if t.IsSmartContract() {
				transaction.BadTxForBan(t.KeyID())
			}
			_ = transaction.MarkTransactionBad(t.DbTransaction, t.Hash(), err.Error())
			if t.SysUpdate {
				if err := syspar.SysUpdate(t.DbTransaction); err != nil {
					log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating syspar")
					return err
				}
				t.SysUpdate = false
			}
			if b.GenBlock {
				continue
			}

			return err
		}

		if t.SysUpdate {
			b.SysUpdate = true
			t.SysUpdate = false
		}
		if err := sqldb.SetTransactionStatusBlockMsg(t.DbTransaction, t.BlockHeader.BlockID, t.TxResult, t.Hash()); err != nil {
			t.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": t.Hash()}).Error("updating transaction status block id")
			return err
		}
		if t.Notifications.Size() > 0 {
			b.Notifications = append(b.Notifications, t.Notifications)
		}
		playTxs.UsedTx = append(playTxs.UsedTx, t.Hash())
		playTxs.TxExecutionSql = append(playTxs.TxExecutionSql, t.DbTransaction.ExecutionSql...)
		var (
			eco      int64
			contract string
		)
		if t.IsSmartContract() {
			eco = t.SmartContract().TxSmart.EcosystemID
			contract = t.SmartContract().TxContract.Name
		}
		playTxs.Lts = append(playTxs.Lts, &sqldb.LogTransaction{
			Block:        b.Header.BlockID,
			Hash:         t.Hash(),
			TxData:       t.FullData,
			Timestamp:    t.Timestamp(),
			Address:      t.KeyID(),
			EcosystemID:  eco,
			ContractName: contract,
		})
		playTxs.Rts = append(playTxs.Rts, t.RollBackTx...)
		processedTx = append(processedTx, t)
	}
	return nil
}
