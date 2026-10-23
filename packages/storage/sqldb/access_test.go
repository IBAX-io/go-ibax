/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"os"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"slices"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func testCatalog() *Catalog {
	c := newCatalog()
	c.tables["1_keys"] = &relation{keys: [][]string{{"ecosystem", "id"}, {"ecosystem", "account"}}}
	c.tables["1_members"] = &relation{keys: [][]string{{"account", "ecosystem"}}}
	c.tables["1_tables"] = &relation{keys: [][]string{{"id"}, {"ecosystem", "name"}}}
	c.tables["1_history"] = &relation{keys: [][]string{{"block_id", "id"}}}
	c.tables["1_applications"] = &relation{keys: [][]string{{"id"}}}
	c.tables["1_v"] = &relation{view: true}
	c.parents["1_history_p7"] = "1_history"
	c.funcs["next_id"] = true
	return c
}

func reads(a *Access) []string {
	var rs []string
	for r := range a.reads {
		rs = append(rs, r.Table+":"+strings.ReplaceAll(r.Key, "\x1f", "|"))
	}
	if a.readAll {
		rs = append(rs, "*")
	}
	slices.Sort(rs)
	return rs
}

func TestAccessReads(t *testing.T) {
	for _, c := range []struct {
		sql    string
		vars   []any
		reads  []string
		serial bool
	}{
		{sql: `SELECT * FROM "1_keys" WHERE id = $1 and ecosystem = $2 ORDER BY "1_keys"."id" LIMIT 1`,
			vars: []any{int64(-5), int64(1)}, reads: []string{"1_keys:ecosystem,id=1|-5"}},
		{sql: `SELECT id,amount,"ecosystem" FROM "1_keys" WHERE "ecosystem" = $1 and "id" = $2`,
			vars: []any{"1", "77"}, reads: []string{"1_keys:ecosystem,id=1|77"}},
		{sql: `SELECT * FROM "1_keys" WHERE "ecosystem" = $1 and "account" = $2 and "deleted" = $3`,
			vars: []any{int64(1), "0000-0000", int64(0)}, reads: []string{"1_keys:ecosystem,account=1|0000-0000"}},
		{sql: `SELECT * FROM "1_keys" WHERE "1_keys".ecosystem = 1 AND (id = '7  ')`,
			reads: []string{"1_keys:ecosystem,id=1|7"}},
		{sql: `SELECT * FROM "1_keys" WHERE ecosystem = $1 AND id = $2`,
			vars: []any{decimal.NewFromInt(2), float64(9)}, reads: []string{"1_keys:ecosystem,id=2|9"}},
		{sql: `SELECT * FROM "1_keys" WHERE ecosystem = $1 OR id = $2`, vars: []any{1, 2}, reads: []string{"1_keys:"}},
		{sql: `SELECT * FROM "1_keys" WHERE ecosystem = $1 AND (id = $2 OR id = $3)`, vars: []any{1, 2, 3},
			reads: []string{"1_keys:"}},
		{sql: `SELECT * FROM "1_keys" WHERE ecosystem = $1 AND id = $2::bigint`, vars: []any{1, 2}, reads: []string{"1_keys:"}},
		{sql: `SELECT * FROM "1_keys" WHERE ecosystem = $1 AND id > $2`, vars: []any{1, 2}, reads: []string{"1_keys:"}},
		{sql: `SELECT * FROM "1_keys" WHERE ecosystem = $1 AND id = $2`, vars: []any{1, nil}, reads: []string{"1_keys:"}},
		{sql: `SELECT * FROM "1_keys" k JOIN "1_keys" b ON b.id = k.id WHERE k.ecosystem = 1 AND k.id = 2`,
			reads: []string{"1_keys:"}},
		{sql: `SELECT data.* FROM "1_tables", jsonb_each_text(permissions) AS data WHERE ecosystem = $1 AND name = $2`,
			vars: []any{1, "keys"}, reads: []string{"1_tables:"}},
		{sql: `SELECT * FROM "1_applications" WHERE ecosystem = $1 AND name = $2`, vars: []any{1, "x"},
			reads: []string{"1_applications:"}},
		{sql: `SELECT count(*) FROM "1_history_p7" WHERE block_id = 1 AND id = 2`, reads: []string{"1_history:block_id,id=1|2"}},
		{sql: `SELECT * FROM public."1_keys" k LEFT JOIN "1_members" m ON m.account = k.account WHERE k.id = 1`,
			reads: []string{"1_keys:", "1_members:"}},
		{sql: `SELECT * FROM "1_keys" WHERE id IN (SELECT recipient_id FROM "1_history" WHERE block_id = 3)`,
			reads: []string{"1_history:", "1_keys:"}},
		{sql: `SELECT * FROM ("1_keys" a JOIN "1_members" b USING (account))`, reads: []string{"1_keys:", "1_members:"}},
		{sql: `SELECT * FROM "1_v"`, reads: []string{"*"}},
		{sql: `SELECT * FROM unknown_table`, reads: []string{"*"}},
		{sql: `SELECT extract(year from created_at) FROM "1_keys"`, reads: []string{"*"}},
		{sql: `SELECT relname FROM pg_catalog.pg_class WHERE relname = $1`, vars: []any{"x"}},
		{sql: `SELECT * FROM "1_keys" UNION SELECT * FROM "1_members"`, reads: []string{"1_keys:", "1_members:"}},
		{sql: `WITH x AS (SELECT * FROM "1_keys") SELECT * FROM x`, reads: []string{"*"}},
		{sql: `/* a comment; with a semicolon */ SELECT 1 -- ; and another
		`},
		{sql: `SELECT 'a;b', E'c\';d', $q$e;f$q$ FROM "1_keys" WHERE ecosystem = 1 AND id = 2;`,
			reads: []string{"1_keys:ecosystem,id=1|2"}},
		{sql: `SELECT next_id('1_keys')`, serial: true},
		{sql: `SELECT nextval('s')`, serial: true},
		{sql: `SELECT 1; SELECT 2`, serial: true},
		{sql: `EXPLAIN ANALYZE SELECT 1`, serial: true},
		{sql: `WITH x AS (DELETE FROM "1_keys" RETURNING id) SELECT * FROM x`, serial: true},
		{sql: `CREATE TABLE "1_x" (id int)`, serial: true},
		{sql: `SET TRANSACTION SNAPSHOT '00000003-0000001B-1'`},
	} {
		a := NewAccess(true, testCatalog(), nil)
		a.record(c.sql, c.vars, false)
		if got := reads(a); !slices.Equal(got, c.reads) {
			t.Errorf("%s: reads %q, want %q", c.sql, got, c.reads)
		}
		if (a.Serial() != "") != c.serial {
			t.Errorf("%s: serial %q", c.sql, a.Serial())
		}
	}
}

