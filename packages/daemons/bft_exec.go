/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

// The executor of a chain of the BFT consensus (F18 direction A): the consensus orders the blocks
// and commits them, the executor executes the committed blocks after, one at a time in the order
// of the heights, each under DBLock. It starts from the block after the last one the node
// executed, so a node that stopped between the commit of a block and its execution executes it
// as it starts again. A committed block whose execution hash is not that of this node stops it:
// the execution of the node is not that of the chain (block.ErrExecHash).

const (
	// bftExecRetry is how long the executor waits before it executes again a block whose
	// execution failed for another reason than its execution hash, as the database did
	bftExecRetry = 5 * time.Second
	// bftRateMin is the fewest transactions of a block whose execution measures the rate of the
	// executor, and the fewest a proposer takes for its block once the rate is measured
	bftRateMin = 100
	// bftSyncWait is how long the synchronization waits for the executor before it verifies the
	// certificate of a height whose validators set the execution of a block before stores
	bftSyncWait = time.Minute
)

// errSetUnknown is a validators set of a height above what the executed blocks stored: the
// execution of block h stores the set of h+BftSetDelay
var errSetUnknown = errors.New("the node has not executed the block that stores the validators set")

// bftExecutor executes the committed blocks of the node
type bftExecutor struct {
	logger *log.Entry
	wake   chan struct{}

	mu sync.Mutex
	// executed is the height of the last block the node executed, and progress is closed as it
	// moves, for those that wait for it
	executed int64
	progress chan struct{}
	// stopped is the error that stopped the executor, ErrExecHash
	stopped error
	// rate is the transactions a second of the last block of at least bftRateMin, zero before one
	rate float64
	// notify is called after each block the executor executes
	notify func()
	// retryAt is when the executor tries again a block whose execution failed
	retryAt time.Time
	// committedAt is when the node committed the blocks it has not executed, and lastEnd when the
	// last execution ended: the idle time of the executor (metrics.BftExecIdle)
	committedAt map[int64]time.Time
	lastEnd     time.Time
}

var (
	bftExec      = &bftExecutor{wake: make(chan struct{}, 1), progress: make(chan struct{})}
	bftExecStart sync.Once
)

// startBftExecutor starts the executor once, from the executed height of the node
func startBftExecutor(ctx context.Context, logger *log.Entry) {
	bftExecStart.Do(func() {
		e := bftExec
		e.logger = logger.WithFields(log.Fields{"consensus": "bft", "executor": true})
		executed, err := sqldb.ExecutedHeight()
		if err != nil {
			e.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Fatal("reading the executed height")
		}
		e.moved(executed)
		go e.run(ctx)
	})
}

