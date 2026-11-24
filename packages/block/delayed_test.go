/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The execution of an ordered block lists, in its transaction, the delayed contracts due at the
// height, of the highest rate first and then of their rows, and runs each as a system transaction
// signed by the validator at the position of the header in the set of the height; it needs only a
// database, IBAX_TEST_DSN, where it makes a schema of its own that it rolls back
func TestDelayedTxs(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	tx := conn.Begin()
	require.NoError(t, tx.Error)
	saved := sqldb.DBConn
	sqldb.DBConn = tx
	dbTx := sqldb.NewDbTransaction(tx)
	t.Cleanup(func() {
		sqldb.DBConn = saved
		tx.Rollback()
	})
	require.NoError(t, tx.Exec(`CREATE SCHEMA delayed_txs; SET LOCAL search_path TO delayed_txs`).Error)
	require.NoError(t, tx.AutoMigrate(&sqldb.BftValidatorSet{}))
	require.NoError(t, tx.Exec(`CREATE TABLE "1_delayed_contracts" (id bigint PRIMARY KEY, contract varchar(255) NOT NULL DEFAULT '',
		key_id bigint NOT NULL DEFAULT 0, block_id bigint NOT NULL DEFAULT 0, every_block bigint NOT NULL DEFAULT 0,
		counter bigint NOT NULL DEFAULT 0, high_rate bigint NOT NULL DEFAULT 0, "limit" bigint NOT NULL DEFAULT 0,
		deleted bigint NOT NULL DEFAULT 0, conditions text NOT NULL DEFAULT '')`).Error)
	useSuite("ECC_P256", "SHA256")
	keys := make([][]byte, 2)
	for i := range keys {
		_, pub, err := crypto.GenNodeKeyPair()
		require.NoError(t, err)
		keys[i] = pub
	}
	set := bft.Validators{Keys: keys}
	require.NoError(t, (&sqldb.BftValidatorSet{FromHeight: 2, Data: set.Marshal()}).Create(dbTx))
	// rows 1 and 2 of the same rate every 10 and 5 blocks from 10, row 3 of a higher rate every 10
	// from 20, row 4 deleted, row 5 at its limit
	require.NoError(t, tx.Exec(`INSERT INTO "1_delayed_contracts" (id, contract, key_id, block_id, every_block, high_rate, deleted, counter, "limit") VALUES
		(2, '@1B', 12, 10, 5, 4, 0, 0, 0), (1, '@1A', 11, 10, 10, 4, 0, 0, 0), (3, '@1C', 13, 20, 10, 9, 0, 0, 0),
		(4, '@1D', 14, 10, 10, 9, 1, 0, 0), (5, '@1E', 15, 10, 10, 9, 0, 2, 2)`).Error)

	delayed := func(height, position int64) ([]*transaction.Transaction, error) {
		b := &Block{BlockData: &types.BlockData{Header: &types.BlockHeader{BlockId: height, Timestamp: 1700000000 + height, NodePosition: position}}}
		return b.delayedTxs(dbTx)
	}
	due := func(height, position int64) []*transaction.Transaction {
		txs, err := delayed(height, position)
		require.NoError(t, err)
		return txs
	}
	check := func(height, position, signer int64, rows ...int64) {
		txs := due(height, position)
		require.Len(t, txs, len(rows), "height %d", height)
		for i, id := range rows {
			s := txs[i].SmartContract()
			require.True(t, bytes.Equal(txs[i].Hash(), transaction.DelayedHash(height, id)), "height %d: row %d at %d", height, id, i)
			require.True(t, s.System)
			require.Equal(t, signer, s.TxSmart.SignedBy)
			require.Equal(t, 10+id, s.TxSmart.KeyID)
			require.Equal(t, 1700000000+height, s.TxSmart.Time)
		}
	}
	check(10, 1, crypto.Address(keys[1]), 1, 2)
	check(15, 0, crypto.Address(keys[0]), 2)
	check(20, 0, crypto.Address(keys[0]), 3, 1, 2)
	check(11, 0, 0)
	// a position no validator of the height has is of no committed block: its signature is checked
	// at that position
	_, err = delayed(20, 5)
	require.ErrorContains(t, err, "no validator at position 5")
}