func TestAccessWrites(t *testing.T) {
	cat := testCatalog()
	a := NewAccess(true, cat, nil)
	tr := &DbTransaction{access: &accessSlot{}}
	tr.SetAccess(a)

	key := func(m map[string]string) func(string) (string, bool) {
		return func(c string) (string, bool) { v, ok := m[c]; return v, ok }
	}
	tr.DeclareWrite(`"1_keys"`, []func(string) (string, bool){key(map[string]string{"id": "5", "ecosystem": "1", "account": "x"})}, 0, false, true)
	a.record(`UPDATE "1_keys" SET amount=amount+$1::numeric WHERE "ecosystem" = $2 and "id" = $3`, []any{"1", 1, 5}, false)
	a.record(`SAVEPOINT "tx-1";`, nil, false)
	tr.DeclareWrite("1_history", []func(string) (string, bool){key(map[string]string{"id": "9", "block_id": "3"})}, 9, true, false)
	a.record(`INSERT INTO "1_history" (block_id,id) VALUES ($1,$2)`, []any{3, 9}, false)
	a.record(`UPDATE "1_members" SET "member_info"=$1 FROM "1_tables" WHERE "ecosystem" = $2 and "account" = $3`, []any{"{}", 1, "x"}, false)
	if len(a.Writes()) != 3 || a.Inserts()["1_history"] != 1 {
		t.Fatalf("writes %+v", a.Writes())
	}
	want := []string{"1_history:block_id,id=3|9", "1_keys:ecosystem,account=1|x", "1_keys:ecosystem,id=1|5", "1_members:", "1_tables:"}
	if got := reads(a); !slices.Equal(got, want) {
		t.Fatalf("implied reads %q, want %q", got, want)
	}
	a.record(`ROLLBACK TO SAVEPOINT "tx-1"`, nil, false)
	if len(a.Writes()) != 1 {
		t.Fatalf("rollback to savepoint: writes %+v", a.Writes())
	}
	a.record(`RELEASE SAVEPOINT "tx-1"`, nil, false)
	if len(a.marks) != 0 {
		t.Fatal("release")
	}
	a.record(`DELETE FROM "1_keys" WHERE id = 1`, nil, true)
	if len(a.Writes()) != 1 {
		t.Fatal("a failed write is kept")
	}
	a.record(`INSERT INTO "1_v" (id) VALUES (1)`, nil, false)
	if a.Serial() == "" {
		t.Fatal("a write to a view runs speculatively")
	}

	w := NewWritten()
	w.AddWrites(a)
	other := NewAccess(true, cat, nil)
	other.record(`SELECT * FROM "1_keys" WHERE ecosystem = 1 AND account = 'x'`, nil, false)
	if r, ok := w.Conflict(other); !ok || r.Key != "ecosystem,account=1\x1fx" {
		t.Fatalf("conflict %v %v", r, ok)
	}
	free := NewAccess(true, cat, nil)
	free.record(`SELECT * FROM "1_keys" WHERE ecosystem = 1 AND id = 6`, nil, false)
	free.record(`SELECT * FROM "1_members" WHERE ecosystem = 1 AND account = 'x'`, nil, false)
	if r, ok := w.Conflict(free); ok {
		t.Fatalf("conflict %v", r)
	}
	w.Add(Row{Table: "1_members"})
	if _, ok := w.Conflict(free); !ok {
		t.Fatal("a write of a whole table")
	}
}

