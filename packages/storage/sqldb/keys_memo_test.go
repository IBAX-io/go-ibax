/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// cachedTestTx is a transaction as StartTransaction begins it, with a cache and its statements
// counted, on the database of IBAX_TEST_DSN, with a temporary table of keys
func cachedTestTx(t *testing.T) *DbTransaction {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	db, err := openGorm(dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	ctx, statements := withStatementCount()
	ctx, cache := withTxCache(ctx)
	conn := db.WithContext(ctx).Begin()
	require.NoError(t, conn.Error)
	t.Cleanup(func() { conn.Rollback() })
	tr := &DbTransaction{conn: conn, statements: statements, cache: cache}
	require.NoError(t, conn.Exec(`CREATE TEMP TABLE "1_keys" (id bigint, pub bytea, multi bigint, amount numeric(30),
		maxpay numeric(30), deleted bigint, blocked bigint, ecosystem bigint, account char(24), conditions text) ON COMMIT DROP`).Error)
	require.NoError(t, conn.Exec(`INSERT INTO "1_keys" VALUES
		(5, '\x0102', 0, 120, 7, 0, 1, 1, '0000-0000-0000-0000-0005', 'C'),
		(6, NULL, NULL, NULL, NULL, NULL, NULL, 1, NULL, NULL),
		(5, '\x03', 0, 9, 0, 1, 0, 2, '0000-0000-0000-0000-0005', '')`).Error)
	return tr
}

// TestKeyMemo: a key the memo gives is the key a read of the database gives, found or not, with
// null columns, with an id set already, after a write of keys and after a rollback to a savepoint
// that takes the write back; the memo reads a row once while keys are not written
func TestKeyMemo(t *testing.T) {
	tr := cachedTestTx(t)
	prefilled := func(eco, id int64) *Key {
		return &Key{ecosystem: eco, ID: id, AccountID: "a", PublicKey: []byte{9}, Amount: "1", Maxpay: "2",
			Deleted: 3, Blocked: 4, Conditions: "x"}
	}
	var km KeyMemo
	// check compares the reads of the memo with a fresh read of each case, and tells the
	// statements the reads of the memo sent
	check := func(name string) int64 {
		t.Helper()
		var sent int64
		for _, c := range []struct{ eco, id, wallet int64 }{
			{1, 0, 5}, {1, 0, 6}, {2, 0, 5}, {1, 0, 7}, {3, 0, 5}, {1, 5, 5}, {1, 6, 5}, {1, 6, 6},
		} {
			want := prefilled(c.eco, c.id)
			wantFound, wantErr := want.Get(NewDbTransaction(tr.conn), c.wallet)
			before := tr.Statements()
			got := prefilled(c.eco, c.id)
			gotFound, gotErr := km.Get(tr, got, c.wallet)
			state := prefilled(c.eco, c.id)
			stateFound, stateErr := km.GetState(tr, state, c.wallet)
			sent += tr.Statements() - before
			require.Equal(t, wantErr, gotErr, "%s %+v", name, c)
			require.Equal(t, wantFound, gotFound, "%s %+v", name, c)
			require.Equal(t, want, got, "%s %+v", name, c)
			require.Equal(t, wantErr, stateErr, "%s %+v", name, c)
			require.Equal(t, wantFound, stateFound, "%s %+v", name, c)
			// a key with an id reads its public key for the state too
			if c.id == 0 {
				want.PublicKey = []byte{9}
			}
			require.Equal(t, want, state, "%s %+v", name, c)
		}
		return sent
	}
	// the cases with an id read each time, two reads each
	const byID = 3 * 2
	require.Equal(t, int64(5+byID), check("first"))
	require.Equal(t, int64(byID), check("again"))

	require.NoError(t, tr.conn.Exec(`UPDATE "1_keys" SET amount = 100, pub = '\x04' WHERE id = 5 AND ecosystem = 1`).Error)
	require.Equal(t, int64(5+byID), check("after an update"))
	require.NoError(t, tr.Savepoint(`"tx-1"`))
	require.NoError(t, tr.conn.Exec(`INSERT INTO "1_keys" (id, amount, ecosystem) VALUES (7, 3, 1)`).Error)
	require.NoError(t, tr.conn.Exec(`UPDATE "1_keys" SET amount = amount - 1 WHERE id = 5 AND ecosystem = 1`).Error)
	require.Equal(t, int64(5+byID), check("in the savepoint"))
	require.NoError(t, tr.RollbackSavepoint(`"tx-1"`))
	require.Equal(t, int64(5+byID), check("after the rollback"))
	require.Equal(t, int64(byID), check("after the rollback, again"))
	require.NoError(t, tr.ReleaseSavepoint(`"tx-1"`))
	require.Equal(t, int64(byID), check("after the release"))
}

// TestKeyMemoState: a read of the state of a key leaves its public key, and a read of the key
// after it reads the public key; a read of the key serves a read of the state
func TestKeyMemoState(t *testing.T) {
	tr := cachedTestTx(t)
	var km KeyMemo
	count := func(read func() (bool, error)) int64 {
		t.Helper()
		before := tr.Statements()
		found, err := read()
		require.NoError(t, err)
		require.True(t, found)
		return tr.Statements() - before
	}
	state := (&Key{}).SetTablePrefix(1)
	require.Equal(t, int64(1), count(func() (bool, error) { return km.GetState(tr, state, 5) }))
	require.Nil(t, state.PublicKey)
	require.Equal(t, "120", state.Amount)
	key := (&Key{}).SetTablePrefix(1)
	require.Equal(t, int64(1), count(func() (bool, error) { return km.Get(tr, key, 5) }))
	require.Equal(t, []byte{1, 2}, key.PublicKey)
	key = (&Key{}).SetTablePrefix(1)
	require.Equal(t, int64(0), count(func() (bool, error) { return km.Get(tr, key, 5) }))
	require.Equal(t, []byte{1, 2}, key.PublicKey)
	state = (&Key{}).SetTablePrefix(1)
	require.Equal(t, int64(0), count(func() (bool, error) { return km.GetState(tr, state, 5) }))
	require.Nil(t, state.PublicKey)
	require.Equal(t, "120", state.Amount)
}

// TestKeyMemoTransactions: the memo answers for the transaction that read the rows only; a
// transaction without a cache reads each time
func TestKeyMemoTransactions(t *testing.T) {
	tr := cachedTestTx(t)
	var km KeyMemo
	count := func(on *DbTransaction) int64 {
		t.Helper()
		before := tr.Statements()
		found, err := km.GetState(on, (&Key{}).SetTablePrefix(1), 5)
		require.NoError(t, err)
		require.True(t, found)
		return tr.Statements() - before
	}
	require.Equal(t, int64(1), count(tr))
	require.Equal(t, int64(0), count(tr))
	other := &DbTransaction{conn: tr.conn, statements: tr.statements, cache: tr.cache}
	require.Equal(t, int64(1), count(other))
	require.Equal(t, int64(0), count(other))
	require.Equal(t, int64(1), count(tr))
	plain := NewDbTransaction(tr.conn)
	require.Equal(t, int64(1), count(plain))
	require.Equal(t, int64(1), count(plain))
	var none *KeyMemo
	before := tr.Statements()
	_, err := none.Get(tr, (&Key{}).SetTablePrefix(1), 5)
	require.NoError(t, err)
	_, err = none.Get(tr, (&Key{}).SetTablePrefix(1), 5)
	require.NoError(t, err)
	require.Equal(t, int64(2), tr.Statements()-before)
}
