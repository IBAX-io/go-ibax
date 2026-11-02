/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// blockRun is the execution of the transactions of a block in the transaction of the block. It
// runs the transactions of b.Transactions it has not run each time it executes: a validator
// runs the batches of a streaming proposal as they come (F18 design 4.2), every other execution
// runs the block at once. The outputs of the accounts and the parameters of the ecosystems a run
// reads after the first are those of the chain before the block, as the first reads them.
type blockRun struct {
	b         *Block
	dbTx      *sqldb.DbTransaction
	afters    *types.AfterTxs
	processed [][]byte
	bad       chan badTxStruct
	fees      *smart.FeeSettlement
	kind      string
	start     time.Time
	keys      map[int64]bool
	ecos      map[int64]bool
	// ran is the number of the transactions of the block run
	ran int
	// state runs the transactions of a block that is not the first
	state *stateExec
	// stopped is a block the node generates whose transactions stopped
	stopped bool
	// failedAt is the position, from 1, of the transaction that failed the block of another node
	failedAt int
}

// beginRun begins the execution of the block
func (b *Block) beginRun(dbTx *sqldb.DbTransaction) *blockRun {
	r := &blockRun{
		b:         b,
		dbTx:      dbTx,
		afters:    &types.AfterTxs{Rts: make([]*types.RollbackTx, 0), Txs: make([]*types.AfterTx, 0)},
		processed: make([][]byte, 0, len(b.Transactions)),
		fees:      smart.NewFeeSettlement(),
		kind:      b.execKind(),
		start:     time.Now(),
		keys:      make(map[int64]bool),
		ecos:      make(map[int64]bool),
	}
	// a validator's trial of the block of another node bans no key and marks no transaction bad
	mark := !b.trial || b.GenBlock
	r.bad = make(chan badTxStruct)
	go func(ch chan badTxStruct, kind string) {
		for badTxItem := range ch {
			metrics.TxResults.WithLabelValues(kind, "bad").Inc()
			if !mark {
				continue
			}
			transaction.BadTxForBan(badTxItem.keyID)
			transaction.MarkTransactionBad(badTxItem.hash, badTxItem.msg)
		}
	}(r.bad, r.kind)
	b.OutputsMap = make(map[sqldb.KeyUTXO][]sqldb.SpentInfo)
	b.EcoParams = nil
	// UTXO multiple ecosystem fuelRate
	b.PrevSysPar = syspar.GetSysParCache()
	if b.GenBlock {
		b.size = newBlockSize(syspar.GetMaxBlockSize(), b.Header, b.PrevHeader)
	}
	return r
}

// prepare reads the outputs of the accounts and the parameters of the ecosystems of the
// transactions it has not read: those of the first run in the transaction of the block, those of
// a later run of the chain before the block
func (r *blockRun) prepare(txs []*transaction.Transaction) error {
	var keyIds, ecosystemIds []int64
	for _, t := range txs {
		if !r.keys[t.KeyID()] {
			r.keys[t.KeyID()] = true
			keyIds = append(keyIds, t.KeyID())
		}
		if t.IsSmartContract() && !r.ecos[t.SmartContract().TxSmart.EcosystemID] {
			r.ecos[t.SmartContract().TxSmart.EcosystemID] = true
			ecosystemIds = append(ecosystemIds, t.SmartContract().TxSmart.EcosystemID)
		}
	}
	db := r.dbTx
	if r.ran > 0 {
		db = nil
	}
	if len(keyIds) > 0 {
		outputs, err := sqldb.GetTxOutputs(db, keyIds)
		if err != nil {
			return err
		}
		if r.state != nil {
			r.state.st.AddOutputs(outputs)
		} else {
			sqldb.PutAllOutputsMap(outputs, r.b.OutputsMap)
		}
	}
	if len(ecosystemIds) > 0 {
		ecoParams, err := sqldb.GetEcoParam(db, ecosystemIds)
		if err != nil {
			return err
		}
		if r.state != nil {
			r.state.addEcoParams(ecoParams)
		} else {
			r.b.EcoParams = append(r.b.EcoParams, ecoParams...)
		}
	}
	return nil
}

// execute runs the transactions of the block it has not run
func (r *blockRun) execute() error {
	b := r.b
	if r.stopped || r.ran == len(b.Transactions) && r.ran > 0 {
		return nil
	}
	from := r.ran
	if err := r.prepare(b.Transactions[from:]); err != nil {
		return err
	}
	if from == 0 {
		b.stage(metrics.StagePrepare, r.start)
	}
	// the transactions run one after the other in the order of the block, which the node that
	// generates it stores as it runs them: every node runs them in the same order
	if b.IsGenesis() {
		txs := make([]*transaction.Transaction, 0, len(b.Transactions))
		for _, tx := range b.Transactions {
			t, err := transaction.UnmarshallTransaction(bytes.NewBuffer(tx.FullData), false)
			if err != nil {
				return err
			}
			txs = append(txs, t)
		}
		r.ran = len(b.Transactions)
		return b.executeTxs(r.dbTx, r.bad, r.afters, &r.processed, txs, r.fees)
	}
	if err := checkStopNetworkAlone(b.Transactions); err != nil {
		return err
	}
	if r.state == nil {
		state, err := b.beginState(r.dbTx)
		if err != nil {
			return err
		}
		r.state = state
	}
	r.ran = len(b.Transactions)
	return r.state.run(r, from, r.ran)
}

// end ends the execution: the fees of the block credited and its state written if it succeeded,
// the records of its transactions written whether or not
func (r *blockRun) end(err error) error {
	b := r.b
	if r.state != nil {
		r.state.stop()
		if err == nil {
			err = r.state.finish(r)
		}
	} else if err == nil && b.IsGenesis() {
		var rts []*types.RollbackTx
		if rts, err = r.fees.Settle(r.dbTx, b.Header); err != nil {
			err = fmt.Errorf("settling the fees: %w", err)
		} else {
			r.afters.Rts = append(r.afters.Rts, rts...)
		}
	}
	close(r.bad)
	metrics.TxResults.WithLabelValues(r.kind, "ok").Add(float64(len(r.processed)))
	b.stage(metrics.StageExecute, r.start)
	start := time.Now()
	defer b.stage(metrics.StageAfter, start)
	b.AfterTxs = r.afters
	if b.GenBlock {
		b.TxFullData = r.processed
	}
	if errA := b.AfterPlayTxs(r.dbTx); errA != nil {
		if err == nil {
			return errA
		}
		return fmt.Errorf("%v; %w", err, errA)
	}
	return err
}
