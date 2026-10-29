/*----------------------------------------------------------------
- Copyright (c) IBAX. All rights reserved.
- See LICENSE in the project root for license information.
---------------------------------------------------------------*/

package block

import (
	"bytes"
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

	if !b.GenBlock {
		// The signatures of the transactions this node has not seen are verified in parallel
		// here, so that their execution finds them verified; an error is the execution's to report
		transaction.VerifyAll(b.Transactions)
	}
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

func (b *Block) ProcessTxs(dbTx *sqldb.DbTransaction) (err error) {
	afters := &types.AfterTxs{
		Rts: make([]*types.RollbackTx, 0),
		Txs: make([]*types.AfterTx, 0),
	}
	processedTx := make([][]byte, 0, len(b.Transactions))

	// a validator's trial of the block of another node bans no key and marks no transaction bad
	mark := !b.trial || b.GenBlock
	kind := b.execKind()
	start := time.Now()
	processBadTx := func() chan badTxStruct {
		ch := make(chan badTxStruct)
		go func() {
			for badTxItem := range ch {
				metrics.TxResults.WithLabelValues(kind, "bad").Inc()
				if !mark {
					continue
				}
				transaction.BadTxForBan(badTxItem.keyID)
				transaction.MarkTransactionBad(badTxItem.hash, badTxItem.msg)
			}
		}()
		return ch
	}

	txBadChan := processBadTx()
	defer func() {
		close(txBadChan)
		metrics.TxResults.WithLabelValues(kind, "ok").Add(float64(len(processedTx)))
		b.stage(metrics.StageExecute, start)
		start = time.Now()
		defer b.stage(metrics.StageAfter, start)
		if b.IsGenesis() || b.GenBlock || b.AfterTxs != nil {
			b.AfterTxs = afters
		}
		if b.GenBlock {
			b.TxFullData = processedTx
		}

		if errA := b.AfterPlayTxs(dbTx); errA != nil {
			if err == nil {
				err = errA
			} else if err != nil {
				err = fmt.Errorf("%v; %w", err, errA)
			}
			return
		}
	}()

	var keyIds []int64
	var keyIdsMap = make(map[int64]bool)
	var ecosystemIds []int64
	var ecosystemIdsMap = make(map[int64]bool)
	for indexTx := 0; indexTx < len(b.Transactions); indexTx++ {
		t := b.Transactions[indexTx]
		if !keyIdsMap[t.KeyID()] {
			keyIdsMap[t.KeyID()] = true
			keyIds = append(keyIds, t.KeyID())
		}
		if t.IsSmartContract() && !ecosystemIdsMap[t.SmartContract().TxSmart.EcosystemID] {
			ecosystemIdsMap[t.SmartContract().TxSmart.EcosystemID] = true
			ecosystemIds = append(ecosystemIds, t.SmartContract().TxSmart.EcosystemID)
		}
	}
	// query all keys utxo
	outputs, err := sqldb.GetTxOutputs(dbTx, keyIds)
	if err != nil {
		return err
	}
	b.OutputsMap = make(map[sqldb.KeyUTXO][]sqldb.SpentInfo)
	sqldb.PutAllOutputsMap(outputs, b.OutputsMap)
	// query all ecosystems combination percent
	ecoParams, err := sqldb.GetEcoParam(dbTx, ecosystemIds)
	if err != nil {
		return err
	}
	b.EcoParams = ecoParams
	// UTXO multiple ecosystem fuelRate
	b.PrevSysPar = syspar.GetSysParCache()
	if b.GenBlock {
		b.size = newBlockSize(syspar.GetMaxBlockSize(), b.Header, b.PrevHeader)
	}
	b.stage(metrics.StagePrepare, start)
	start = time.Now()
	// the transactions run one after the other in the order of the block, which the node that
	// generates it stores as it runs them: every node runs them in the same order
	txs := b.Transactions
	if b.IsGenesis() {
		txs = make([]*transaction.Transaction, 0, len(b.Transactions))
		for _, tx := range b.Transactions {
			t, err := transaction.UnmarshallTransaction(bytes.NewBuffer(tx.FullData), false)
			if err != nil {
				return err
			}
			txs = append(txs, t)
		}
	} else if err := checkStopNetworkAlone(txs); err != nil {
		return err
	}
	fees := smart.NewFeeSettlement()
	if !b.IsGenesis() {
		return b.executeState(dbTx, txBadChan, afters, &processedTx, txs, fees)
	}
	if err := b.executeTxs(dbTx, txBadChan, afters, &processedTx, txs, fees); err != nil {
		return err
	}
	rts, err := fees.Settle(dbTx, b.Header)
	if err != nil {
		return fmt.Errorf("settling the fees: %w", err)
	}
	afters.Rts = append(afters.Rts, rts...)
	return nil
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
				if err == transaction.ErrTimeLeft {
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
		var credits []smart.FeeCredit
		if t.IsSmartContract() {
			credits = t.SmartContract().FeeCredits
		}
		if b.GenBlock && !b.size.add(t.Compressed(), after, t.RollBackTx, fees.NewAccounts(credits)) {
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

		fees.Add(credits)
		afters.Txs = append(afters.Txs, after)
		afters.Rts = append(afters.Rts, t.RollBackTx...)
		*processedTx = append(*processedTx, t.FullData)

		sqldb.UpdateTxInputs(t.Hash(), b.Header.BlockId, t.TxInputsMap, b.OutputsMap)
		sqldb.InsertTxOutputs(t.Hash(), t.TxOutputsMap, b.OutputsMap)
	}
	b.execEnd = time.Now()
	return nil
}
