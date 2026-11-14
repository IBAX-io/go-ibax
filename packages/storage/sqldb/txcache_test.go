/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWritten(t *testing.T) {
	for _, c := range []struct {
		sql   string
		table string
		none  bool
		all   bool
	}{
		{sql: `SELECT * FROM "1_keys" WHERE id = $1`, none: true},
		{sql: "select\n\tid from \"1_tables\"", none: true},
		{sql: `SAVEPOINT "tx-1"`, none: true},
		{sql: `RELEASE SAVEPOINT "tx-1"`, none: true},
		{sql: `SET LOCAL statement_timeout = 1000`, none: true},
		{sql: `SHOW server_version`, none: true},
		{sql: `INSERT INTO "1_keys" (id) VALUES ($1)`, table: `"1_keys"`},
		{sql: `insert into rollback_tx (id) values ($1)`, table: `rollback_tx`},
		{sql: `UPDATE "1_parameters" SET value = $1 WHERE id = $2`, table: `"1_parameters"`},
		{sql: `update only public."1_app_params" set value = $1`, table: `"1_app_params"`},
		{sql: `UPDATE public . "1_tables" SET columns = $1`, table: `"1_tables"`},
		{sql: `DELETE FROM "1_history" WHERE id = $1`, table: `"1_history"`},
		{sql: `delete from public.info_block`, table: `info_block`},
		{sql: `ROLLBACK TO SAVEPOINT "tx-1"`, all: true},
		{sql: `ALTER TABLE "1_keys" ADD COLUMN x text`, all: true},
		{sql: `CREATE INDEX "1_keys_x" ON "1_keys" (x)`, all: true},
		{sql: `DROP TABLE "1_x"`, all: true},
		{sql: `TRUNCATE "1_keys"`, all: true},
		{sql: `DO $$ BEGIN END $$`, all: true},
		{sql: `WITH x AS (DELETE FROM "1_keys" RETURNING id) SELECT * FROM x`, all: true},
		{sql: `UPDATE "1_keys" SET x = 1; DELETE FROM "1_tables"`, all: true},
		{sql: `select 1; update "1_keys" set x = 1`, all: true},
		{sql: `-- a comment
UPDATE "1_keys" SET x = 1`, all: true},
		{sql: `UPDATE "1_keys" SET x = 1;`, table: `"1_keys"`},
	} {
		table, none, all := written(c.sql)
		if table != c.table || none != c.none || all != c.all {
			t.Errorf("%q: got %q %v %v, want %q %v %v", c.sql, table, none, all, c.table, c.none, c.all)
		}
	}
}

func TestTxCache(t *testing.T) {
	_, c := withTxCache(t.Context())
	tr := &DbTransaction{cache: c}
	loads := 0
	load := func() (int64, error) { loads++; return 7, nil }
	for range 3 {
		if v, err := cached(tr, `"1_Keys"`, "maxid", load); v != 7 || err != nil {
			t.Fatalf("got %d %v", v, err)
		}
	}
	if loads != 1 {
		t.Fatalf("loads %d, want 1", loads)
	}
	c.forget("1_tables")
	cached(tr, "1_keys", "maxid", load)
	if loads != 1 {
		t.Fatalf("another table forgot it: loads %d", loads)
	}
	c.forget(`"1_keys"`)
	cached(tr, "1_keys", "maxid", load)
	if loads != 2 {
		t.Fatalf("its table did not forget it: loads %d", loads)
	}
	cached(tr, schemaTable, "indexes:1_keys", load)
	c.forgetAll()
	cached(tr, "1_keys", "maxid", load)
	cached(tr, schemaTable, "indexes:1_keys", load)
	if loads != 5 {
		t.Fatalf("forgetAll: loads %d, want 5", loads)
	}

	fail := errors.New("fail")
	if _, err := cached(tr, "1_x", "k", func() (int, error) { return 0, fail }); err != fail {
		t.Fatalf("got %v", err)
	}
	if _, ok := c.get("1_x", "k"); ok {
		t.Fatal("an error is cached")
	}

	if _, ok := tr.CachedMaxID("1_keys"); !ok {
		t.Fatal("max id not cached")
	}
	c.forget("1_keys")
	tr.KeepMaxID("1_keys", 8)
	if id, _ := tr.CachedMaxID("1_keys"); id != 8 {
		t.Fatalf("max id %d, want 8", id)
	}

	var none *DbTransaction
	loads = 0
	cached(none, "1_keys", "maxid", load)
	cached(none, "1_keys", "maxid", load)
	if loads != 2 {
		t.Fatalf("no transaction: loads %d, want 2", loads)
	}
}

