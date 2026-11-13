/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/transaction"
)

const (
	// tailMinTxs is the fewest transactions of a trial that tells the tail of a transaction: the
	// fixed time of a smaller block outweighs what its transactions add. A block takes that many
	// before it keeps time for their tail, so the trials go on telling it.
	tailMinTxs = transaction.TailFreeTxs
	// tailWeight is the weight of the last trial in the tail
	tailWeight = 0.3
)

// txTail is what a transaction adds to a trial after the transactions of the block run and the
// writer of their records wrote what it was given: writing its state, the rows after it and the
// block, and holding the trial. A proposer keeps that
// much of max_block_generation_time for each transaction its block takes.
var txTail struct {
	sync.Mutex
	d time.Duration
}

// txWrite is the time of the statements the writer of the records of the transactions a block
// committed takes for a transaction. A proposer keeps that much for each transaction it gave the
// writer and the writer has not written.
var txWrite struct {
	sync.Mutex
	d time.Duration
}

// writeCost is the time of the writer for a transaction the executions of the node measured, 0
// before the first
func writeCost() time.Duration {
	txWrite.Lock()
	defer txWrite.Unlock()
	return txWrite.d
}

// observeWrite takes the time of the writer for a transaction of an execution whose writer wrote
// written: one of less than a part tells nothing
func observeWrite(written afterWritten) {
	if written.txs < afterPartTxs {
		return
	}
	d := written.busy / time.Duration(written.txs)
	txWrite.Lock()
	defer txWrite.Unlock()
	if txWrite.d == 0 {
		txWrite.d = d
		return
	}
	txWrite.d += time.Duration(tailWeight * float64(d-txWrite.d))
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
