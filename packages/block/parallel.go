/*----------------------------------------------------------------
- Copyright (c) IBAX. All rights reserved.
- See LICENSE in the project root for license information.
---------------------------------------------------------------*/

package block

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/random"
	"github.com/IBAX-io/go-ibax/packages/conf"
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
	pkgerrors "github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// The transactions of a block run in the state of the block, in memory, which writes what they
// changed to PostgreSQL at the end of the block. Workers run them ahead in parallel, each in a
// view of the state; the block commits the views in its order, once their reads hold. A
// transaction whose run does not hold runs again once the transactions before it committed, and
// one that changes what the node keeps out of the database, or that the state cannot run in
// parallel, runs in order while no worker runs. Every node commits what the transactions run in
// the order of the block do, so the block is the same whatever the workers.

// execWorkers is the number of the workers that run the transactions of a block in parallel
func execWorkers() int {
	if n := conf.Config.Exec.Workers; n > 0 {
		return n
	}
	return runtime.NumCPU()
}

// maxRestarts bounds the runs in order of a transaction that each put more tables in PostgreSQL
const maxRestarts = 64

// errRestarted is the error of a settlement whose view has to run again
var errRestarted = errors.New("the settlement runs again after the state writes tables to PostgreSQL")

// errRestarts is a transaction that does not run in order however many tables go to PostgreSQL
var errRestarts = errors.New("the transaction does not run in order in the state of the block")

// txRun is a run of a transaction of the block on a clone, in a view of the state
type txRun struct {
	t *transaction.Transaction
	v *sqldb.TxView
	// err is the error of the run of the transaction
	err error
	// fatal is an error of the block, out of the run of the transaction
	fatal error
	done  chan struct{}
}

// runJob is a transaction a worker runs
type runJob struct {
	i   int
	t   *transaction.Transaction
	run *txRun
}

// stateExec runs the transactions of a block in its state. The block gives it its transactions
// in parts, which the workers run ahead as they come.
type stateExec struct {
	b      *Block
	dbTx   *sqldb.DbTransaction
	st     *sqldb.BlockState
	limits *transaction.Limits
	// txs are the transactions of the block and runs their runs ahead, which the block reads
	// alone; the workers have their transactions and runs by their jobs
	txs  []*transaction.Transaction
	runs []*txRun
	// ecoMu guards ecoParams, the parameters of the ecosystems of the transactions, which grow
	// with the transactions while the workers run
	ecoMu     sync.RWMutex
	ecoParams []sqldb.EcoParam
	// the workers, which start with the first part of more than one transaction: they take the
	// parts from jobs, and run at most window ahead of the block
	jobs    chan []runJob
	window  chan struct{}
	stopRun func()
	// node is held to read, by the runs in parallel, and to write, by the runs in order, what
	// the node keeps out of the database: the contracts of the VM and the parameters
	node sync.RWMutex
	// again is the connection of the runs the commit runs again in parallel, which it pins at
	// its first
	again *sql.Conn
}

// contractOf is the name of the contract of the transaction
func contractOf(t *transaction.Transaction) string {
	if t.IsSmartContract() && t.SmartContract().TxContract != nil {
		return t.SmartContract().TxContract.Name
	}
	return ""
}

// addEcoParams adds the parameters of ecosystems the transactions before did not have
func (e *stateExec) addEcoParams(params []sqldb.EcoParam) {
	e.ecoMu.Lock()
	e.ecoParams = append(slices.Clip(e.ecoParams), params...)
	e.ecoMu.Unlock()
}

func (e *stateExec) ecosystems() []sqldb.EcoParam {
	e.ecoMu.RLock()
	defer e.ecoMu.RUnlock()
	return e.ecoParams
}