func TestCachedFirst(t *testing.T) {
	_, c := withTxCache(t.Context())
	tr := &DbTransaction{cache: c}
	finds := 0
	row := Table{ID: 3, Name: "keys", Ecosystem: 1, Permissions: Permissions{Insert: "true"}}
	find := func(dest *Table) func() (bool, error) {
		return func() (bool, error) { finds++; *dest = row; return true, nil }
	}
	a := &Table{Ecosystem: 1}
	if found, err := cachedFirst(tr, "1_tables", "table:1:keys", a, a.ID != 0, find(a)); !found || err != nil || *a != row {
		t.Fatalf("got %v %v %+v", found, err, a)
	}
	b := &Table{Ecosystem: 1}
	if found, _ := cachedFirst(tr, "1_tables", "table:1:keys", b, b.ID != 0, find(b)); !found || *b != row || finds != 1 {
		t.Fatalf("hit: got %v %+v, finds %d", found, b, finds)
	}
	b.Permissions.Insert = "false"
	d := &Table{Ecosystem: 1}
	cachedFirst(tr, "1_tables", "table:1:keys", d, d.ID != 0, find(d))
	if d.Permissions.Insert != "true" {
		t.Fatal("a change of a row read changes the cache")
	}
	e := &Table{ID: 9, Ecosystem: 1}
	cachedFirst(tr, "1_tables", "table:1:keys", e, e.ID != 0, find(e))
	if finds != 2 {
		t.Fatalf("a dest with an id is read: finds %d", finds)
	}

	missing := &Table{Ecosystem: 1, Name: "before"}
	notFound := func() (bool, error) { finds++; return false, nil }
	for range 2 {
		if found, _ := cachedFirst(tr, "1_tables", "table:1:none", missing, false, notFound); found || missing.Name != "before" {
			t.Fatalf("not found: got %v %+v", found, missing)
		}
	}
	if finds != 3 {
		t.Fatalf("not found is cached: finds %d", finds)
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

// TestTxCacheGeneration: the generation of a table moves on a statement that may write it, on any
// change of everything and on a rollback that takes back a write of it; reads, savepoints and
// their release, and writes of other tables leave it
func TestTxCacheGeneration(t *testing.T) {
	_, c := withTxCache(t.Context())
	gen := c.generation(`"1_keys"`)
	step := func(name string, f func(), moves bool) {
		t.Helper()
		f()
		g := c.generation("1_KEYS")
		if (g != gen) != moves {
			t.Fatalf("%s: generation %d after %d, moves %v", name, g, gen, moves)
		}
		gen = g
	}
	step("read", func() { followStatement(withCache(c), `SELECT * FROM "1_keys" WHERE id = $1`) }, false)
	step("other table", func() { c.forget("1_tables") }, false)
	step("update", func() { followStatement(withCache(c), `UPDATE "1_keys" SET amount = $1`) }, true)
	step("schema", func() { c.forgetAll() }, true)
	step("savepoint", func() { savepointStatement(c, `SAVEPOINT "a"`) }, false)
	step("rollback, nothing written", func() { savepointStatement(c, `ROLLBACK TO SAVEPOINT "a"`) }, false)
	step("insert", func() { c.forget(`"1_keys"`) }, true)
	step("rollback of another table", func() {
		savepointStatement(c, `SAVEPOINT "b"`)
		c.forget("1_tables")
		savepointStatement(c, `ROLLBACK TO SAVEPOINT "b"`)
	}, false)
	step("rollback of the insert", func() { savepointStatement(c, `ROLLBACK TO SAVEPOINT "a"`) }, true)
	step("release", func() { savepointStatement(c, `RELEASE SAVEPOINT "a"`) }, false)
	step("rollback, unknown savepoint", func() { savepointStatement(c, `ROLLBACK TO SAVEPOINT "c"`) }, true)
	step("rollback of a change of everything", func() {
		savepointStatement(c, `SAVEPOINT "d"`)
		c.forgetAll()
		gen = c.generation("1_keys")
		savepointStatement(c, `ROLLBACK TO SAVEPOINT "d"`)
	}, true)
}

func withCache(c *txCache) context.Context {
	return context.WithValue(context.Background(), txCacheKey{}, c)
}

// refCacheTable is the name of the table the cache keys by, as it was written
func refCacheTable(table string) string {
	return strings.ToLower(strings.Trim(table, `"`))
}

// TestCacheTable: the name the cache keys by is the name without its quotes in lower case, for
// the names of the tables, each string of one and two bytes and a set of runes
func TestCacheTable(t *testing.T) {
	names := []string{"", `"`, `""`, "1_keys", `"1_keys"`, "1_KEYS", `"1_Keys"`, `""1_keys""`, `1_ke"ys`,
		"1_tables", "1_parameters", "1_app_params", "1_history", "*", "1_ключи", "1_KİYS", "1_\u212a", "\xff1_keys"}
	for c := 0; c < 256; c++ {
		names = append(names, string([]byte{byte(c)}), "1_keys"+string([]byte{byte(c)}))
		for d := 0; d < 256; d++ {
			names = append(names, string([]byte{byte(c), byte(d)}))
		}
	}
	for r := rune(0x80); r < 0x3000; r += 7 {
		names = append(names, "a"+string(r), string(r)+"Z")
	}
	for _, s := range names {
		if got, want := cacheTable(s), refCacheTable(s); got != want {
			t.Fatalf("cacheTable(%q) = %q, want %q", s, got, want)
		}
	}
}

func FuzzCacheTable(f *testing.F) {
	for _, s := range []string{"1_keys", `"1_KEYS"`, "1_ключи", "\xff", `"`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if got, want := cacheTable(s), refCacheTable(s); got != want {
			t.Fatalf("cacheTable(%q) = %q, want %q (valid %v)", s, got, want, utf8.ValidString(s))
		}
	})
}

// TestCacheKeys: the rows of tables, their permissions, the parameters and the parameters of the
// applications are keyed in the cache by the keys they were, so that a read finds what the cache
// has under the key
func TestCacheKeys(t *testing.T) {
	_, c := withTxCache(t.Context())
	tr := &DbTransaction{cache: c}
	for _, eco := range []int64{1, 0, -7, 1 << 62, -1 << 63} {
		for _, name := range []string{"keys", "", "a:b", "ключи"} {
			table := Table{ID: 3, Name: name, Ecosystem: eco, Columns: fmt.Sprint(eco)}
			c.put("1_tables", fmt.Sprintf("table:%d:%s", eco, name), firstRow[Table]{found: true, row: table})
			got := &Table{Ecosystem: eco}
			if found, err := got.Get(tr, name); !found || err != nil || *got != table {
				t.Fatalf("table %d %q: %v %v %+v", eco, name, found, err, got)
			}
			for _, jsonKey := range []string{"", "insert"} {
				perms := map[string]string{"k": fmt.Sprint(eco, name, jsonKey)}
				c.put("1_tables", fmt.Sprintf("perm:%d:%s:%s", eco, name, jsonKey), perms)
				gotPerms, err := (&Table{Ecosystem: eco}).GetPermissions(tr, name, jsonKey)
				if err != nil || gotPerms["k"] != perms["k"] {
					t.Fatalf("permissions %d %q %q: %v %v", eco, name, jsonKey, gotPerms, err)
				}
			}
			param := StateParameter{ecosystem: eco, ID: 4, Name: name, Value: fmt.Sprint(eco)}
			c.put("1_parameters", fmt.Sprintf("param:%d:%s", eco, name), firstRow[StateParameter]{found: true, row: param})
			gotParam := &StateParameter{ecosystem: eco}
			if found, err := gotParam.Get(tr, name); !found || err != nil || *gotParam != param {
				t.Fatalf("parameter %d %q: %v %v %+v", eco, name, found, err, gotParam)
			}
			for _, app := range []int64{1, 0, -3, 17, -1 << 63} {
				ap := AppParam{ecosystem: eco, ID: 5, AppID: app, Name: name, Value: fmt.Sprint(eco, app)}
				c.put("1_app_params", fmt.Sprintf("appparam:%d:%d:%s", eco, app, name), firstRow[AppParam]{found: true, row: ap})
				gotAP := &AppParam{ecosystem: eco}
				if found, err := gotAP.Get(tr, app, name); !found || err != nil || *gotAP != ap {
					t.Fatalf("app parameter %d %d %q: %v %v %+v", eco, app, name, found, err, gotAP)
				}
			}
		}
	}
}
