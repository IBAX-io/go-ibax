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
// transactions the block took and of the one; it never is below 0 nor above the time of a block
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

	limits.StartAt(time.Now(), 10*time.Millisecond)
	if left := limits.TimeLeft(); !near(left, 1990) {
		t.Fatalf("%d ms left to the first transaction, want 1990", left)
	}
	if err := block.check(nil, letGenBlock); err != nil {
		t.Fatalf("a block with time left stops: %v", err)
	}
	limits.StartAt(time.Now().Add(-3*time.Second), 10*time.Millisecond)
	if left := limits.TimeLeft(); left != 0 {
		t.Fatalf("%d ms left past the time of the block", left)
	}
	if err := block.check(nil, letGenBlock); !errors.Is(err, ErrLimitStop) {
		t.Fatalf("a block past its time goes on: %v", err)
	}

	limits.StartAt(time.Now().Add(-500*time.Millisecond), 10*time.Millisecond)
	if left := limits.TimeLeft(); !near(left, 1480) {
		t.Fatalf("%d ms left 500 ms in a block of 2000 ms that took 1 transaction, want 1480", left)
	}
	if err := block.check(nil, letGenBlock); err != nil {
		t.Fatalf("a block with time left stops: %v", err)
	}
	// the tail of 2 transactions of 600 ms is past half the block
	limits.StartAt(time.Now().Add(-500*time.Millisecond), 600*time.Millisecond)
	if left := limits.TimeLeft(); !near(left, 500) {
		t.Fatalf("%d ms left with the tail at half the block, want 500", left)
	}
	limits.StartAt(time.Now().Add(-1600*time.Millisecond), 600*time.Millisecond)
	if err := block.check(nil, letGenBlock); !errors.Is(err, ErrLimitStop) {
		t.Fatalf("a transaction with no time left for its tail goes on: %v", err)
	}
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
