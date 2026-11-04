/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestCopyInto checks the logs and the rollback records a connection of the state copies are
// those a plain connection inserts
func TestCopyInto(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	db, err := openDB(dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	stateDB, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)
	plainDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	const block = -999_000
	write := func(conn *gorm.DB, tag byte) ([]LogTransaction, []RollbackTx) {
		tx := conn.Begin()
		require.NoError(t, tx.Error)
		defer tx.Rollback()
		lts := []*LogTransaction{
			{Hash: []byte{tag, 0, '\\', '\t', 0xff}, Block: block, Timestamp: 17, Address: -5, EcosystemID: 2, Status: 1, ContractName: "@1Tab\tLine\nBack\\slash"},
			{Hash: []byte{tag, 1}, Block: block, ContractName: ""},
		}
		require.NoError(t, CreateLogTransactionBatches(tx, lts))
		rts := []*RollbackTx{
			{BlockID: block, TxHash: []byte{tag, 2}, NameTable: "1_keys", TableID: "-42", Data: `{"amount":"1\t2","s":"a\\nb"}`, DataHash: []byte{9, '\n'}},
			{BlockID: block, TxHash: []byte{tag, 3}, NameTable: "1_keys", TableID: "7", Data: "", DataHash: []byte{}},
		}
		require.NoError(t, CreateBatchesRollbackTx(tx, rts))
		var gotL []LogTransaction
		require.NoError(t, tx.Raw(`SELECT * FROM "log_transactions" WHERE "block" = ? ORDER BY "hash"`, block).Scan(&gotL).Error)
		var gotR []RollbackTx
		require.NoError(t, tx.Raw(`SELECT * FROM "rollback_tx" WHERE "block_id" = ? ORDER BY "id"`, block).Scan(&gotR).Error)
		for i := range gotL {
			gotL[i].Hash = gotL[i].Hash[1:]
		}
		for i := range gotR {
			gotR[i].TxHash = gotR[i].TxHash[1:]
			require.Equal(t, rts[i].ID, gotR[i].ID)
			gotR[i].ID = 0
		}
		return gotL, gotR
	}
	none := func() []stateRow { return nil }
	copied, err := copyInto(stateDB, "log_transactions", []string{"hash"}, none)
	require.NoError(t, err)
	require.True(t, copied)
	copied, err = copyInto(plainDB, "log_transactions", []string{"hash"}, none)
	require.NoError(t, err)
	require.False(t, copied)

	copiedL, copiedR := write(stateDB, 'c')
	insertedL, insertedR := write(plainDB, 'i')
	require.Len(t, copiedL, 2)
	require.Len(t, copiedR, 2)
	require.Equal(t, insertedL, copiedL)
	require.Equal(t, insertedR, copiedR)
}
