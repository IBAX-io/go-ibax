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
	"slices"
	"sync"
	"sync/atomic"
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
func execWorkers() int { return conf.Config.Exec.WorkerCount() }

// maxRestarts bounds the runs in order of a transaction that each put more tables in PostgreSQL
const maxRestarts = 64

// errRestarted is the error of a settlement whose view has to run again
var errRestarted = errors.New("the settlement runs again after the state writes tables to PostgreSQL")

// errRestarts is a transaction that does not run in order however many tables go to PostgreSQL
var errRestarts = errors.New("the transaction does not run in order in the state of the block")

// A block whose runs in parallel often do not hold keeps the readers of its transactions: the
// commit of a transaction wakes those that read what it changed, which run again on the workers
// ahead of their commit, so the commit does not run them again one after the other. A block
// keeps them when, of the transactions of the block before that ran in parallel, at least
// 1/earlyShare did not hold, and at least earlyMinRuns ran.
const (
	earlyShare   = 10
	earlyMinRuns = 64
)

// earlyOn tells the last block that ran its transactions in parallel had them often not hold
var earlyOn atomic.Bool

// txRun is a run of a transaction of the block on a clone, in a view of the state
type txRun struct {
	t *transaction.Transaction
	v *sqldb.TxView
	// err is the error of the run of the transaction
	err error
	// fatal is an error of the block, out of the run of the transaction
	fatal error
	done  chan struct{}
	// gen counts the runs of the transaction, the run of its job the first
	gen uint32
}

// runJob is a transaction a worker runs
type runJob struct {
	i   int
	t   *transaction.Transaction
	run *txRun
	gen uint32
	// taken is set by the first of a worker and the commit to take a run again a commit woke;
	// nil for the first run of a transaction
	taken *atomic.Bool
}

