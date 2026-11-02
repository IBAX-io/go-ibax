/*----------------------------------------------------------------
- Copyright (c) IBAX. All rights reserved.
- See LICENSE in the project root for license information.
---------------------------------------------------------------*/

package block

import (
	"encoding/hex"
	"fmt"
	"slices"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/random"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/notificator"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// PlaySafe is inserting block safely
func (b *Block) PlaySafe() error {
	return b.playSafe(nil)
}

// verifySignatures verifies in parallel the signatures of the transactions of a block another
// node made, so that their execution finds them verified (plan 5.9): those this node verified when
// they arrived are in the cache of accepted signatures, and only those it has not seen are
// verified here. An error is the execution's to report, which checks the signature against the
// key the account registered. The transactions of a block the node generates were verified when
// they entered its queue. What it reads of the chain, the key an account registered, only fills
// the cache, the execution decides: Validate runs it before it locks the writes.
func (b *Block) verifySignatures() {
	if b.GenBlock || len(b.Transactions) == 0 {
		return
	}
	start := time.Now()
	transaction.VerifyAll(b.Transactions)
	b.stage(metrics.StageVerify, start)
}

// playSafe executes the block and inserts it, and runs also in the transaction of the block
func (b *Block) playSafe(also func(dbTx *sqldb.DbTransaction) error) error {
	logger := b.GetLogger()
	b.beginProfile()
	defer func() { b.prof = nil }()
	dbTx, err := sqldb.StartTransaction()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("starting db transaction")
		return err
	}

	b.verifySignatures()
	err = b.ProcessTxs(dbTx)
	if err != nil {
		dbTx.Rollback()
		return err
	}

	if b.GenBlock && len(b.TxFullData) == 0 {
		dbTx.Commit()
		return ErrEmptyBlock
	}

	start := time.Now()
	if err := b.InsertIntoBlockchain(dbTx); err != nil {
		dbTx.Rollback()
		return err
	}
	if also != nil {
		if err := also(dbTx); err != nil {
			dbTx.Rollback()
			return err
		}
	}
	if syspar.IsBftConsensus() {
		if err := storeValidators(dbTx, b.Header.BlockId+1); err != nil {
			dbTx.Rollback()
			return err
		}
	}
	b.stage(metrics.StageInsert, start)
	start = time.Now()
	err = dbTx.Commit()
	if err != nil {
		return err
	}
	b.stage(metrics.StageCommit, start)
	hashes := make([][]byte, len(b.Transactions))
	for i, t := range b.Transactions {
		hashes[i] = t.Hash()
	}
	transaction.TxPool.Remove(hashes)
	start = time.Now()
	for _, q := range b.Notifications {
		q.Send()
	}
	b.stage(metrics.StageNotify, start)
	b.endProfile(dbTx)
	return nil
}

type badTxStruct struct {
	index int
	hash  []byte
	msg   string
	keyID int64
}

// ProcessTxs runs the transactions of the block in the transaction of the block
func (b *Block) ProcessTxs(dbTx *sqldb.DbTransaction) error {
	r := b.beginRun(dbTx)
	return r.end(r.execute())
}

// checkStopNetworkAlone fails the transactions of a block if a StopNetwork transaction is among
// others: it stops the network, so no transaction runs after it, and none before it
func checkStopNetworkAlone(txs []*transaction.Transaction) error {
	if len(txs) > 1 && slices.ContainsFunc(txs, func(t *transaction.Transaction) bool { return t.Type() == types.StopNetworkTxType }) {
		return errStopNetworkNotAlone
	}
	return nil
}

