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

// TestSealReadsThroughState checks what the proposer reads to seal its block before the state of
// the block is written, the honor_nodes parameter and the reviews that stop nodes, through a view
// of the state after the transactions of the block, is what the block reads once its state is
// written: the validators set the block leaves is the same before and after its writes
func TestSealReadsThroughState(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	bumpSchema()
	db, err := openDB(dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)
	tx := gdb.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { tx.Rollback() })
	block := NewDbTransaction(tx)
	st, err := NewBlockState(block, []string{"", ""})
	require.NoError(t, err)

	type reads struct {
		nodes   string
		stopped []BftReview
	}
	// read reads as the seal does, again once a read the state does not run moved its table to
	// PostgreSQL
	read := func(dbTx func() (*DbTransaction, func() bool, func() error)) reads {
		for range 4 {
			vtx, restart, prepare := dbTx()
			par := &PlatformParameter{}
			_, errPar := par.Get(vtx, "honor_nodes")
			stopped, errStop := (&BftReview{}).GetStopped(vtx, 8)
			if errPar == nil && errStop == nil && !restart() {
				return reads{par.Value, stopped}
			}
			require.True(t, restart(), "%v; %v", errPar, errStop)
			require.NoError(t, prepare())
		}
		t.Fatal("the reads restart without end")
		return reads{}
	}

	// the block changes honor_nodes and stops a node from height 7, in its state
	v := st.NewOrderedView(0, "")
	vtx := v.Transaction()
	require.NoError(t, vtx.conn.Exec(`UPDATE "1_platform_parameters" SET value = ? WHERE name = 'honor_nodes'`, `[{"public_key":"aa"}]`).Error)
	require.NoError(t, vtx.conn.Create(&BftReview{ID: 9001, EvidenceID: 1, PublicKey: []byte{0xaa}, StopHeight: 7}).Error)
	require.NoError(t, vtx.conn.Create(&BftReview{ID: 9002, EvidenceID: 2, PublicKey: []byte{0xbb}, StopHeight: 9}).Error)
	require.False(t, v.Restart())
	st.Commit(0, v)

	sealed := read(func() (*DbTransaction, func() bool, func() error) {
		v := st.NewOrderedView(1, "")
		return v.Transaction(), v.Restart, v.Prepare
	})
	require.Equal(t, `[{"public_key":"aa"}]`, sealed.nodes)
	require.Len(t, sealed.stopped, 1)
	require.Equal(t, int64(9001), sealed.stopped[0].ID)

	require.NoError(t, st.Flush())
	st.End()
	block.ForgetCache()
	written := read(func() (*DbTransaction, func() bool, func() error) {
		return block, func() bool { return false }, func() error { return nil }
	})
	require.Equal(t, written, sealed)
}