// earlyJob is a run again of a transaction a commit woke
type earlyJob struct {
	run   *txRun
	taken atomic.Bool
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
	// rerun are the runs again of the transactions the commits woke, which the workers take
	// before the transactions of jobs; early is the last of each transaction, which the block
	// reads alone. It is nil unless the block keeps the readers.
	rerun chan runJob
	early []*earlyJob
	// tried counts the transactions that ran in parallel, conflicts those whose first run did not
	// hold
	tried, conflicts int
	// node is held to read, by the runs in parallel, and to write, by the runs in order, what
	// the node keeps out of the database: the contracts of the VM and the parameters
	node sync.RWMutex
	// again is the connection of the runs the commit runs again in parallel, which it pins at
	// its first
	again *sql.Conn
	// stream writes the records of the transactions the block committed while it runs; stop
	// ends it with what it wrote and its error
	stream    *afterStream
	written   afterWritten
	streamErr error
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
// with limits that check nothing: the block checks them in its order. why is why it runs, as
// TxExecutions counts it.
func (e *stateExec) parallel(i int, t *transaction.Transaction, conn *sql.Conn, why string) txRun {
	metrics.TxExecutions.WithLabelValues(why).Inc()
	r := e.play(t, e.st.NewParallelView(i, contractOf(t), conn), e.limits.Deferred())
	e.st.SetRunIDs(i, r.v.IDCounts())
	return r
}

// reloadSysPar reads again the parameters of the platform a failed run changed in memory
func (e *stateExec) reloadSysPar(i int) error {
	e.stream.hold()
	if err := syspar.SysUpdate(e.st.NewOrderedView(i, "").Transaction()); err != nil {
		return fmt.Errorf("updating syspar: %w", err)
	}
	e.st.SysUpdate()
	return nil
}

// ordered runs the transaction i in order, while no worker runs, with the limits of the block
func (e *stateExec) ordered(i int) txRun {
	e.stream.hold()
	e.node.Lock()
	defer e.node.Unlock()
	for range maxRestarts {
		metrics.TxExecutions.WithLabelValues("ordered").Inc()
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
	return e.parallel(i, e.txs[i], e.again, "again")
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
			for {
				var j runJob
				select {
				case j = <-e.rerun:
				default:
					var ok bool
					select {
					case j = <-e.rerun:
					case j, ok = <-next:
						if !ok {
							return
						}
					}
				}
				if j.taken != nil && !j.taken.CompareAndSwap(false, true) {
					// the commit came to the transaction first and runs it itself
					continue
				}
				e.node.RLock()
				r := e.runReader(j, conn)
				e.node.RUnlock()
				r.done, r.gen = j.run.done, j.gen
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

// runReader runs the job, and keeps the run as a reader of the block, which keeps them. A commit
// since the run began may have changed what it read before it was a reader: the run that does
// not hold then runs again at once.
func (e *stateExec) runReader(j runJob, conn *sql.Conn) txRun {
	why := "parallel"
	if j.gen > 0 {
		why = "early"
	}
	r := e.parallel(j.i, j.t, conn, why)
	if e.rerun == nil {
		return r
	}
	for range 2 {
		if r.fatal != nil || r.v == nil || !e.st.Register(j.i, j.gen, r.v) || e.st.Holds(r.v) {
			break
		}
		r = e.parallel(j.i, j.t, conn, "recheck")
	}
	return r
}

// wake runs again on the workers the transactions after cur whose runs the commits woke; one the
// workers have no room for runs again at its commit
func (e *stateExec) wake(cur int) {
	for _, w := range e.st.Woken() {
		if w.I <= cur || w.I >= len(e.early) {
			continue
		}
		ej := &earlyJob{run: &txRun{done: make(chan struct{})}}
		select {
		case e.rerun <- runJob{i: w.I, t: e.txs[w.I], run: ej.run, gen: w.Gen + 1, taken: &ej.taken}:
			e.early[w.I] = ej
		default:
			e.early[w.I] = nil
		}
	}
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
	return &stateExec{b: b, dbTx: dbTx, st: st, limits: limits, ecoParams: b.EcoParams, stream: newAfterStream(dbTx)}, nil
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
			if earlyOn.Load() {
				e.st.WatchReaders()
				e.rerun = make(chan runJob, 64*workers)
			}
			if err := e.startWorkers(workers); err != nil {
				return err
			}
		}
	}
	if e.rerun != nil {
		// the workers may start on a later part: early covers every transaction of the block
		e.early = append(e.early, make([]*earlyJob, len(e.txs)-len(e.early))...)
	}
	queued := e.jobs != nil
	if queued {
		e.jobs <- part
	}
	err := b.commitState(e, r, from, to, queued)
	b.execEnd = time.Now()
	return err
}

// stop stops the workers and the writer of the records of the transactions
func (e *stateExec) stop() {
	if e.stream != nil {
		e.written, e.streamErr = e.stream.stop()
		e.stream = nil
	}
	if e.stopRun != nil {
		e.stopRun()
		e.stopRun = nil
		earlyOn.Store(e.tried >= earlyMinRuns && e.conflicts*earlyShare >= e.tried)
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

// earlyRun is the last run again of the transaction i a commit woke, once it is done. It is nil
// for none, and for one no worker began: the commit runs that one itself, as a worker would only
// begin it once it is free. woken tells a commit woke the transaction.
func (e *stateExec) earlyRun(i int) (er *txRun, woken bool) {
	if e.early == nil || e.early[i] == nil {
		return nil, false
	}
	ej := e.early[i]
	if ej.taken.CompareAndSwap(false, true) {
		return nil, true
	}
	// the run began before the commit came to the transaction: it ends sooner than a run now
	<-ej.run.done
	return ej.run, true
}

// earlyHolds tells a run again a commit woke can commit: it ran in parallel, needs no order and
// its reads hold
func earlyHolds(st *sqldb.BlockState, er *txRun) bool {
	if er.fatal != nil || er.v == nil {
		return false
	}
	if needs, _ := er.v.NeedsOrder(); needs {
		return false
	}
	return st.Holds(er.v)
}

// commitState takes the runs of the transactions from to to in their order, runs again those
// that do not hold, and commits them. A transaction that fails is left out of a block the node
// generates, and fails a block of another node. b.Transactions takes the runs it commits.
func (b *Block) commitState(e *stateExec, br *blockRun, from, to int, queued bool) error {
	st := e.st
	txs, txBadChan, afters, processedTx, fees := b.Transactions, br.bad, br.afters, &br.processed, br.fees
	// the block takes the connection of its transaction for a statement, and holds it until its
	// next transaction, and once it commits them
	defer e.stream.hold()
	for curTx := from; curTx < to; curTx++ {
		e.stream.release()
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
		} else {
			e.tried++
			// a run a commit woke does not hold: the run again it woke holds, unless it needs the
			// order; then a run now holds, as the transactions before it committed
			if er, woken := e.earlyRun(curTx); woken || !st.Holds(r.v) {
				e.conflicts++
				if er != nil && earlyHolds(st, er) {
					r, run = *er, "early"
				} else if r, run = e.runAgain(curTx), "again"; r.fatal == nil {
					if needs, _ := r.v.NeedsOrder(); needs || !st.Holds(r.v) {
						r, run = e.ordered(curTx), "ordered"
					}
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
				if errors.Is(err, transaction.ErrTimeLeft) {
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
			// the savepoint of a run in order is on the connection of the block, held since the run
			e.stream.hold()
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
		if e.rerun != nil {
			e.wake(curTx)
		}
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
		e.stream.send(afters)
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