func TestAccessMaxID(t *testing.T) {
	a := NewAccess(true, testCatalog(), map[string]int64{"1_history": 3})
	if id := a.observeMaxID(`"1_history"`, 10); id != 13 {
		t.Fatalf("got %d", id)
	}
	if id := a.observeMaxID("1_history", 10); id != 13 {
		t.Fatalf("got %d", id)
	}
	if id := a.observeMaxID("1_history", 14); id != 14 {
		t.Fatalf("got %d", id)
	}
	if a.MaxIDs()["1_history"] != 13 {
		t.Fatal(a.MaxIDs())
	}
}

func TestTxCacheSavepoints(t *testing.T) {
	_, c := withTxCache(t.Context())
	tr := &DbTransaction{cache: c}
	loads := 0
	load := func() (int64, error) { loads++; return 1, nil }
	cached(tr, "1_keys", "k", load)
	cached(tr, "1_tables", "k", load)
	savepointStatement(c, `SAVEPOINT "a-1";`)
	c.forget("1_keys")
	cached(tr, "1_keys", "k", load)
	savepointStatement(c, `ROLLBACK TO SAVEPOINT "a-1"`)
	cached(tr, "1_keys", "k", load)
	cached(tr, "1_tables", "k", load)
	if loads != 4 {
		t.Fatalf("loads %d, want 4", loads)
	}
	savepointStatement(c, `RELEASE SAVEPOINT "a-1"`)
	if len(c.marks) != 0 || len(c.written) != 0 {
		t.Fatal("release")
	}
	savepointStatement(c, `ROLLBACK TO SAVEPOINT "b"`)
	cached(tr, "1_tables", "k", load)
	if loads != 5 {
		t.Fatalf("unknown savepoint: loads %d, want 5", loads)
	}
}

