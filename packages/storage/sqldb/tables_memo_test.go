/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// tablesTestTx is cachedTestTx with a temporary table of tables
func tablesTestTx(t *testing.T) *DbTransaction {
	tr := cachedTestTx(t)
	require.NoError(t, tr.conn.Exec(`CREATE TEMP TABLE "1_tables" (id bigint NOT NULL, name varchar(100) NOT NULL,
		permissions jsonb, columns jsonb, conditions text NOT NULL DEFAULT '', app_id bigint NOT NULL DEFAULT 1,
		ecosystem bigint NOT NULL DEFAULT 1) ON COMMIT DROP`).Error)
	require.NoError(t, tr.conn.Exec(`INSERT INTO "1_tables" VALUES
		(1, 'keys', '{"insert": "true", "update": "C1", "new_column": "N", "read": "", "filter": "F", "extra": 5}',
			'{"amount": "A", "pub": "{\"update\": \"U\", \"read\": \"R\"}"}', 'ContractConditions("C")', 1, 1),
		(2, 'history', '{"insert": "I", "update": "false", "new_column": "N"}', '{}', '', 2, 1),
		(3, 'keys', '{"insert": "2", "update": "2", "new_column": "2"}', '{"amount": "2"}', '', 1, 2)`).Error)
	return tr
}

// forgetCache empties the cache of the transaction without moving a generation, so that only the
// memo can answer a read without a statement
func forgetCache(tr *DbTransaction) {
	tr.cache.mu.Lock()
	clear(tr.cache.byTable)
	tr.cache.mu.Unlock()
}

// TestTableMemo: the rows and permissions the memo gives are those a read of the database gives,
// found or not, into a table set already, after a change of permissions, of columns, a new table,
// a rollback to a savepoint that takes a change back and a rollback to a savepoint the cache does
// not know; the memo answers without a statement while 1_tables is not written
func TestTableMemo(t *testing.T) {
	tr := tablesTestTx(t)
	prefilled := func(eco, id int64) *Table {
		return &Table{ID: id, Name: "x", Permissions: Permissions{Insert: "p"}, Columns: "c", Conditions: "k",
			AppID: 9, Ecosystem: eco}
	}
	var tm TableMemo
	cases := []struct {
		eco, id int64
		name    string
	}{
		{1, 0, "keys"}, {1, 0, "history"}, {2, 0, "keys"}, {1, 0, "nope"}, {3, 0, "keys"}, {1, 1, "keys"}, {1, 2, "keys"},
	}
	// check compares the reads of the memo with a fresh read of each case, and tells the
	// statements the reads of the memo sent
	check := func(name string) int64 {
		t.Helper()
		var sent int64
		fresh := NewDbTransaction(tr.conn)
		for _, c := range cases {
			want := prefilled(c.eco, c.id)
			wantFound, wantErr := want.Get(fresh, c.name)
			wantPerms, wantPermsErr := prefilled(c.eco, c.id).GetPermissions(fresh, c.name, "")
			before := tr.Statements()
			got := prefilled(c.eco, c.id)
			gotFound, gotErr := tm.Get(tr, got, c.name)
			gotPerms, gotPermsErr := tm.Permissions(tr, prefilled(c.eco, c.id), c.name)
			sent += tr.Statements() - before
			require.Equal(t, wantErr, gotErr, "%s %+v", name, c)
			require.Equal(t, wantFound, gotFound, "%s %+v", name, c)
			require.Equal(t, want, got, "%s %+v", name, c)
			require.Equal(t, wantPermsErr, gotPermsErr, "%s %+v", name, c)
			require.Equal(t, wantPerms, gotPerms, "%s %+v", name, c)
		}
		return sent
	}
	// the cases with an id read their row each time; a permission is read once by table
	const byID = 2
	const first = 5*2 + byID
	require.Equal(t, int64(first), check("first"))
	forgetCache(tr)
	require.Equal(t, int64(byID), check("again"))

	// a change of the permissions of a table
	require.NoError(t, tr.conn.Exec(`UPDATE "1_tables" SET permissions = '{"insert": "false", "update": "C2"}' WHERE id = 1`).Error)
	require.Equal(t, int64(first), check("after a change of permissions"))
	forgetCache(tr)
	require.Equal(t, int64(byID), check("after a change of permissions, again"))
	// a new column
	require.NoError(t, tr.conn.Exec(`UPDATE "1_tables" SET columns = columns || '{"maxpay": "M"}' WHERE id = 1`).Error)
	require.Equal(t, int64(first), check("after a new column"))
	// a new table
	require.NoError(t, tr.conn.Exec(`INSERT INTO "1_tables" (id, name, permissions, columns, ecosystem) VALUES
		(4, 'nope', '{"insert": "N"}', '{"a": "B"}', 1)`).Error)
	require.Equal(t, int64(first), check("after a new table"))
	forgetCache(tr)
	require.Equal(t, int64(byID), check("after a new table, again"))

	// a change taken back by a rollback to a savepoint
	require.NoError(t, tr.Savepoint(`"tx-1"`))
	require.NoError(t, tr.conn.Exec(`UPDATE "1_tables" SET permissions = '{"read": "R"}', columns = '{}' WHERE ecosystem = 1`).Error)
	require.NoError(t, tr.conn.Exec(`DELETE FROM "1_tables" WHERE id = 2`).Error)
	require.Equal(t, int64(first), check("in the savepoint"))
	require.NoError(t, tr.RollbackSavepoint(`"tx-1"`))
	require.Equal(t, int64(first), check("after the rollback"))
	forgetCache(tr)
	require.Equal(t, int64(byID), check("after the rollback, again"))
	require.NoError(t, tr.ReleaseSavepoint(`"tx-1"`))
	forgetCache(tr)
	require.Equal(t, int64(byID), check("after the release"))

	// a rollback to a savepoint the cache does not know: the cache did not follow its statement
	_, err := tr.conn.Statement.ConnPool.ExecContext(tr.conn.Statement.Context, `SAVEPOINT unknown`)
	require.NoError(t, err)
	require.NoError(t, tr.conn.Exec(`UPDATE "1_tables" SET permissions = '{"update": "U"}', columns = '{"x": "y"}' WHERE id IN (1, 3)`).Error)
	require.Equal(t, int64(first), check("in the unknown savepoint"))
	forgetCache(tr)
	require.Equal(t, int64(byID), check("in the unknown savepoint, again"))
	require.NoError(t, tr.RollbackSavepoint(`unknown`))
	require.Equal(t, int64(first), check("after the rollback to the unknown savepoint"))

	// a write of another table leaves the memo
	require.NoError(t, tr.conn.Exec(`UPDATE "1_keys" SET amount = 1 WHERE id = 5`).Error)
	forgetCache(tr)
	require.Equal(t, int64(byID), check("after a write of keys"))
}

