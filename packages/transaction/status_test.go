/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/migration"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/stretchr/testify/require"
)

// TestStatus: a transaction is pending while the pool holds it; once a block takes it, its status
// is the block's, from log_transactions, whatever the pool refuses after; past MaxTxBack, when the
// window of the duplicates has let it go, its status stays until its range leaves the
// history_blocks window. A transaction the pool refused and no block took has the refusal.
func TestStatus(t *testing.T) {
	db := windowDB(t)
	saved := TxPool
	TxPool = NewPool()
	t.Cleanup(func() { TxPool = saved })
	conf.Config.Storage.Archive = false
	const pb = migration.PartitionBlocks
	const T = int64(1) << 32
	status := func(hash string) (TxStatus, bool) {
		ts, found, err := Status([]byte(hash))
		require.NoError(t, err)
		return *ts, found
	}

	TxPool.Restore([]*Transaction{keyTx("ok", 1, T*1000, 1), keyTx("penalty", 2, T*1000, 1)})
	ts, found := status("ok")
	require.True(t, found)
	require.Equal(t, TxStatus{Hash: []byte("ok")}, ts, "pending")

	logBlock(t, db, pb-1, T)
	require.NoError(t, db.Create(&sqldb.LogTransaction{Hash: []byte("ok"), Block: pb - 1, Result: "42"}).Error)
	require.NoError(t, db.Create(&sqldb.LogTransaction{Hash: []byte("penalty"), Block: pb - 1, Status: 1, Penalty: 1,
		Result: `{"type":"panic","error":"no funds"}`}).Error)
	require.NoError(t, sqldb.TxWindow.Add(T, []sqldb.WindowTx{{Hash: []byte("ok"), Signed: T}, {Hash: []byte("penalty"), Signed: T}}))
	TxPool.Remove([][]byte{[]byte("ok"), []byte("penalty")})
	committed := map[string]TxStatus{
		"ok":      {Hash: []byte("ok"), BlockID: pb - 1, Error: "42"},
		"penalty": {Hash: []byte("penalty"), BlockID: pb - 1, Error: `{"type":"panic","error":"no funds"}`, Penalty: 1},
	}
	check := func(when string) {
		for hash, want := range committed {
			ts, found := status(hash)
			require.True(t, found, "%s %s", hash, when)
			require.Equal(t, want, ts, "%s %s", hash, when)
		}
	}
	check("in its block")

	// sent again, it is refused as a duplicate, and keeps the status of its block
	TxPool.Bad([]byte("ok"), "duplicate")
	check("refused again")

	// a refused transaction no block took has the refusal
	TxPool.Bad([]byte("refused"), `{"type":"error","error":"bad"}`)
	ts, found = status("refused")
	require.True(t, found)
	require.Equal(t, TxStatus{Hash: []byte("refused"), Error: `{"type":"error","error":"bad"}`}, ts)
	_, found = status("none")
	require.False(t, found)

	// past MaxTxBack the window lets the transactions go, and the retention at the boundary 2pb
	// keeps their range in the history_blocks window
	// (signed at most MaxTxForw after their block, and kept the margin of the window longer)
	late := T + consts.MaxTxForw + consts.MaxTxBack + 61
	require.NoError(t, sqldb.TxWindow.Add(late, nil))
	known, err := sqldb.TxWindow.Known([][]byte{[]byte("ok")}, late)
	require.NoError(t, err)
	require.False(t, known["ok"], "out of the window")
	dbTx := sqldb.NewDbTransaction(db)
	require.NoError(t, sqldb.Retain(dbTx, pb, T, 2*pb))
	logBlock(t, db, 2*pb-1, late)
	require.NoError(t, sqldb.Retain(dbTx, 2*pb, late, 2*pb))
	check("past MaxTxBack")

	// out of the history_blocks window the range leaves
	require.NoError(t, sqldb.RestoreRetired(dbTx, 2*pb))
	require.NoError(t, sqldb.Retain(dbTx, 2*pb, late, pb))
	_, found = status("penalty")
	require.False(t, found, "out of history_blocks")
}
