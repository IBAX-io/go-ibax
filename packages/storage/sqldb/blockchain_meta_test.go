/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GetMeta reads every field of the block but its data, in a transaction or not; it needs only a
// database, IBAX_TEST_DSN, where it makes a schema of its own that it rolls back
func TestBlockChainGetMeta(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	tx := conn.Begin()
	require.NoError(t, tx.Error)
	saved := DBConn
	DBConn = tx
	t.Cleanup(func() {
		DBConn = saved
		tx.Rollback()
	})
	require.NoError(t, tx.Exec(`CREATE SCHEMA blockchain_meta; SET LOCAL search_path TO blockchain_meta`).Error)
	require.NoError(t, tx.AutoMigrate(&BlockChain{}))
	// a block of ML-DSA transactions: some 36 MB
	data := bytes.Repeat([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9}, 4_000_000)
	want := BlockChain{ID: 7, Hash: []byte("hash"), RollbacksHash: []byte("rollbacks"), Data: data, EcosystemID: 1,
		KeyID: 5, NodePosition: 2, Time: 1_700_000_000, Tx: 9_800, ConsensusMode: 2, CandidateNodes: []byte("[]")}
	require.NoError(t, want.Create(nil))

	full, meta := &BlockChain{}, &BlockChain{}
	start := time.Now()
	found, err := full.GetIn(nil, 7)
	tookFull := time.Since(start)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, want, *full)
	start = time.Now()
	found, err = meta.GetMeta(nil, 7)
	tookMeta := time.Since(start)
	require.NoError(t, err)
	require.True(t, found)
	want.Data = nil
	require.Equal(t, want, *meta)
	t.Logf("block of %d bytes: read whole %v, without its data %v", len(data), tookFull, tookMeta)

	found, err = (&BlockChain{}).GetMeta(&DbTransaction{conn: tx}, 8)
	require.NoError(t, err)
	require.False(t, found)
}