// signal wakes the executor: the block of the height was committed
func (e *bftExecutor) signal(height int64) {
	e.mu.Lock()
	if e.committedAt == nil {
		e.committedAt = make(map[int64]time.Time)
	}
	e.committedAt[height] = time.Now()
	e.mu.Unlock()
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// setNotify sets what the executor calls after each block it executes
func (e *bftExecutor) setNotify(f func()) {
	e.mu.Lock()
	e.notify = f
	e.mu.Unlock()
}

func (e *bftExecutor) run(ctx context.Context) {
	tick := time.NewTicker(bftTick)
	defer tick.Stop()
	for {
		for ctx.Err() == nil && e.step() {
		}
		select {
		case <-ctx.Done():
			return
		case <-e.wake:
		case <-tick.C:
		}
	}
}

// step executes the block after the executed one if the node committed it, and reports whether
// it did
func (e *bftExecutor) step() bool {
	e.mu.Lock()
	idle := e.stopped != nil || time.Now().Before(e.retryAt)
	e.mu.Unlock()
	if idle {
		return false
	}
	DBLock()
	executed, committed, err := heights()
	if err != nil {
		DBUnlock()
		e.failed(executed+1, err)
		return false
	}
	// the commit of block 1 executes it, and a node that joins imports the executed state
	e.moved(executed)
	metrics.BftExecLag.Set(float64(committed - executed))
	if executed >= committed {
		DBUnlock()
		return false
	}
	height := executed + 1
	start := time.Now()
	e.idle(height, start)
	b, err := block.Committed(height)
	if err == nil {
		err = b.Execute()
	}
	DBUnlock()
	if err != nil {
		e.failed(height, err)
		return false
	}
	took := time.Since(start)
	metrics.BftExecLag.Set(float64(committed - height))
	e.mu.Lock()
	e.lastEnd = time.Now()
	if n := len(b.Transactions); n >= bftRateMin && took > 0 {
		e.rate = float64(n) / took.Seconds()
	}
	notify := e.notify
	e.mu.Unlock()
	e.moved(height)
	e.logger.WithFields(log.Fields{"height": height, "txs": len(b.Transactions), "took": took, "lag": committed - height}).
		Debug("executed")
	if notify != nil {
		notify()
	}
	return true
}

// idle observes the time the executor was idle before it started the execution of the height at
// start: from the commit of the block, or the end of the execution before it if that came later.
// Nothing for the first execution of the node, or for a block it committed before it started.
func (e *bftExecutor) idle(height int64, start time.Time) {
	e.mu.Lock()
	at, ok := e.committedAt[height]
	for h := range e.committedAt {
		if h <= height {
			delete(e.committedAt, h)
		}
	}
	lastEnd := e.lastEnd
	e.mu.Unlock()
	if !ok || lastEnd.IsZero() {
		return
	}
	if lastEnd.After(at) {
		at = lastEnd
	}
	metrics.BftExecIdle.Observe(max(start.Sub(at), 0).Seconds())
}

// failed takes an execution of the height that failed: an execution hash not that of the node
// stops the executor, another error is tried again after bftExecRetry
func (e *bftExecutor) failed(height int64, err error) {
	fields := log.Fields{"type": consts.BlockError, "height": height, "error": err}
	e.mu.Lock()
	defer e.mu.Unlock()
	if errors.Is(err, block.ErrExecHash) {
		e.stopped = err
		e.logger.WithFields(fields).Error("the execution of this node is not that of the chain: the executor stops")
		close(e.progress)
		e.progress = make(chan struct{})
		return
	}
	e.retryAt = time.Now().Add(bftExecRetry)
	e.logger.WithFields(fields).Error("executing a committed block")
}

// moved takes the executed height
func (e *bftExecutor) moved(height int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if height > e.executed {
		e.executed = height
	}
	close(e.progress)
	e.progress = make(chan struct{})
}

// wait waits until the node executed the block of the height, up to the deadline
func (e *bftExecutor) wait(ctx context.Context, height int64, deadline time.Time) error {
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	for {
		e.mu.Lock()
		executed, stopped, progress := e.executed, e.stopped, e.progress
		e.mu.Unlock()
		switch {
		case executed >= height:
			return nil
		case stopped != nil:
			return stopped
		}
		select {
		case <-progress:
		case <-timer.C:
			return fmt.Errorf("%w %d: the node executed up to %d", block.ErrNotExecuted, height, executed)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// capacity is the transactions a proposer takes at most for its block: those the executor runs
// in syspar.BftExecTarget at its last rate, at least bftRateMin, at most max_tx_block; max_tx_block
// before the executor measured its rate
func (e *bftExecutor) capacity() int {
	e.mu.Lock()
	rate := e.rate
	e.mu.Unlock()
	limit := syspar.GetMaxTxCount()
	if rate == 0 {
		return limit
	}
	return min(max(int(rate*syspar.BftExecTarget.Seconds()), bftRateMin), limit)
}

// heights is the executed and the committed heights of the node
func heights() (executed, committed int64, err error) {
	if executed, err = sqldb.ExecutedHeight(); err != nil {
		return 0, 0, err
	}
	committed, err = sqldb.CommittedHeight()
	return executed, committed, err
}

// knownValidators is the validators set of the height, which the node knows once it executed the
// block BftSetDelay before it (errSetUnknown before)
func knownValidators(height uint64) (bft.Validators, error) {
	executed, err := sqldb.ExecutedHeight()
	if err != nil {
		return bft.Validators{}, err
	}
	if int64(height) > executed+consts.BftSetDelay {
		return bft.Validators{}, fmt.Errorf("%w: height %d, executed %d", errSetUnknown, height, executed)
	}
	return validatorsAt(height)
}

// commitMu orders the commits of the consensus and of the synchronization
var commitMu sync.Mutex

// commitCertified commits the block of a certificate unless the node committed its height, hands
// it to the executor, and reports whether it committed it
func commitCertified(bin []byte, cert *bft.Certificate) (bool, error) {
	commitMu.Lock()
	defer commitMu.Unlock()
	committed, err := sqldb.CommittedHeight()
	if err != nil {
		return false, err
	}
	if committed >= int64(cert.Height) {
		return false, nil
	}
	if _, err := block.Commit(bin, cert); err != nil {
		return false, err
	}
	bftExec.signal(int64(cert.Height))
	return true, nil
}
