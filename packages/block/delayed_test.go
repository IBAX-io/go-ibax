/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
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
// of the node key at the position of the header; it needs only a database, IBAX_TEST_DSN, where it
// makes a schema of its own that it rolls back
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
		// the honor nodes of the parameters as they were
		require.NoError(t, tx.Where("true").Delete(&sqldb.PlatformParameter{}).Error)
		syspar.SysUpdate(dbTx)
		sqldb.DBConn = saved
		tx.Rollback()
	})
	require.NoError(t, tx.Exec(`CREATE SCHEMA delayed_txs; SET LOCAL search_path TO delayed_txs`).Error)
	require.NoError(t, tx.AutoMigrate(&sqldb.PlatformParameter{}))
	require.NoError(t, tx.Exec(`CREATE TABLE "1_delayed_contracts" (id bigint PRIMARY KEY, contract varchar(255) NOT NULL DEFAULT '',
		key_id bigint NOT NULL DEFAULT 0, block_id bigint NOT NULL DEFAULT 0, every_block bigint NOT NULL DEFAULT 0,
		counter bigint NOT NULL DEFAULT 0, high_rate bigint NOT NULL DEFAULT 0, "limit" bigint NOT NULL DEFAULT 0,
		deleted bigint NOT NULL DEFAULT 0, conditions text NOT NULL DEFAULT '')`).Error)
	useSuite("ECC_P256", "SHA256")
	var nodes []map[string]any
	keys := make([][]byte, 2)
	for i := range keys {
		_, pub, err := crypto.GenNodeKeyPair()
		require.NoError(t, err)
		keys[i] = pub
		nodes = append(nodes, map[string]any{"tcp_address": fmt.Sprintf("127.0.0.1:%d", 7078+i),
			"api_address": fmt.Sprintf("http://127.0.0.1:%d", 7079+i), "public_key": crypto.PubToHex(pub), "unban_time": "0"})
	}
	value, err := json.Marshal(nodes)
	require.NoError(t, err)
	require.NoError(t, tx.Create(&sqldb.PlatformParameter{ID: 1, Name: syspar.HonorNodes, Value: string(value)}).Error)
	require.NoError(t, syspar.SysUpdate(dbTx))
	// rows 1 and 2 of the same rate every 10 and 5 blocks from 10, row 3 of a higher rate every 10
	// from 20, row 4 deleted, row 5 at its limit
	require.NoError(t, tx.Exec(`INSERT INTO "1_delayed_contracts" (id, contract, key_id, block_id, every_block, high_rate, deleted, counter, "limit") VALUES
		(2, '@1B', 12, 10, 5, 4, 0, 0, 0), (1, '@1A', 11, 10, 10, 4, 0, 0, 0), (3, '@1C', 13, 20, 10, 9, 0, 0, 0),
		(4, '@1D', 14, 10, 10, 9, 1, 0, 0), (5, '@1E', 15, 10, 10, 9, 0, 2, 2)`).Error)

	due := func(height, position int64) []*transaction.Transaction {
		b := &Block{BlockData: &types.BlockData{Header: &types.BlockHeader{BlockId: height, Timestamp: 1700000000 + height, NodePosition: position}}}
		txs, err := b.delayedTxs(dbTx)
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
	// a position no honor node has: the transactions name no node, and their check discards them
	check(20, 5, 0, 3, 1, 2)
}