// TestTableMemoTransactions: the memo answers for the transaction that read the rows only; a
// transaction without a cache and a nil memo read each time
func TestTableMemoTransactions(t *testing.T) {
	tr := tablesTestTx(t)
	var tm TableMemo
	count := func(m *TableMemo, on *DbTransaction) int64 {
		t.Helper()
		before := tr.Statements()
		tbl := (&Table{}).setEcosystem(1)
		found, err := m.Get(on, tbl, "keys")
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, int64(1), tbl.ID)
		perms, err := m.Permissions(on, tbl, "keys")
		require.NoError(t, err)
		require.Equal(t, "C1", perms["update"])
		return tr.Statements() - before
	}
	require.Equal(t, int64(2), count(&tm, tr))
	forgetCache(tr)
	require.Equal(t, int64(0), count(&tm, tr))
	other := &DbTransaction{conn: tr.conn, statements: tr.statements, cache: tr.cache}
	forgetCache(tr)
	require.Equal(t, int64(2), count(&tm, other))
	forgetCache(tr)
	require.Equal(t, int64(0), count(&tm, other))
	forgetCache(tr)
	require.Equal(t, int64(2), count(&tm, tr))
	plain := NewDbTransaction(tr.conn)
	require.Equal(t, int64(2), count(&tm, plain))
	require.Equal(t, int64(2), count(&tm, plain))
	forgetCache(tr)
	require.Equal(t, int64(2), count(nil, tr))
	forgetCache(tr)
	require.Equal(t, int64(2), count(nil, tr))
}

func (t *Table) setEcosystem(eco int64) *Table {
	t.Ecosystem = eco
	return t
}
