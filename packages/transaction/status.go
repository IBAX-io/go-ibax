/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

// TxStatus is the status of a transaction; Retry tells the pool of the node refused it or took it
// out for its room, and takes it sent again once a block takes some of its transactions
type TxStatus struct {
	sqldb.TransactionStatus
	Retry bool
}

// Status is the status of a transaction: the one of its block, else the pool's, pending with no
// error or bad with the error it was found bad with
func Status(hash []byte) (*TxStatus, bool, error) {
	ts := &TxStatus{}
	found, err := ts.Get(hash)
	if err != nil || (found && ts.BlockID > 0) {
		return ts, found, err
	}
	if _, bad, ok := TxPool.Status(hash); ok {
		return &TxStatus{TransactionStatus: sqldb.TransactionStatus{Hash: hash, Error: bad.Msg}, Retry: bad.Retry}, true, nil
	}
	return ts, found, nil
}
