/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/snapshot"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// stateTestDB makes a database of its own on the server of IBAX_TEST_DSN for the node, its
// DBConn, and for the snapshots, as states with the files under a directory of the test
func stateTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database")
	}
	useSuite("ECC_P256", "SHA256")
	admin, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close() })
	name := "statehash_" + strings.ToLower(rand.Text()[:10])
	_, err = admin.Exec(`CREATE DATABASE ` + name)
	require.NoError(t, err)
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.Database = name
	db := stdlib.OpenDB(*cfg)
	node, err := gorm.Open(postgres.New(postgres.Config{Conn: stdlib.OpenDB(*cfg)}), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	require.NoError(t, node.AutoMigrate(&sqldb.BftStateHash{}))

	savedConn, savedStates := sqldb.DBConn, states
	sqldb.DBConn = node
	dir := t.TempDir()
	states = &stateHashes{open: func() (*sql.DB, string, error) { return db, dir, nil }}
	syspar.SetFirstBlockData(&types.FirstBlock{BftConsensus: 1, SnapshotInterval: 10000, SnapshotDelay: 1000})
	t.Cleanup(func() {
		for h := range states.pending {
			states.wait(h)
		}
		sqldb.DBConn, states = savedConn, savedStates
		syspar.SetFirstBlockData(nil)
		if raw, err := node.DB(); err == nil {
			raw.Close()
		}
		db.Close()
		admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`)
	})
	return db
}

// A block carries the state hash of a snapshot height at the delay after it, and of no other
func TestStateHeight(t *testing.T) {
	syspar.SetFirstBlockData(&types.FirstBlock{BftConsensus: 1, SnapshotInterval: 10000, SnapshotDelay: 1000})
	t.Cleanup(func() { syspar.SetFirstBlockData(nil) })
	for height, want := range map[int64]int64{1000: 0, 10000: 0, 10999: 0, 11000: 10000, 21000: 20000, 21001: 0} {
		require.Equal(t, want, stateHeight(height), height)
	}
	require.True(t, isSnapshotHeight(20000))
	require.False(t, isSnapshotHeight(20001))
	syspar.SetFirstBlockData(&types.FirstBlock{SnapshotInterval: 10000, SnapshotDelay: 1000})
	require.Equal(t, int64(0), stateHeight(11000))
	require.False(t, isSnapshotHeight(10000))
}

// After the commit of a snapshot height the node hashes the state as of the commit, whatever
// commits after it, writes the files of the snapshot and keeps those of the latest two
func TestStateHashesAfterCommit(t *testing.T) {
	db := stateTestDB(t)
	_, err := db.Exec(`CREATE TABLE "1_keys" (id bigint PRIMARY KEY, amount numeric(30));
		INSERT INTO "1_keys" VALUES (1, 100), (2, 5)`)
	require.NoError(t, err)
	s, err := snapshot.Begin(t.Context(), db)
	require.NoError(t, err)
	want, err := s.Hash(t.Context(), 1, nil)
	require.NoError(t, err)
	s.Close()

	states.afterCommit(10001)
	_, status, err := states.get(10001)
	require.NoError(t, err)
	require.Equal(t, stateUnknown, status)

	states.afterCommit(10000)
	_, err = db.Exec(`UPDATE "1_keys" SET amount = 0`)
	require.NoError(t, err)
	hash, status, err := states.wait(10000)
	require.NoError(t, err)
	require.Equal(t, stateDone, status)
	require.Equal(t, want.Hash, hash)

	manifest, err := os.ReadFile(filepath.Join(states.dir, "10000", manifestName))
	require.NoError(t, err)
	var state snapshot.State
	require.NoError(t, json.Unmarshal(manifest, &state))
	require.Equal(t, want.Hash, state.Hash)
	data, err := os.ReadFile(filepath.Join(states.dir, "10000", "1_keys.copy"))
	require.NoError(t, err)
	require.Equal(t, "1\t100\n2\t5\n", string(data))

	require.NoError(t, os.MkdirAll(filepath.Join(states.dir, "5.tmp"), 0o755))
	for _, h := range []int64{20000, 30000} {
		states.afterCommit(h)
		_, status, err := states.wait(h)
		require.NoError(t, err)
		require.Equal(t, stateDone, status)
	}
	entries, err := os.ReadDir(states.dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	require.ElementsMatch(t, []string{"20000", "30000"}, names)
	// the hash outlives the files
	hash, status, err = states.get(10000)
	require.NoError(t, err)
	require.Equal(t, stateDone, status)
	require.Equal(t, want.Hash, hash)
}

// The proposer puts the state hash in its block if it has it, and none else; a validator
// prevotes a block with a state hash only if it has the same, and commits a certified one unless
// its own differs
func TestStateHashRules(t *testing.T) {
	stateTestDB(t)
	block := func(id int64, gen, trial bool, hash []byte) *Block {
		b := &Block{BlockData: &types.BlockData{Header: &types.BlockHeader{BlockId: id, StateHash: hash}}, GenBlock: gen}
		b.trial = trial
		return b
	}
	mine, other := []byte("mine"), []byte("other")
	require.NoError(t, (&sqldb.BftStateHash{Height: 10000, Hash: mine}).Save())
	done := make(chan struct{})
	states.pending = map[int64]chan struct{}{20000: done}

	gen := block(11000, true, true, other)
	require.NoError(t, gen.putStateHash())
	require.Equal(t, mine, gen.Header.StateHash)
	for _, h := range []int64{11001, 21000, 31000} {
		gen := block(h, true, true, other)
		require.NoError(t, gen.putStateHash())
		require.Nil(t, gen.Header.StateHash, h)
	}

	for _, trial := range []bool{true, false} {
		require.NoError(t, block(11000, false, trial, nil).checkStateHash())
		require.NoError(t, block(11000, false, trial, mine).checkStateHash())
		require.ErrorContains(t, block(11000, false, trial, other).checkStateHash(), "the hash of the state of this node")
		require.ErrorContains(t, block(11001, false, trial, mine).checkStateHash(), "without a snapshot")
	}
	// unknown: a proposal is refused, a certified block taken
	require.ErrorContains(t, block(31000, false, true, mine).checkStateHash(), "no state hash of height 30000")
	require.NoError(t, block(31000, false, false, mine).checkStateHash())
	// pending: a proposal is refused, a certified block waits for the hash
	require.ErrorContains(t, block(21000, false, true, mine).checkStateHash(), "no state hash of height 20000")
	checked := make(chan error)
	go func() { checked <- block(21000, false, false, other).checkStateHash() }()
	require.NoError(t, (&sqldb.BftStateHash{Height: 20000, Hash: mine}).Save())
	states.mu.Lock()
	delete(states.pending, 20000)
	states.mu.Unlock()
	close(done)
	require.ErrorContains(t, <-checked, "the hash of the state of this node")
}
