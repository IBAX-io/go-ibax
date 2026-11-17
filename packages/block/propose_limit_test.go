/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"testing"
	"time"
)

// A transaction the time stops as the first of a block, none stopped before it, never fits only
// if the block had the whole of max_block_generation_time: a proposer that started its round late
// and cut its block takes it in a later block, and marks it bad in none
func TestNeverFitsOnlyInAWholeBlock(t *testing.T) {
	const full = 2 * time.Second
	for _, c := range []struct {
		processed int
		stopped   bool
		limit     time.Duration
		never     bool
	}{
		{0, false, 0, true},
		{0, false, full, true},
		{0, false, full + time.Second, true},
		{0, false, full - time.Millisecond, false},
		{0, false, 200 * time.Millisecond, false},
		{0, true, 0, false},
		{1, false, 0, false},
		{1, false, 200 * time.Millisecond, false},
	} {
		if got := neverFits(c.processed, c.stopped, c.limit, full); got != c.never {
			t.Errorf("processed %d, stopped %v, limit %v: never fits %v", c.processed, c.stopped, c.limit, got)
		}
	}
}