// play runs the transaction t, the i-th of the block, on a clone in the view, with the limits
func (e *stateExec) play(t *transaction.Transaction, v *sqldb.TxView, limits *transaction.Limits) (r txRun) {
	b := e.b
	r.t, r.v = t.Clone(), v
	vtx := v.Transaction()
	mark := consts.SetSavePointMarkBlock(hex.EncodeToString(r.t.Hash()))
	if r.fatal = vtx.Savepoint(mark); r.fatal != nil {
		return
	}
	r.fatal = r.t.WithOption(notificator.NewQueue(), b.GenBlock, b.Header, b.PrevHeader, vtx, random.TxRand(r.t.Hash()), limits,
		mark, b.OutputsMap, b.PrevSysPar, e.ecosystems())
	if r.fatal != nil {
		return
	}
	start := time.Now()
	r.err = r.t.Play()
	metrics.TxStage.WithLabelValues(metrics.TxPlay).Observe(metrics.Since(start))
	if b.GenBlock && r.err == nil {
		// the block the node generates stores the transaction compressed: the worker compresses
		// it, not the commit
		r.t.Compressed()
	}
	// a run that has to run again takes back what it wrote, though the contract that met the
	// statement after which it runs again failed and paid, and the transaction gave no error
	if (r.err != nil && r.err != transaction.ErrNetworkStopping) || v.Restart() {
		start = time.Now()
		errRoll := vtx.RollbackSavepoint(mark)
		metrics.TxStage.WithLabelValues(metrics.TxRollback).Observe(metrics.Since(start))
		if errRoll != nil {
			r.fatal = fmt.Errorf("%v; %w", r.err, errRoll)
		}
	}
	return
}

// parallel runs the transaction t, the i-th of the block, in parallel on the connection conn,
// with limits that check nothing: the block checks them in its order
func (e *stateExec) parallel(i int, t *transaction.Transaction, conn *sql.Conn) txRun {
	r := e.play(t, e.st.NewParallelView(i, contractOf(t), conn), e.limits.Deferred())
	e.st.SetRunIDs(i, r.v.IDCounts())
	return r
}

// reloadSysPar reads again the parameters of the platform a failed run changed in memory
func (e *stateExec) reloadSysPar(i int) error {
	if err := syspar.SysUpdate(e.st.NewOrderedView(i, "").Transaction()); err != nil {
		return fmt.Errorf("updating syspar: %w", err)
	}
	e.st.SysUpdate()
	return nil
}

// ordered runs the transaction i in order, while no worker runs, with the limits of the block
func (e *stateExec) ordered(i int) txRun {
	e.node.Lock()
	defer e.node.Unlock()
	for range maxRestarts {
		r := e.play(e.txs[i], e.st.NewOrderedView(i, contractOf(e.txs[i])), e.limits)
		if r.v.Barrier() {
			e.st.SysUpdate()
		}
		if r.fatal != nil || !r.v.Restart() {
			return r
		}
		if r.t.SysUpdate {
			if r.fatal = e.reloadSysPar(i); r.fatal != nil {
				return r
			}
		}
		if r.fatal = r.v.Prepare(); r.fatal != nil {
			return r
		}
	}
	return txRun{fatal: errRestarts}
}

// settle credits the fees of the block in order after its transactions
func (e *stateExec) settle(fees *smart.FeeSettlement) ([]*types.RollbackTx, error) {
	const mark = "settlement"
	i := len(e.txs)
	for range maxRestarts {
		v := e.st.NewOrderedView(i, "")
		vtx := v.Transaction()
		if err := vtx.Savepoint(mark); err != nil {
			return nil, err
		}
		rts, err := fees.Settle(vtx, e.b.Header)
		if err == nil && v.Restart() {
			err = errRestarted
		}
		if err == nil {
			e.st.Commit(i, v)
			return rts, nil
		}
		if errRoll := vtx.RollbackSavepoint(mark); errRoll != nil {
			return nil, fmt.Errorf("%v; %w", err, errRoll)
		}
		if !v.Restart() {
			return nil, fmt.Errorf("settling the fees: %w", err)
		}
		if err := v.Prepare(); err != nil {
			return nil, err
		}
	}
	return nil, errRestarts
}

