/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// afterTxs are n transactions of the block blockID with their logs
func afterTxs(blockID int64, from, n int) []*types.AfterTx {
	txs := make([]*types.AfterTx, n)
	for i := range txs {
		k := from + i
		txs[i] = &types.AfterTx{Lts: &types.LogTransaction{
			Hash:  []byte{byte(k >> 24), byte(k >> 16), byte(k >> 8), byte(k)},
			Block: blockID,
		}}
	}
	return txs
}

// TestStreamWritesAllItWasGiven checks the writer of the records of the transactions writes, by
// the stop, every part the block gave it, in statements of at most afterChunk rows
func TestStreamWritesAllItWasGiven(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	sqlDB, err := conn.DB()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	tx := conn.Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.NoError(t, tx.Exec(`CREATE SCHEMA after_stream_test; SET LOCAL search_path TO after_stream_test`).Error)
	require.NoError(t, tx.AutoMigrate(&sqldb.LogTransaction{}))

	const parts = 3
	s := newAfterStream(sqldb.NewDbTransaction(tx))
	afters := &types.AfterTxs{}
	for i := range parts {
		afters.Txs = append(afters.Txs, afterTxs(1, i*afterPartTxs, afterPartTxs)...)
		s.send(afters)
	}
	s.release()
	written, err := s.stop()
	require.NoError(t, err)
	require.Equal(t, parts*afterPartTxs, written.txs)
	require.Len(t, written.lts, parts*afterPartTxs)
	var rows int64
	require.NoError(t, tx.Table("log_transactions").Count(&rows).Error)
	require.Equal(t, int64(parts*afterPartTxs), rows)
}
