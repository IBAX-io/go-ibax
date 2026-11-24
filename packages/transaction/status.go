/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

// TxState is the state of a transaction /txstatus answers
type TxState string

const (
	// TxPending is a transaction in the pool, or in a block the node committed and has not
	// executed yet: an ordered block has its statuses once it is executed, up to BftExecDepth
	// intervals after its commit
	TxPending TxState = "pending"
	// TxExecuted is a transaction its block ran: Error is its result
	TxExecuted TxState = "executed"
	// TxPenalty is a transaction its block ran and that failed: Error is its error, the fee is paid
	TxPenalty TxState = "penalty"
	// TxDiscarded is a transaction the execution of its block discarded: it failed a check before
	// its contract ran, has no effect and pays no fee; Error is the reason
	TxDiscarded TxState = "discarded"
	// TxRefused is a transaction the pool refused or let go: Error is the reason, Retry tells it is
	// for the room of the pool
	TxRefused TxState = "refused"
)

// TxStatus is the status of a transaction: its state, the block that took it with its penalty and
// its result or reason (Error), or the refusal of the pool, whose Retry tells the pool refused it or
// took it out for its room, and takes it sent again once a block takes some of its transactions
type TxStatus struct {
	Hash    []byte
	State   TxState
	BlockID int64
	Error   string
	Penalty int64
	Retry   bool
}

// Status is the status of a transaction: pending in the pool, else the one of its executed block,
// which log_transactions keeps for good, else pending in a committed block the node has not
// executed, else the refusal of the pool. A refusal comes after the block: a transaction sent again
// after its block is refused as a duplicate, and keeps the status of its block.
func Status(hash []byte) (*TxStatus, bool, error) {
	pending, bad, inPool := TxPool.Status(hash)
	if pending {
		return &TxStatus{Hash: hash, State: TxPending}, true, nil
	}
	lt := &sqldb.LogTransaction{}
	found, err := lt.GetByHash(hash)
	if err != nil {
		return &TxStatus{}, false, err
	}
	if found {
		state := TxExecuted
		switch {
		case lt.Status == int64(pbgo.TxInvokeStatusCode_DISCARDED):
			state = TxDiscarded
		case lt.Penalty == 1:
			state = TxPenalty
		}
		return &TxStatus{Hash: hash, State: state, BlockID: lt.Block, Error: lt.Result, Penalty: lt.Penalty}, true, nil
	}
	committed, err := sqldb.TxWindow.Has(hash)
	if err != nil {
		return &TxStatus{}, false, err
	}
	if committed {
		return &TxStatus{Hash: hash, State: TxPending}, true, nil
	}
	if inPool {
		return &TxStatus{Hash: hash, State: TxRefused, Error: bad.Msg, Retry: bad.Retry}, true, nil
	}
	return &TxStatus{}, false, nil
}
