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
	"gorm.io/gorm/logger"
)

// A node sends the blocks after a block that fit in the bytes of a request, and the first one
// whatever its length; it needs only a database, IBAX_TEST_DSN, where it makes a schema of its own
// that it rolls back
func TestGetBlocksWithin(t *testing.T) {
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
	require.NoError(t, tx.Exec(`CREATE SCHEMA blocks_within; SET LOCAL search_path TO blocks_within`).Error)
	require.NoError(t, tx.AutoMigrate(&BlockChain{}))
	// blocks 1..10 of 100 bytes but block 4 of 1000
	for id := int64(1); id <= 10; id++ {
		size := 100
		if id == 4 {
			size = 1000
		}
		require.NoError(t, tx.Create(&BlockChain{ID: id, Hash: []byte{1}, RollbacksHash: []byte{1},
			Data: make([]byte, size), CandidateNodes: []byte{0}}).Error)
	}
	ids := func(blocks []BlockChain) (out []int64) {
		for _, b := range blocks {
			out = append(out, b.ID)
		}
		return
	}
	b := &BlockChain{}
	for _, c := range []struct {
		after int64
		limit int
		bytes int64
		want  []int64
	}{
		{0, 1000, 300, []int64{1, 2, 3}},
		{0, 2, 300, []int64{1, 2}},
		{0, 1000, 299, []int64{1, 2}},
		// a block bigger than the bytes goes alone
		{3, 1000, 300, []int64{4}},
		{2, 1000, 300, []int64{3}},
		{4, 1000, 10000, []int64{5, 6, 7, 8, 9, 10}},
		{10, 1000, 300, nil},
	} {
		got, err := b.GetBlocksWithin(c.after, c.limit, c.bytes)
		require.NoError(t, err)
		require.Equal(t, c.want, ids(got), "after %d, %d blocks, %d bytes", c.after, c.limit, c.bytes)
	}
}
