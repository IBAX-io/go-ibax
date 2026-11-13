/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/types"
)

// The time of the writer for a transaction is that of the first execution whose writer wrote a part
// or more, then moves toward each later one by tailWeight; the waits of the writer are not in it
func TestObserveWrite(t *testing.T) {
	txWrite.d = 0
	t.Cleanup(func() { txWrite.d = 0 })

	observeWrite(afterWritten{txs: afterPartTxs - 1, busy: time.Second, spent: time.Minute})
	if got := writeCost(); got != 0 {
		t.Fatalf("a time of %v from an execution that wrote less than a part", got)
	}
	observeWrite(afterWritten{txs: 2 * afterPartTxs, busy: time.Duration(2*afterPartTxs) * 100 * time.Microsecond, spent: time.Minute})
	if got := writeCost(); got != 100*time.Microsecond {
		t.Fatalf("a time of %v for a transaction the statements of which took 0.1 ms, want 0.1 ms", got)
	}
	observeWrite(afterWritten{txs: afterPartTxs, busy: time.Duration(afterPartTxs) * 1100 * time.Microsecond})
	if got := writeCost(); got != 400*time.Microsecond {
		t.Fatalf("a time of %v after an execution of 1.1 ms a transaction, want 0.4 ms", got)
	}
}

// The backlog of the writer is what it was given and has not written, at the time of the writer
// for a transaction
func TestStreamBacklog(t *testing.T) {
	txWrite.d = 250 * time.Microsecond
	t.Cleanup(func() { txWrite.d = 0 })
	s := &afterStream{parts: make(chan afterPart, 4)}
	afters := &types.AfterTxs{}
	for range afterPartTxs - 1 {
		afters.Txs = append(afters.Txs, &types.AfterTx{})
	}
	s.send(afters)
	if got := s.backlog(); got != 0 {
		t.Fatalf("a backlog of %v before the block gave the writer a part", got)
	}
	afters.Txs = append(afters.Txs, &types.AfterTx{})
	s.send(afters)
	if got, want := s.backlog(), time.Duration(afterPartTxs)*250*time.Microsecond; got != want {
		t.Fatalf("a backlog of %v after a part of %d transactions, want %v", got, afterPartTxs, want)
	}
	s.pending.Add(-afterPartTxs)
	if got := s.backlog(); got != 0 {
		t.Fatalf("a backlog of %v once the writer wrote the part", got)
	}
}

// The tail of a transaction is the first trial of tailMinTxs transactions or more, then moves
// toward each later one by tailWeight; a smaller trial or one with no end of its transactions
// leaves it
func TestObserveTail(t *testing.T) {
	txTail.d = 0
	t.Cleanup(func() { txTail.d = 0 })
	near := func(got, want time.Duration) bool { return got >= want && got < want+want/10 }

	observeTail(time.Now().Add(-time.Second), tailMinTxs-1)
	observeTail(time.Time{}, 1000)
	if got := tail(); got != 0 {
		t.Fatalf("a tail of %v from trials that tell none", got)
	}
	observeTail(time.Now().Add(-time.Second), 1000)
	if got := tail(); !near(got, time.Millisecond) {
		t.Fatalf("a tail of %v after 1000 transactions in 1 s, want 1 ms", got)
	}
	observeTail(time.Now().Add(-11*time.Second), 1000)
	if got := tail(); !near(got, 4*time.Millisecond) {
		t.Fatalf("a tail of %v after a trial of 11 ms a transaction, want 4 ms", got)
	}
}