// runAgain runs the transaction i again in parallel, on the connection of the commit
func (e *stateExec) runAgain(i int) txRun {
	if e.again == nil {
		conn, err := sqldb.PinConn(context.Background())
		if err != nil {
			return txRun{fatal: err}
		}
		e.again = conn
	}
	return e.parallel(i, e.txs[i], e.again)
}

// startWorkers starts the workers on the transactions of the parts of jobs in their order, at
// most window ahead of the block, each on a connection it holds through the block. The block
// takes a run of each transaction once it is done, and frees its room in the window. stopRun
// stops them and gives the connections back.
func (e *stateExec) startWorkers(workers int) error {
	conns := make([]*sql.Conn, 0, workers)
	release := func() {
		for _, c := range conns {
			c.Close()
		}
	}
	for range workers {
		c, err := sqldb.PinConn(context.Background())
		if err != nil {
			release()
			return err
		}
		conns = append(conns, c)
	}
	e.jobs = make(chan []runJob, 1024)
	e.window = make(chan struct{}, 4*workers)
	quit := make(chan struct{})
	next := make(chan runJob)
	go func() {
		defer close(next)
		for {
			var part []runJob
			select {
			case part = <-e.jobs:
			case <-quit:
				return
			}
			for _, j := range part {
				select {
				case e.window <- struct{}{}:
				case <-quit:
					return
				}
				select {
				case next <- j:
				case <-quit:
					return
				}
			}
		}
	}()
	var wg sync.WaitGroup
	for _, conn := range conns {
		wg.Go(func() {
			for j := range next {
				e.node.RLock()
				r := e.parallel(j.i, j.t, conn)
				e.node.RUnlock()
				r.done = j.run.done
				*j.run = r
				close(r.done)
			}
		})
	}
	e.stopRun = func() {
		close(quit)
		wg.Wait()
		release()
	}
	return nil
}

// beginState begins the state of a block that is not the first
func (b *Block) beginState(dbTx *sqldb.DbTransaction) (*stateExec, error) {
	limits := transaction.NewLimits(b.limitMode())
	if !b.genStart.IsZero() {
		limits.StartAt(b.genStart, tail())
	}
	st, err := sqldb.NewBlockState(dbTx, nil)
	if err != nil {
		return nil, err
	}
	st.SetOutputs(b.OutputsMap)
	return &stateExec{b: b, dbTx: dbTx, st: st, limits: limits, ecoParams: b.EcoParams}, nil
}

// run runs the transactions from to to of the block in its state, in parallel, and commits them
// in their order, as executeTxs runs them
func (e *stateExec) run(r *blockRun, from, to int) error {
	b := e.b
	txs := b.Transactions[from:to]
	contracts := make([]string, len(txs))
	part := make([]runJob, len(txs))
	for k, t := range txs {
		contracts[k] = contractOf(t)
		run := &txRun{done: make(chan struct{})}
		e.runs = append(e.runs, run)
		part[k] = runJob{i: from + k, t: t, run: run}
	}
	e.st.Extend(contracts)
	e.txs = append(e.txs, txs...)
	if e.jobs == nil {
		if workers := min(execWorkers(), len(txs)); workers > 1 {
			if err := e.startWorkers(workers); err != nil {
				return err
			}
		}
	}
	queued := e.jobs != nil
	if queued {
		e.jobs <- part
	}
	err := b.commitState(e, r, from, to, queued)
	b.execEnd = time.Now()
	return err
}

// stop stops the workers
func (e *stateExec) stop() {
	if e.stopRun != nil {
		e.stopRun()
		e.stopRun = nil
	}
	if e.again != nil {
		e.again.Close()
		e.again = nil
	}
}

