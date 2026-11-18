/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/stretchr/testify/require"
)

// TestRollbackRecordsByBlock: a block reads its rollback records by its key, the last first and by
// the indexes of their transactions, and deletes them at once; the ids go on from the highest of
// the last block, which the primary key finds, and the transaction record finds its rows by its
// block and its index
func TestRollbackRecordsByBlock(t *testing.T) {
	db := retentionDB(t)
	tx := GetDB(db)
	rows := func(block int64, txs ...int32) []*RollbackTx {
		rts := make([]*RollbackTx, 0, 2*len(txs))
		for _, idx := range txs {
			for _, id := range []string{"1", "2"} {
				rts = append(rts, &RollbackTx{BlockID: block, TxIdx: idx, NameTable: "1_keys", TableID: id})
			}
		}
		require.NoError(t, CreateBatchesRollbackTx(tx, rts, NewTableRefs()))
		return rts
	}
	five := rows(5, 0, 1)
	six := rows(6, 0, 1)
	require.Equal(t, five[len(five)-1].ID+1, six[0].ID)
	id, err := NewDbTransaction(tx).MaxID("rollback_tx")
	require.NoError(t, err)
	require.Equal(t, six[len(six)-1].ID, id)

	byTx, err := (&RollbackTx{}).GetRollbackTransactions(db, 6)
	require.NoError(t, err)
	require.Len(t, byTx, 2)
	for _, idx := range []int{0, 1} {
		records := byTx[idx]
		require.Len(t, records, 2)
		require.Equal(t, "2", records[0]["table_id"])
		require.Equal(t, "1", records[1]["table_id"])
		require.Equal(t, "1_keys", records[0]["table_name"])
	}
	require.Greater(t, converter.StrToInt64(byTx[1][1]["id"]), converter.StrToInt64(byTx[0][0]["id"]))

	require.NoError(t, DeleteBlockRollbackTransactions(db, 6))
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "rollback_tx" WHERE "block_id" = 6`))
	require.EqualValues(t, 4, countRows(t, db, `SELECT count(*) FROM "rollback_tx" WHERE "block_id" = 5`))
	again := rows(6, 0)
	require.Equal(t, five[len(five)-1].ID+1, again[0].ID)

	_, err = GetTxRecord(db, 5, 1)
	require.NoError(t, err)
}

// TestRollbackTableRefs: a block gives the tables rollback_tables does not have the next refs, in
// the order of its records and once whatever the statements it writes them in, and its rollback
// deletes them; another execution of the block gives the same refs
func TestRollbackTableRefs(t *testing.T) {
	db := retentionDB(t)
	tx := GetDB(db)
	var last int32
	require.NoError(t, tx.Raw(`SELECT coalesce(max("ref"), 0) FROM "rollback_tables"`).Scan(&last).Error)
	write := func(block int64, refs *TableRefs, tables ...string) []*RollbackTx {
		rts := make([]*RollbackTx, len(tables))
		for i, name := range tables {
			rts[i] = &RollbackTx{BlockID: block, NameTable: name, TableID: "1"}
		}
		require.NoError(t, CreateBatchesRollbackTx(tx, rts, refs))
		return rts
	}
	refs := NewTableRefs()
	first := write(7, refs, "t_b", "t_a", "t_b")
	second := write(7, refs, "t_a", "t_c")
	require.Equal(t, []int32{last + 1, last + 2, last + 1}, []int32{first[0].TableRef, first[1].TableRef, first[2].TableRef})
	require.Equal(t, []int32{last + 2, last + 3}, []int32{second[0].TableRef, second[1].TableRef})
	byTx, err := (&RollbackTx{}).GetRollbackTransactions(db, 7)
	require.NoError(t, err)
	var names []string
	for _, r := range byTx[0] {
		names = append(names, r["table_name"])
	}
	require.Equal(t, []string{"t_c", "t_a", "t_b", "t_a", "t_b"}, names)

	// the next block finds the refs the last gave
	next := write(8, NewTableRefs(), "t_c", "t_d")
	require.Equal(t, []int32{last + 3, last + 4}, []int32{next[0].TableRef, next[1].TableRef})

	require.NoError(t, DeleteBlockRollbackTransactions(db, 8))
	require.NoError(t, DeleteBlockRollbackTransactions(db, 7))
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "rollback_tables" WHERE "ref" > ?`, last))
	again := write(7, NewTableRefs(), "t_d", "t_a")
	require.Equal(t, []int32{last + 1, last + 2}, []int32{again[0].TableRef, again[1].TableRef})
}
