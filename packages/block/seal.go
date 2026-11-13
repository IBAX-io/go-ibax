/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"errors"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	log "github.com/sirupsen/logrus"
)

// The proposal of a proposer is ready once its block is sealed, whatever the database does then.
// The trial of a block the node generates seals the block in memory after its transactions: the
// fees credited in the state of the block, the hashes of all its rollback records, its outputs,
// the validators set it leaves read through its state, its header and BinData. The trial is held
// then, and goes on writing the block in its transaction: what the writer of the records of the
// transactions was given, the state of the block, the rest of the records, the outputs and the
// block. The commit of the height waits for those writes, a drop of the trial stops them. What
// the proposal carries is computed from the execution alone, as every validator computes it, so
// the time the database takes to write the block moves the commit of the height, not the
// proposal. A trial whose writes fail is not committed: the node executes the block again as one
// whose trial failed, as Commit does.

// errTrialDropped is the error of the writes of a held trial the node dropped
var errTrialDropped = errors.New("the held trial was dropped")

// sealTrial ends the trial of a block the node generates whose transactions ran, in a state, with
// the error err: it seals the block and holds the trial, which writes the block after, or rolls
// it back
func (b *Block) sealTrial(dbTx *sqldb.DbTransaction, mem memState, r *blockRun, err error) error {
	e := r.state
	err = r.close(err)
	if err == nil && len(b.TxFullData) == 0 {
		err = ErrEmptyBlock
	}
	// retention drops what is older than history_blocks as the chain has it before the block
	history := syspar.GetHistoryBlocks()
	var outputs []sqldb.SpentInfo
	if err == nil {
		start := time.Now()
		outputs, err = b.sealState(e)
		b.stage(metrics.StageSeal, start)
	}
	b.trial = false
	b.SysUpdate = false
	start := time.Now()
	if err != nil {
		e.cancel()
		b.Notifications = nil
		dbTx.Rollback()
		if errMem := mem.restore(); errMem != nil {
			b.prof = nil
			return errMem
		}
		b.stage(metrics.StageRollback, start)
		b.prof = nil
		return err
	}
	left := takeMemState()
	if err := mem.restore(); err != nil {
		e.cancel()
		dbTx.Rollback()
		b.prof = nil
		return err
	}
	b.stage(metrics.StageHold, start)
	// the time after the transactions to the seal is of the tail, the wait for the writer is not
	sealed := time.Since(b.execEnd)
	p := b.prof
	b.prof = nil
	if p != nil {
		p.stage(metrics.StageReady, p.start)
	}
	t := &heldTrial{b: b, dbTx: dbTx, mem: left, done: make(chan struct{}), stream: e.stream}
	held.Lock()
	held.t = t
	held.Unlock()
	go t.write(e, p, trialWrites{outputs: outputs, history: history, sealed: sealed})
	return nil
}

// sealState seals the block of the trial from the state of its execution, before the state is
// written: its rollbacks hash from the hashes of all its rollback records and its outputs, the
// validators set it leaves, its header and BinData. It returns the outputs.
func (b *Block) sealState(e *stateExec) ([]sqldb.SpentInfo, error) {
	outputs := sqldb.GetAllOutputs(b.OutputsMap)
	hashes, err := rollbackHashes(newAfterRows(nil, b.AfterTxs.Rts).rts)
	if err != nil {
		return nil, fmt.Errorf("rollbacks hash: %w", err)
	}
	if b.rollbacksHash, err = rollbacksHash(b.Header.BlockId, hashes, outputs); err != nil {
		return nil, fmt.Errorf("rollbacks hash: %w", err)
	}
	return outputs, e.read(b.seal)
}

// trialWrites is what a held trial writes after its block was sealed: the outputs of the block,
// the history_blocks of the retention, and the time the trial took from its transactions to the
// seal
type trialWrites struct {
	outputs []sqldb.SpentInfo
	history int64
	sealed  time.Duration
}

// write writes the block of the trial after its seal, in the transaction of the trial, and ends
// the profile p of the trial; it changes nothing of the block. It stops at its next step once the
// trial is dropped.
func (t *heldTrial) write(e *stateExec, p *execProfile, w trialWrites) {
	defer close(t.done)
	b := t.b
	start := time.Now()
	e.stopStream()
	drained := time.Since(start)
	written := e.written
	observeWrite(written)
	if p != nil {
		p.stage(metrics.StageStream, time.Now().Add(-written.spent))
		p.note("drained", drained)
	}
	err := e.streamErr
	step := func(name string, f func() error) {
		if err != nil {
			return
		}
		if t.dropped.Load() {
			err = errTrialDropped
			return
		}
		start := time.Now()
		err = f()
		if p != nil {
			p.stage(name, start)
		}
	}
	after := time.Now()
	step(metrics.StageAfter, func() error {
		if err := e.flush(); err != nil {
			return err
		}
		return b.writeAfters(t.dbTx, written, w.outputs, w.history)
	})
	step(metrics.StageInsert, func() error { return b.writeBlock(t.dbTx) })
	t.err = err
	if err != nil {
		if !errors.Is(err, errTrialDropped) && !errors.Is(err, errStreamCanceled) {
			b.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Warn("writing the block of a held proposal")
		}
		return
	}
	observeTail(w.sealed+time.Since(after), len(b.TxFullData))
	if p != nil {
		p.end(b.Header.BlockId, len(b.TxFullData), t.dbTx.Statements())
	}
}
