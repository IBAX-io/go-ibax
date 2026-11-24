/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/stretchr/testify/require"
)

// windowKnown is which of the hashes the window knows for a check at checkTime
func windowKnown(t *testing.T, checkTime int64, hashes ...string) []bool {
	bs := make([][]byte, len(hashes))
	for i, h := range hashes {
		bs[i] = []byte(h)
	}
	known, err := TxWindow.Known(bs, checkTime)
	require.NoError(t, err)
	out := make([]bool, len(hashes))
	for i, h := range hashes {
		out[i] = known[h]
	}
	return out
}

// TestTxWindow: the window loads the transactions a transaction can still repeat from
// log_transactions, rolls forward with the blocks, lets go those of a block rolled back, answers
// a check it started after from log_transactions, and loads after a restart what it held before
func TestTxWindow(t *testing.T) {
	db := retentionDB(t)
	ResetTxWindow()
	t.Cleanup(ResetTxWindow)
	T := int64(1) << 32
	logTx := func(hash string, block, time int64) {
		addBlocks(t, db, time, block)
		executeBlock(t, db, time, block)
		require.NoError(t, GetDB(db).Create(&LogTransaction{Hash: []byte(hash), Block: block}).Error)
	}
	logTx("old", 10, T-2000)
	logTx("edge", 20, T-700)
	logTx("new", 30, T)

	require.NoError(t, LoadTxWindow())
	require.Equal(t, 2, TxWindow.Len(), "the transactions of the blocks before T-MaxTxBack-MaxTxForw-margin are not loaded")
	require.Equal(t, []bool{false, true, true, false}, windowKnown(t, T, "old", "edge", "new", "none"))
	// a check the window started after reads log_transactions
	require.Equal(t, []bool{true, true, true, false}, windowKnown(t, T-consts.MaxTxForw-windowMargin-1, "old", "edge", "new", "none"))

	// a block later than the edge lets it go: a transaction signed then is out of MaxTxBack
	logTx("add", 40, T+700)
	require.NoError(t, TxWindow.Add(T+700, []WindowTx{{Hash: []byte("add"), Signed: T + 690}}))
	require.Equal(t, []bool{false, true, true}, windowKnown(t, T+700, "edge", "new", "add"))
	require.Equal(t, 2, TxWindow.Len())

	// a restart loads what the window held
	ResetTxWindow()
	require.NoError(t, LoadTxWindow())
	require.Equal(t, []bool{false, true, true}, windowKnown(t, T+700, "edge", "new", "add"))
	require.Equal(t, 2, TxWindow.Len())

	// the rollback of block 40 lets its transaction go
	TxWindow.Remove([][]byte{[]byte("add")})
	require.Equal(t, []bool{true, false}, windowKnown(t, T+700, "new", "add"))

	// a window used before it is loaded loads itself
	ResetTxWindow()
	require.Equal(t, []bool{true, true}, windowKnown(t, T+700, "new", "add"))
}

// TestTxWindowUnexecuted: the window loads the transactions of the blocks an ordered chain
// committed after the last block the node executed, which have no logs yet, by the parser of
// package block, and fails to load without it
func TestTxWindowUnexecuted(t *testing.T) {
	db := retentionDB(t)
	ResetTxWindow()
	t.Cleanup(ResetTxWindow)
	saved := ParseWindowTxs
	t.Cleanup(func() { ParseWindowTxs = saved })
	T := int64(1) << 32
	addBlocks(t, db, T, 10, 11, 12)
	executeBlock(t, db, T, 10)
	require.NoError(t, GetDB(db).Create(&LogTransaction{Hash: []byte("executed"), Block: 10}).Error)

	ParseWindowTxs = nil
	require.Error(t, LoadTxWindow(), "blocks committed after the executed one, and no parser")

	var parsed []int64
	ParseWindowTxs = func(data []byte) ([]WindowTx, error) {
		parsed = append(parsed, int64(data[0]))
		return []WindowTx{{Hash: []byte{'c', data[0]}, Signed: T}}, nil
	}
	require.NoError(t, GetDB(db).Exec(`UPDATE "block_chain" SET "data" = decode(lpad(to_hex("id"), 2, '0'), 'hex')`).Error)
	ResetTxWindow()
	require.NoError(t, LoadTxWindow())
	require.Equal(t, []int64{11, 12}, parsed, "the blocks after the executed one, in order")
	require.Equal(t, []bool{true, true, true, false}, windowKnown(t, T, "executed", "c\x0b", "c\x0c", "c\x0a"))
}

// TestTxWindowEmptyChain: the window of a chain without blocks holds what its blocks add
func TestTxWindowEmptyChain(t *testing.T) {
	retentionDB(t)
	ResetTxWindow()
	t.Cleanup(ResetTxWindow)
	require.NoError(t, LoadTxWindow())
	require.NoError(t, TxWindow.Add(1000, []WindowTx{{Hash: []byte("a"), Signed: 1000}}))
	require.NoError(t, TxWindow.Add(1000+consts.MaxTxBack+windowMargin+1, []WindowTx{{Hash: []byte("b"), Signed: 1700}}))
	require.Equal(t, []bool{false, true}, windowKnown(t, 1700, "a", "b"))
}
