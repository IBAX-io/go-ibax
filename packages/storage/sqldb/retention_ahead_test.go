/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"os"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// commitRows writes the block of the height and its certificate, as the commit of an ordered block
// does
func commitRows(db *DbTransaction, time, height int64) error {
	if err := (&BlockChain{ID: height, Hash: []byte{byte(height)}, RollbacksHash: []byte{},
		Data: []byte{0}, CandidateNodes: []byte{0}, Time: time}).Create(db); err != nil {
		return err
	}
	return (&BftCertificate{Height: height, Hash: []byte{byte(height)}, Data: []byte{0}}).Create(db)
}

// The executor runs the retention at a boundary block while the consensus committed the blocks
// after it: the committed blocks and their certificates stay, of a range the boundary keeps, and
// the committed and the executed heights are those of the commits and of the execution
func TestRetainWithCommitsAhead(t *testing.T) {
	db := retentionDB(t)
	conf.Config.Storage.Archive = false
	T := int64(1) << 32
	chainRows(t, db, T)
	require.NoError(t, Retain(db, pb, testWindow))
	addBlocks(t, db, T, 2*pb)
	executeBlock(t, db, T, 2*pb)
	for _, h := range []int64{2*pb + 1, 2*pb + 2} {
		require.NoError(t, commitRows(db, T, h))
	}
	require.NoError(t, Retain(db, 2*pb, testWindow))

	committed, err := CommittedHeight()
	require.NoError(t, err)
	require.EqualValues(t, 2*pb+2, committed)
	executed, err := ExecutedHeight()
	require.NoError(t, err)
	require.EqualValues(t, 2*pb, executed)
	for _, h := range []int64{2*pb + 1, 2*pb + 2} {
		found, err := (&BlockChain{}).Get(h)
		require.NoError(t, err)
		require.True(t, found, "block %d", h)
		found, err = (&BftCertificate{}).Get(nil, h)
		require.NoError(t, err)
		require.True(t, found, "the certificate of %d", h)
	}
	found, err := (&BlockChain{}).Get(5)
	require.NoError(t, err)
	require.False(t, found, "range 0 left at 2pb")
}

// The commit of a block and the retention of the boundary block the executor runs at the same time
// lock block_chain and bft_certificates in the same order: whichever takes them first, the other
// waits for it and goes on, with no deadlock. The commit runs in a transaction of its own on a
// range the database has; both transactions are rolled back.
func TestRetainAndCommitConcurrent(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	conf.Config.Storage.Archive = false
	T := int64(1) << 32
	const height = pb + 7
	commitConn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	var exists int64
	require.NoError(t, commitConn.Raw(`SELECT count(*) FROM "block_chain" WHERE "id" = ?`, height).Row().Scan(&exists))
	require.Zero(t, exists, "the database has block %d", height)

	// retain runs the boundaries pb and 2pb in the transaction of the executor, range 0 leaving at 2pb
	retain := func(db *DbTransaction) error {
		if err := Retain(db, pb, testWindow); err != nil {
			return err
		}
		if err := (&BlockChain{ID: 2 * pb, Hash: []byte{1}, RollbacksHash: []byte{0}, Data: []byte{0},
			CandidateNodes: []byte{0}, Time: T}).Create(db); err != nil {
			return err
		}
		return Retain(db, 2*pb, testWindow)
	}
	blocked := func(done <-chan error, what string) {
		select {
		case err := <-done:
			t.Fatalf("%s did not wait: %v", what, err)
		case <-time.After(300 * time.Millisecond):
		}
	}
	finished := func(done <-chan error, what string) {
		select {
		case err := <-done:
			require.NoError(t, err, what)
		case <-time.After(30 * time.Second):
			t.Fatalf("%s did not finish", what)
		}
	}

	t.Run("commit first", func(t *testing.T) {
		db := retentionDB(t)
		chainRows(t, db, T)
		commit := commitConn.Begin()
		require.NoError(t, commit.Error)
		defer commit.Rollback()
		cdb := NewDbTransaction(commit)
		require.NoError(t, (&BlockChain{ID: height, Hash: []byte{7}, RollbacksHash: []byte{}, Data: []byte{0},
			CandidateNodes: []byte{0}, Time: T}).Create(cdb))
		done := make(chan error, 1)
		go func() { done <- retain(db) }()
		blocked(done, "the retention")
		require.NoError(t, (&BftCertificate{Height: height, Hash: []byte{7}, Data: []byte{0}}).Create(cdb))
		require.NoError(t, commit.Rollback().Error)
		finished(done, "the retention")
	})

	t.Run("retention first", func(t *testing.T) {
		db := retentionDB(t)
		chainRows(t, db, T)
		require.NoError(t, retain(db))
		commit := commitConn.Begin()
		require.NoError(t, commit.Error)
		defer commit.Rollback()
		done := make(chan error, 1)
		go func() { done <- commitRows(NewDbTransaction(commit), T, height) }()
		blocked(done, "the commit")
		require.NoError(t, GetDB(db).Rollback().Error)
		finished(done, "the commit")
	})
}
