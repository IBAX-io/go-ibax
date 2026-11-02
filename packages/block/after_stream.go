/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// The records of a transaction, its log, its status and its rollback records, do not change once
// the block commits it, and no contract reads them. While the block runs, a writer writes those of
// the transactions it committed into the transaction of the block, in parts, while the block does
// not use the connection of its transaction. The block holds the connection but while it commits
// runs in parallel, and takes it before a statement until its next transaction, so a part never
// goes between the statements of a run in order or after a savepoint the block may roll back to.
// The writer takes the hashes of the rollback records before they have their ids, and writes the
// parts in the order of the block, which gives the rollback records the ids in that order:
// AfterPlayTxs writes the rest, and the rollbacks hash of the block is the one it had without.

const (
	// afterPartTxs and afterPartRts are the transactions, or their rollback records, a part has
	// at least
	afterPartTxs = 4096
	afterPartRts = 8192
	// afterChunk is the most rows of a table a statement of the writer writes: the block waits for
	// at most one statement to take back its connection
	afterChunk = 2048
)

// afterPart are transactions the block committed and their rollback records
type afterPart struct {
	txs []*types.AfterTx
	rts []*types.RollbackTx
}

// afterWritten are the records the writer wrote: of the first txs transactions of the block and
// of their first rts rollback records, with their logs and the hashes of their rollback records
type afterWritten struct {
	txs, rts int
	lts      []*sqldb.LogTransaction
	hashes   []string
	spent    time.Duration
}

// afterStream writes the records of the transactions a block committed while it runs
type afterStream struct {
	dbTx *sqldb.DbTransaction
	// conn is held by the block while held, and by the writer while it writes a statement
	conn  sync.Mutex
	held  bool
	parts chan afterPart
	done  chan struct{}
	// sentTxs and sentRts are the transactions and the rollback records the block gave the writer
	sentTxs, sentRts int
	// written and err are those of the writer, which the block reads once it is done
	written afterWritten
	err     error
}

// newAfterStream starts the writer of the records of the transactions of the block in its
// transaction dbTx; the block holds the connection
func newAfterStream(dbTx *sqldb.DbTransaction) *afterStream {
	s := &afterStream{dbTx: dbTx, parts: make(chan afterPart, 4), done: make(chan struct{}), held: true}
	s.conn.Lock()
	go s.write()
	return s
}

// send gives the writer the transactions the block committed since the last part, once they make
// a part, unless the writer has parts enough to write
func (s *afterStream) send(afters *types.AfterTxs) {
	txs, rts := len(afters.Txs)-s.sentTxs, len(afters.Rts)-s.sentRts
	if txs < afterPartTxs && rts < afterPartRts {
		return
	}
	part := afterPart{
		txs: slices.Clip(afters.Txs[s.sentTxs:]),
		rts: slices.Clip(afters.Rts[s.sentRts:]),
	}
	select {
	case s.parts <- part:
		s.sentTxs, s.sentRts = len(afters.Txs), len(afters.Rts)
	default:
	}
}

// hold takes the connection for the block
func (s *afterStream) hold() {
	if !s.held {
		s.conn.Lock()
		s.held = true
	}
}

// release lets the writer use the connection until the block holds it again
func (s *afterStream) release() {
	if s.held {
		s.conn.Unlock()
		s.held = false
	}
}

// stop waits for the writer to write the parts it has, and ends it; the block then has the
// connection alone
func (s *afterStream) stop() (afterWritten, error) {
	close(s.parts)
	s.release()
	<-s.done
	return s.written, s.err
}

// write writes the parts, each table in statements of at most afterChunk rows
func (s *afterStream) write() {
	defer close(s.done)
	for part := range s.parts {
		if s.err != nil {
			continue
		}
		start := time.Now()
		rows := newAfterRows(part.txs, part.rts)
		hashes := make([]string, len(rows.rts))
		for i, rt := range rows.rts {
			if hashes[i], s.err = rollbackHash(rt); s.err != nil {
				break
			}
		}
		if s.err == nil {
			s.err = s.writeRows(rows)
		}
		s.written.txs += len(part.txs)
		s.written.rts += len(part.rts)
		s.written.lts = append(s.written.lts, rows.lts...)
		s.written.hashes = append(s.written.hashes, hashes...)
		s.written.spent += time.Since(start)
	}
}

// writeRows writes rows in chunks, holding the connection for each statement
func (s *afterStream) writeRows(rows *afterRows) error {
	db := sqldb.GetDB(s.dbTx)
	chunks := func(n int, f func(tx *gorm.DB, from, to int) error) error {
		for from := 0; from < n; from += afterChunk {
			s.conn.Lock()
			err := f(db, from, min(from+afterChunk, n))
			s.conn.Unlock()
			if err != nil {
				return err
			}
		}
		return nil
	}
	if err := chunks(len(rows.lts), func(tx *gorm.DB, from, to int) error {
		return sqldb.CreateLogTransactionBatches(tx, rows.lts[from:to])
	}); err != nil {
		return errors.Wrap(err, "batches insert log_transactions")
	}
	if err := chunks(len(rows.rts), func(tx *gorm.DB, from, to int) error {
		return sqldb.CreateBatchesRollbackTx(tx, rows.rts[from:to])
	}); err != nil {
		return errors.Wrap(err, "batches insert rollback tx")
	}
	if err := chunks(len(rows.statuses), func(tx *gorm.DB, from, to int) error {
		return sqldb.UpsertBlockStatusBatches(tx, rows.statuses[from:to])
	}); err != nil {
		return errors.Wrap(err, "batches upsert transaction status")
	}
	return nil
}

// rollbackHash is the hash of a rollback record of the block, which rollbacksHash takes
func rollbackHash(rt *sqldb.RollbackTx) (string, error) {
	data, err := json.Marshal(rt)
	if err != nil {
		return "", err
	}
	return crypto.HashHex(data), nil
}
