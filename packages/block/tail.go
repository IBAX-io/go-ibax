/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"sync"
	"time"
)

const (
	// tailMinTxs is the fewest transactions of a trial that tells the tail of a transaction: the
	// fixed time of a smaller block outweighs what its transactions add
	tailMinTxs = 100
	// tailWeight is the weight of the last trial in the tail
	tailWeight = 0.3
)

// txTail is what a transaction adds to a trial after the transactions of the block run: writing
// its state, the rows after it and the block, and rolling the trial back. A proposer keeps that
// much of max_block_generation_time for each transaction its block takes.
var txTail struct {
	sync.Mutex
	d time.Duration
}

// tail is the tail of a transaction the trials of the node measured, 0 before the first
func tail() time.Duration {
	txTail.Lock()
	defer txTail.Unlock()
	return txTail.d
}

// observeTail takes the tail of a trial of txs transactions whose transactions ended at execEnd
func observeTail(execEnd time.Time, txs int) {
	if execEnd.IsZero() || txs < tailMinTxs {
		return
	}
	d := time.Since(execEnd) / time.Duration(txs)
	txTail.Lock()
	defer txTail.Unlock()
	if txTail.d == 0 {
		txTail.d = d
		return
	}
	txTail.d += time.Duration(tailWeight * float64(d-txTail.d))
}