// finish credits the fees of the block and writes its state to PostgreSQL
func (e *stateExec) finish(r *blockRun) error {
	rts, err := e.settle(r.fees)
	if err != nil {
		return err
	}
	r.afters.Rts = append(r.afters.Rts, rts...)
	if err := e.st.Flush(); err != nil {
		return fmt.Errorf("writing the state of the block: %w", err)
	}
	e.st.End()
	e.dbTx.ForgetCache()
	return nil
}

// commitState takes the runs of the transactions from to to in their order, runs again those
// that do not hold, and commits them. A transaction that fails is left out of a block the node
// generates, and fails a block of another node. b.Transactions takes the runs it commits.
func (b *Block) commitState(e *stateExec, br *blockRun, from, to int, queued bool) error {
	st := e.st
	txs, txBadChan, afters, processedTx, fees := b.Transactions, br.bad, br.afters, &br.processed, br.fees
	for curTx := from; curTx < to; curTx++ {
		var r txRun
		if queued {
			<-e.runs[curTx].done
			r = *e.runs[curTx]
			<-e.window
		}
		if r.fatal != nil {
			return r.fatal
		}
		// a transfer of the network runs in order; so does a run that met what the state runs
		// only in order
		run := "parallel"
		if r.v == nil || !txs[curTx].IsSmartContract() {
			r, run = e.ordered(curTx), "ordered"
		} else if needs, _ := r.v.NeedsOrder(); needs {
			r, run = e.ordered(curTx), "ordered"
		} else if !st.Holds(r.v) {
			// the transactions before it committed: a run now holds, unless it needs the order
			if r, run = e.runAgain(curTx), "again"; r.fatal == nil {
				if needs, _ := r.v.NeedsOrder(); needs || !st.Holds(r.v) {
					r, run = e.ordered(curTx), "ordered"
				}
			}
		}
		metrics.TxRuns.WithLabelValues(run).Inc()
		if r.fatal != nil {
			return r.fatal
		}
		t, err := r.t, r.err
		if err == nil && !r.v.Ordered() {
			err = t.CheckLimits(e.limits)
		}

		if err != nil {
			st.Skip(curTx)
			if !b.GenBlock {
				br.failedAt = curTx + 1
			}
			if err == transaction.ErrNetworkStopping {
				// Set the node in a pause state, when the block is not a trial
				if !b.trial {
					node.PauseNodeActivity(node.PauseTypeStopingNetwork)
				}
				return err
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
					br.stopped = true
					break
				}
				if pkgerrors.Cause(err) == transaction.ErrLimitStop {
					if curTx == 0 {
						txBadChan <- badTxStruct{index: curTx, hash: t.Hash(), msg: err.Error(), keyID: t.KeyID()}
						return err
					}
					br.stopped = true
					break
				}
			}
			txBadChan <- badTxStruct{index: curTx, hash: t.Hash(), msg: err.Error(), keyID: t.KeyID()}
			if t.SysUpdate {
				e.node.Lock()
				err := e.reloadSysPar(curTx)
				e.node.Unlock()
				if err != nil {
					return err
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
			Block:        t.BlockHeader.BlockId,
			Hash:         t.Hash(),
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
			st.Skip(curTx)
			if t.SysUpdate {
				// the run in order changed the parameters in memory, which the block leaves out
				e.node.Lock()
				err := e.reloadSysPar(curTx)
				e.node.Unlock()
				if err != nil {
					return err
				}
			}
			if len(*processedTx) == 0 {
				txBadChan <- badTxStruct{index: curTx, hash: t.Hash(), msg: "transaction is larger than an empty block", keyID: t.KeyID()}
				continue
			}
			br.stopped = true
			break
		}

		st.Commit(curTx, r.v)
		st.ApplyOutputs(t.Hash(), b.Header.BlockId, t.TxInputsMap, t.TxOutputsMap)
		txs[curTx] = t

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
		if b.onCommit != nil {
			b.onCommit(t)
		}
	}
	if fb := st.Fallbacks(); len(fb) > 0 {
		b.GetLogger().WithFields(log.Fields{"fallbacks": fb}).Debug("state of the block")
	}
	return nil
}
