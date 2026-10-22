/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

// Status is the status of a transaction: the one of its block, else the pool's, pending with no
// error or bad with the error it was found bad with
func Status(hash []byte) (*sqldb.TransactionStatus, bool, error) {
	ts := &sqldb.TransactionStatus{}
	found, err := ts.Get(hash)
	if err != nil || (found && ts.BlockID > 0) {
		return ts, found, err
	}
	if _, badMsg, ok := TxPool.Status(hash); ok {
		return &sqldb.TransactionStatus{Hash: hash, Error: badMsg}, true, nil
	}
	return ts, found, nil
}
