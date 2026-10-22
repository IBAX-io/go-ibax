/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"errors"
	"testing"
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