// executeTxs runs the transactions in their order. A transaction that fails is left out of a block
// the node generates, and fails a block of another node
func (b *Block) executeTxs(dbTx *sqldb.DbTransaction, txBadChan chan badTxStruct, afters *types.AfterTxs, processedTx *[][]byte, txs []*transaction.Transaction,
	fees *smart.FeeSettlement) error {
	limits := transaction.NewLimits(b.limitMode())
	if !b.genStart.IsZero() {
		limits.StartAt(b.genStart, tail())
	}
	logger := b.GetLogger()
	for curTx := 0; curTx < len(txs); curTx++ {
		t := txs[curTx]
		start := time.Now()
		err := dbTx.Savepoint(consts.SetSavePointMarkBlock(hex.EncodeToString(t.Hash())))
		metrics.TxStage.WithLabelValues(metrics.TxSavepoint).Observe(metrics.Since(start))
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": t.Hash()}).Error("using savepoint")
			return err
		}
		err = t.WithOption(notificator.NewQueue(), b.GenBlock, b.Header, b.PrevHeader, dbTx, random.TxRand(t.Hash()), limits,
			consts.SetSavePointMarkBlock(hex.EncodeToString(t.Hash())), b.OutputsMap, b.PrevSysPar, b.EcoParams)
		if err != nil {
			return err
		}
		start = time.Now()
		err = t.Play()
		metrics.TxStage.WithLabelValues(metrics.TxPlay).Observe(metrics.Since(start))
		if err != nil {
			if err == transaction.ErrNetworkStopping {
				// Set the node in a pause state, when the block is not a trial
				if !b.trial {
					node.PauseNodeActivity(node.PauseTypeStopingNetwork)
				}
				return err
			}
			start = time.Now()
			errRoll := t.DbTransaction.RollbackSavepoint(consts.SetSavePointMarkBlock(hex.EncodeToString(t.Hash())))
			metrics.TxStage.WithLabelValues(metrics.TxRollback).Observe(metrics.Since(start))
			if errRoll != nil {
				return fmt.Errorf("%v; %w", err, errRoll)
			}
			if b.GenBlock {
				if errors.Is(err, transaction.ErrTimeLeft) {
					// the time left to the block stopped the transaction: it waits for the next
					// block, unless no transaction is in the block and none stopped before it,
					// so it had the most time a block leaves a transaction, and never fits
					if len(*processedTx) == 0 && !b.genStopped {
						txBadChan <- badTxStruct{index: curTx, hash: t.Hash(), msg: errTxTooLong.Error(), keyID: t.KeyID()}
					}
					b.genStopped = true
					break
				}
				if errors.Cause(err) == transaction.ErrLimitStop {
					if curTx == 0 {
						txBadChan <- badTxStruct{index: curTx, hash: t.Hash(), msg: err.Error(), keyID: t.KeyID()}
						return err
					}
					break
				}
			}
			txBadChan <- badTxStruct{index: curTx, hash: t.Hash(), msg: err.Error(), keyID: t.KeyID()}
			if t.SysUpdate {
				if err := syspar.SysUpdate(t.DbTransaction); err != nil {
					return fmt.Errorf("updating syspar: %w", err)
				}
				t.SysUpdate = false
			}
			if b.GenBlock {
				continue
			}
			return err
		}

		var (
			after    = &types.AfterTx{}
			eco      = int64(1)
			contract string
			code     pbgo.TxInvokeStatusCode
		)
		if t.IsSmartContract() {
			eco = t.SmartContract().TxSmart.EcosystemID
			code = t.TxResult.Code
			if t.SmartContract().TxContract != nil {
				contract = t.SmartContract().TxContract.Name
			}
		}
		after.UsedTx = t.Hash()
		after.Lts = &types.LogTransaction{
			Block: t.BlockHeader.BlockId,
			Hash:  t.Hash(),
			//TxData:       t.FullData,
			Timestamp:    t.Timestamp(),
			Address:      t.KeyID(),
			EcosystemId:  eco,
			ContractName: contract,
			InvokeStatus: code,
		}
		after.UpdTxStatus = t.TxResult

		// A transaction that would make the block longer than max_block_size waits for the next
		// block; one that does not fit even into an empty block never will.
		if b.GenBlock && !b.size.add(t.Compressed()) {
			if errRoll := t.DbTransaction.RollbackSavepoint(consts.SetSavePointMarkBlock(hex.EncodeToString(t.Hash()))); errRoll != nil {
				return errRoll
			}
			if len(*processedTx) == 0 {
				txBadChan <- badTxStruct{index: curTx, hash: t.Hash(), msg: "transaction is larger than an empty block", keyID: t.KeyID()}
				continue
			}
			break
		}

		if t.SysUpdate {
			b.SysUpdate = true
			t.SysUpdate = false
		}

		if t.Notifications.Size() > 0 {
			b.Notifications = append(b.Notifications, t.Notifications)
		}

		if t.IsSmartContract() {
			fees.Add(t.SmartContract().FeeCredits)
		}
		afters.Txs = append(afters.Txs, after)
		afters.Rts = append(afters.Rts, t.RollBackTx...)
		*processedTx = append(*processedTx, t.FullData)

		sqldb.UpdateTxInputs(t.Hash(), b.Header.BlockId, t.TxInputsMap, b.OutputsMap)
		sqldb.InsertTxOutputs(t.Hash(), t.TxOutputsMap, b.OutputsMap)
	}
	b.execEnd = time.Now()
	return nil
}
