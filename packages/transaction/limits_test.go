/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"errors"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
)

// The time left to a transaction of a block runs from the start StartAt sets, less the tail of the
// transactions the block took and of the one, but for the first TailFreeTxs transactions, which
// have the whole time of the block; it never is below 0 nor above the time of a block, and the time stops a
// transaction by ErrTimeLeft
func TestTimeLeft(t *testing.T) {
	limits := NewLimits(letGenBlock)
	var block *timeBlockLimit
	for _, l := range limits.Limiters {
		if bl, ok := l.(*timeBlockLimit); ok {
			block = bl
		}
	}
	if block == nil {
		t.Fatal("no time limit of the block")
	}
	block.Limit = 2 * time.Second
	near := func(left, want int64) bool { return left <= want && left > want-100 }

	// the first transaction keeps no tail: one the tail of which is past the time left goes on
	limits.StartAt(time.Now().Add(-1600*time.Millisecond), 600*time.Millisecond)
	if left := limits.TimeLeft(); !near(left, 400) {
		t.Fatalf("%d ms left to the first transaction 1600 ms in a block of 2000 ms, want 400", left)
	}
	limits.StartAt(time.Now().Add(-3*time.Second), 10*time.Millisecond)
	if left := limits.TimeLeft(); left != 0 {
		t.Fatalf("%d ms left past the time of the block", left)
	}
	if err := block.check(nil, letGenBlock); !errors.Is(err, ErrTimeLeft) {
		t.Fatalf("a first transaction past the time of its block goes on: %v", err)
	}
	limits.StartAt(time.Now(), 10*time.Millisecond)
	if left := limits.TimeLeft(); !near(left, 2000) {
		t.Fatalf("%d ms left to the first transaction, want 2000", left)
	}
	if err := block.check(nil, letGenBlock); err != nil {
		t.Fatalf("a block with time left stops: %v", err)
	}
	limits.StartAt(time.Now().Add(-3*time.Second), 10*time.Millisecond)
	if left := limits.TimeLeft(); left != 0 {
		t.Fatalf("%d ms left past the time of the block", left)
	}
	if err := block.check(nil, letGenBlock); !errors.Is(err, ErrTimeLeft) {
		t.Fatalf("a block past its time goes on: %v", err)
	}

	// the first TailFreeTxs transactions keep no tail
	limits.StartAt(time.Now().Add(-500*time.Millisecond), 10*time.Millisecond)
	if left := limits.TimeLeft(); !near(left, 1500) {
		t.Fatalf("%d ms left 500 ms in a block that took 1 transaction, want 1500", left)
	}
	block.Count = TailFreeTxs - 1
	if left := limits.TimeLeft(); !near(left, 1500) {
		t.Fatalf("%d ms left to transaction %d, want 1500", left, TailFreeTxs)
	}
	if err := block.check(nil, letGenBlock); err != nil {
		t.Fatalf("a block with time left stops: %v", err)
	}
	// then the block keeps the tail of the transactions it took and of the one
	if left := limits.TimeLeft(); !near(left, 1500-10*(TailFreeTxs+1)) {
		t.Fatalf("%d ms left after %d transactions with a tail of 10 ms, want %d", left, TailFreeTxs, 1500-10*(TailFreeTxs+1))
	}
	// with no bound at half the block: 10,000 transactions with a tail of 0.3 ms keep 3 s, past the
	// 2 s of the block, as the tail of a large block takes longer than its transactions
	block.Count = 10000
	limits.StartAt(time.Now().Add(-500*time.Millisecond), 300*time.Microsecond)
	if left := limits.TimeLeft(); left != 0 {
		t.Fatalf("%d ms left to a block whose tail is past its time", left)
	}
	if err := block.check(nil, letGenBlock); !errors.Is(err, ErrTimeLeft) {
		t.Fatalf("a transaction with no time left for the tail of its block goes on: %v", err)
	}
	block.Count = 2000
	if left := limits.TimeLeft(); !near(left, 2000-500-600) {
		t.Fatalf("%d ms left after 2,000 transactions with a tail of 0.3 ms, want 900", left)
	}
	// the backlog of the writer is kept besides the tail, from the first transaction on
	var backlog time.Duration
	limits.KeepBacklog(func() time.Duration { return backlog })
	block.Count = 0
	limits.StartAt(time.Now().Add(-500*time.Millisecond), 10*time.Millisecond)
	backlog = 700 * time.Millisecond
	if left := limits.TimeLeft(); !near(left, 2000-500-700) {
		t.Fatalf("%d ms left 500 ms in a block whose writer has 700 ms left, want 800", left)
	}
	block.Count = 2000
	limits.StartAt(time.Now().Add(-500*time.Millisecond), 300*time.Microsecond)
	if left := limits.TimeLeft(); !near(left, 2000-500-600-700) {
		t.Fatalf("%d ms left after 2,000 transactions with a tail of 0.3 ms and a backlog of 700 ms, want 200", left)
	}
	backlog = 1500 * time.Millisecond
	if err := block.check(nil, letGenBlock); !errors.Is(err, ErrTimeLeft) {
		t.Fatalf("a transaction with no time left for the backlog of the writer goes on: %v", err)
	}
	limits.KeepBacklog(nil)
	block.Count = 2000

	limits.StartAt(time.Now().Add(time.Second), 0)
	if left := limits.TimeLeft(); left != 2000 {
		t.Fatalf("%d ms left in a block that starts later, want 2000", left)
	}

	if left := NewLimits(letParsing).TimeLeft(); left != syspar.GetMaxBlockGenerationTime() {
		t.Fatalf("%d ms left with no time limit, want max_block_generation_time", left)
	}
	if !errors.Is(ErrTimeLeft, ErrLimitStop) {
		t.Fatal("ErrTimeLeft is not ErrLimitStop")
	}
}

// LimitTo shortens the time of the block from its start, and never lengthens it
func TestLimitTo(t *testing.T) {
	limits := NewLimits(letGenBlock)
	var block *timeBlockLimit
	for _, l := range limits.Limiters {
		if bl, ok := l.(*timeBlockLimit); ok {
			block = bl
		}
	}
	if block == nil {
		t.Fatal("no time limit of the block")
	}
	block.Limit = 2 * time.Second
	near := func(left, want int64) bool { return left <= want && left > want-100 }

	limits.StartAt(time.Now(), 10*time.Millisecond)
	limits.LimitTo(5 * time.Second)
	if left := limits.TimeLeft(); !near(left, 2000) {
		t.Fatalf("%d ms left after a limit above the time of the block, want 2000", left)
	}
	limits.LimitTo(940 * time.Millisecond)
	if left := limits.TimeLeft(); !near(left, 940) {
		t.Fatalf("%d ms left after a limit of 940 ms, want 940", left)
	}
	limits.LimitTo(2 * time.Second)
	if left := limits.TimeLeft(); !near(left, 940) {
		t.Fatalf("%d ms left after a longer limit, want 940: a limit never lengthens the block", left)
	}
	limits.StartAt(time.Now().Add(-time.Second), 10*time.Millisecond)
	if left := limits.TimeLeft(); left != 0 {
		t.Fatalf("%d ms left 1000 ms in a block limited to 940 ms", left)
	}
	if err := block.check(nil, letGenBlock); !errors.Is(err, ErrTimeLeft) {
		t.Fatalf("a block past its limit goes on: %v", err)
	}
}
