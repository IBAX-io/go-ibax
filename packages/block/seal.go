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
// the time the database takes to write the block moves the commit of the height and the prevote
// of the proposer for its block (Written), not the proposal. A drop of the trial cancels its
// transaction: PostgreSQL cancels the statement running, and the transaction ends as its
// connection closes, so the next trial waits for no write of the dropped one beyond that. A trial whose writes fail is not committed:
// the node executes the block again as one whose trial failed, as Commit does. What the writes
// read of the state outside the database is what the transactions of the block left: the seal
// reads it (seal, the retention's history_blocks) before the state of the node is restored.

// ErrTrialDropped is the error of the writes of a held trial the node dropped before they ended
var ErrTrialDropped = errors.New("the held trial was dropped")

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
	b.writes = t
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
// the profile p of the trial; it changes nothing of the block and reads no platform parameter.
// Once the trial is dropped its statements fail, as its transaction is canceled, and its error is
// ErrTrialDropped.
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
			err = ErrTrialDropped
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
	if err != nil && t.dropped.Load() {
		err = ErrTrialDropped
	}
	t.err = err
	if err != nil {
		if !errors.Is(err, ErrTrialDropped) {
			b.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Warn("writing the block of a held proposal")
		}
		return
	}
	observeTail(w.sealed+time.Since(after), len(b.TxFullData))
	if p != nil {
		p.end(b.Header.BlockId, len(b.TxFullData), t.dbTx.Statements())
	}
}

// Written waits for the writes of a block the node proposed, which its trial goes on with after
// the seal, and returns their error: ErrTrialDropped if the node dropped the trial before they
// ended. A block of no such writes returns nil at once. The node prevotes its block only once its
// writes succeed: a block whose writes fail is not valid at the node, as one whose trial fails.
func (b *Block) Written() error {
	if b.writes == nil {
		return nil
	}
	return b.writes.wait()
}
