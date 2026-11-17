/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

// TxStatus is the status of a transaction: the block that took it, with the penalty and the result
// (Error) of it, or the refusal of the pool, whose Retry tells the pool refused it or took it out
// for its room, and takes it sent again once a block takes some of its transactions
type TxStatus struct {
	Hash    []byte
	BlockID int64
	Error   string
	Penalty int64
	Retry   bool
}

// Status is the status of a transaction: pending in the pool, else the one of its block, which
// log_transactions keeps for the history_blocks window (and an archive node for good), else the
// refusal of the pool. A refusal comes after the block: a transaction sent again after its block
// is refused as a duplicate, and keeps the status of its block.
func Status(hash []byte) (*TxStatus, bool, error) {
	pending, bad, inPool := TxPool.Status(hash)
	if pending {
		return &TxStatus{Hash: hash}, true, nil
	}
	lt := &sqldb.LogTransaction{}
	found, err := lt.GetByHash(hash)
	if err != nil {
		return &TxStatus{}, false, err
	}
	if found {
		return &TxStatus{Hash: hash, BlockID: lt.Block, Error: lt.Result, Penalty: lt.Penalty}, true, nil
	}
	if inPool {
		return &TxStatus{Hash: hash, Error: bad.Msg, Retry: bad.Retry}, true, nil
	}
	return &TxStatus{}, false, nil
}
