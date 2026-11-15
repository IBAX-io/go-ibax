/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"encoding/hex"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/stretchr/testify/require"
)

// TestRollbackRecordsByBlock: a block reads its rollback records by its key, the last first and by
// their transactions, and deletes them at once; the ids go on from the highest of the last block,
// which the primary key finds, and the transaction record finds its rows through its block
func TestRollbackRecordsByBlock(t *testing.T) {
	db := retentionDB(t)
	tx := GetDB(db)
	rows := func(block int64, hashes ...byte) []*RollbackTx {
		rts := make([]*RollbackTx, 0, 2*len(hashes))
		for _, h := range hashes {
			for _, id := range []string{"1", "2"} {
				rts = append(rts, &RollbackTx{BlockID: block, TxHash: []byte{h}, NameTable: "1_keys", TableID: id})
			}
		}
		require.NoError(t, CreateBatchesRollbackTx(tx, rts))
		return rts
	}
	five := rows(5, 1, 2)
	six := rows(6, 3, 4)
	require.Equal(t, five[len(five)-1].ID+1, six[0].ID)
	id, err := NewDbTransaction(tx).MaxID("rollback_tx")
	require.NoError(t, err)
	require.Equal(t, six[len(six)-1].ID, id)

	byTx, err := (&RollbackTx{}).GetRollbackTransactions(db, 6)
	require.NoError(t, err)
	require.Len(t, byTx, 2)
	for _, h := range []byte{3, 4} {
		records := byTx[hex.EncodeToString([]byte{h})]
		require.Len(t, records, 2)
		require.Equal(t, "2", records[0]["table_id"])
		require.Equal(t, "1", records[1]["table_id"])
	}
	require.Greater(t, converter.StrToInt64(byTx["04"][1]["id"]), converter.StrToInt64(byTx["03"][0]["id"]))

	require.NoError(t, DeleteBlockRollbackTransactions(db, 6))
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "rollback_tx" WHERE "block_id" = 6`))
	require.EqualValues(t, 4, countRows(t, db, `SELECT count(*) FROM "rollback_tx" WHERE "block_id" = 5`))
	again := rows(6, 3)
	require.Equal(t, five[len(five)-1].ID+1, again[0].ID)

	require.NoError(t, tx.Create(&LogTransaction{Hash: []byte{2}, Block: 5, Timestamp: 1000}).Error)
	var found []RollbackTx
	require.NoError(t, tx.Table(ArchivedTable("rollback_tx")).
		Where("block_id IN (SELECT block FROM "+ArchivedTable("log_transactions")+" WHERE hash = ?) AND tx_hash = ?",
			[]byte{2}, []byte{2}).Find(&found).Error)
	require.Len(t, found, 2)
	_, err = GetTxRecord(db, "02")
	require.NoError(t, err)
}
