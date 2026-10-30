/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"testing"
	"time"
)

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