// TestSnapshotReplay: a run on the snapshot of the transaction of a block records what it reads and
// writes, and its writes replayed in the transaction of the block write there what it wrote.
// Nothing is committed.
func TestSnapshotReplay(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, registerStatementCount(conn))
	require.NoError(t, registerTxCache(conn))
	require.NoError(t, registerAccess(conn))
	saved := DBConn
	DBConn = conn
	t.Cleanup(func() { DBConn = saved })

	block, err := StartTransaction()
	require.NoError(t, err)
	defer block.Rollback()
	cat, err := LoadCatalog(block)
	require.NoError(t, err)
	require.Equal(t, [][]string{{"ecosystem", "id"}, {"ecosystem", "account"}}, cat.keys("1_keys"))
	require.True(t, cat.known("1_history"))
	require.True(t, cat.funcs["next_id"])
	for child, parent := range cat.parents {
		require.True(t, cat.known(parent), child)
	}

	snapshot, clean, err := block.ExportSnapshot()
	require.NoError(t, err)
	require.True(t, clean)
	worker, err := StartSnapshotTransaction(snapshot)
	require.NoError(t, err)
	defer worker.Rollback()
	_, err = StartSnapshotTransaction("x'; DROP TABLE y; --")
	require.Error(t, err)

	const id = int64(-7000001)
	before, err := block.MaxID("1_history")
	require.NoError(t, err)
	spec := NewAccess(true, cat, map[string]int64{"1_history": 2})
	worker.SetAccess(spec)
	require.NoError(t, worker.Savepoint(`"spec-1"`))
	predicted, err := worker.maxID("1_history")
	require.NoError(t, err)
	require.Equal(t, before+2, predicted)
	var n int64
	require.NoError(t, GetDB(worker).Raw(`SELECT count(*) FROM "1_keys" WHERE ecosystem = $1 AND id = $2`, 1, id).Scan(&n).Error)
	require.Zero(t, n)
	worker.DeclareWrite("1_keys", []func(string) (string, bool){func(c string) (string, bool) {
		return map[string]string{"id": "-7000001", "ecosystem": "1", "account": "spec"}[c], true
	}}, id, false, false)
	require.NoError(t, GetDB(worker).Exec(`INSERT INTO "1_keys" (id, account, ecosystem, amount) VALUES (?, 'spec', 1, 5)`, id).Error)
	worker.SetAccess(nil)
	require.NoError(t, worker.RollbackSavepoint(`"spec-1"`))
	require.Empty(t, spec.Serial())
	require.Len(t, spec.Writes(), 1)
	require.Contains(t, spec.reads, Row{"1_keys", "ecosystem,id=1\x1f-7000001"})

	// the write stays out of the snapshot, and out of the block until replayed
	require.NoError(t, GetDB(worker).Raw(`SELECT count(*) FROM "1_keys" WHERE ecosystem = 1 AND id = ?`, id).Scan(&n).Error)
	require.Zero(t, n)
	w := NewWritten()
	require.False(t, func() bool { _, ok := w.Conflict(spec); return ok }())
	require.NoError(t, block.Replay(spec))
	require.NoError(t, GetDB(block).Raw(`SELECT count(*) FROM "1_keys" WHERE ecosystem = 1 AND id = ? AND account = 'spec'`, id).Scan(&n).Error)
	require.EqualValues(t, 1, n)
	w.AddWrites(spec)
	require.True(t, func() bool { _, ok := w.Conflict(spec); return ok }())
	_, clean, err = block.ExportSnapshot()
	require.NoError(t, err)
	require.False(t, clean)
}
