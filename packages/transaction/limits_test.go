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

// The time left to a block runs from the start StartAt sets, never is below 0 nor above the time
// of a block, and the limits of the groups of a block share it
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

	limits.StartAt(time.Now().Add(-500 * time.Millisecond))
	if left := limits.TimeLeft(); left <= 1400 || left > 1500 {
		t.Fatalf("%d ms left 500 ms in a block of 2000 ms", left)
	}
	if err := block.check(nil, letGenBlock); err != nil {
		t.Fatalf("a block with time left stops: %v", err)
	}
	limits.StartAt(time.Now().Add(-3 * time.Second))
	if left := limits.TimeLeft(); left != 0 {
		t.Fatalf("%d ms left past the time of the block", left)
	}
	if err := block.check(nil, letGenBlock); !errors.Is(err, ErrLimitStop) {
		t.Fatalf("a block past its time goes on: %v", err)
	}
	limits.StartAt(time.Now().Add(time.Second))
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
