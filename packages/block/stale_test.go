/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// A proposal of a height the node took the block of by its certificate, while the proposal
// waited for writes, is stale: neither its trial nor its speculation runs, as the transactions of
// the block the node has would be found again in the chain
func TestStaleProposal(t *testing.T) {
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
	t.Cleanup(func() {
		sqldb.DBConn = saved
		tx.Rollback()
	})
	require.NoError(t, tx.Exec(`CREATE SCHEMA stale_test; SET LOCAL search_path TO stale_test`).Error)
	require.NoError(t, tx.AutoMigrate(&sqldb.InfoBlock{}))
	require.NoError(t, atTop(10), "no block yet")
	require.NoError(t, tx.Create(&sqldb.InfoBlock{BlockID: 10, Hash: []byte{1}, RollbacksHash: []byte{1}, CandidateNodes: []byte{}}).Error)
	require.NoError(t, atTop(11))
	for _, h := range []int64{10, 9} {
		require.ErrorIs(t, atTop(h), ErrStale, "height %d", h)
	}
	b := &Block{BlockData: &types.BlockData{Header: &types.BlockHeader{BlockId: 10}, PrevHeader: &types.BlockHeader{}}}
	_, err = b.speculate(&sync.Mutex{})
	require.True(t, errors.Is(err, ErrStale), "the speculation of a stale proposal: %v", err)
}
