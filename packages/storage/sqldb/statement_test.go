/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// viaDatabaseSQL tells the statement the trace is called for runs through database/sql: on the
// direct path of a view only database/sql.(*Conn).Raw is on the stack
func viaDatabaseSQL() bool {
	pcs := make([]uintptr, 64)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		f, more := frames.Next()
		if strings.HasPrefix(f.Function, "database/sql.") && f.Function != "database/sql.(*Conn).Raw" {
			return true
		}
		if !more {
			return false
		}
	}
}

// directScript runs statements of each kind of the paths that run without database/sql in a view
// and writes what each returned
func directScript(tr *DbTransaction, w *strings.Builder) {
	rec := func(name string, v ...any) {
		fmt.Fprintf(w, "%s:", name)
		for _, x := range v {
			if err, ok := x.(error); ok {
				fmt.Fprintf(w, " error(%v)", err)
				continue
			}
			fmt.Fprintf(w, " %#v", x)
		}
		w.WriteByte('\n')
	}
	all := `SELECT id, n, b, at, j, s, f FROM "dr_t" ORDER BY id`
	res, err := tr.GetAllTransaction(all, -1)
	rec("all", res, err)
	row, err := tr.GetOneRowTransaction(`SELECT n, s FROM "dr_t" WHERE id = ?`, int64(1)).String()
	rec("one", row, err)
	row, err = tr.GetOneRowTransaction(`SELECT n FROM "dr_t" WHERE id = ?`, int64(99)).String()
	rec("one-none", row, err)
	rec("update", tr.ExecSql(`UPDATE "dr_t" SET n = n + ? WHERE id = ?`, "5", int64(1)))
	rec("update-sub", tr.ExecSql(`UPDATE "dr_t" SET n = n - ? WHERE id = ?`, "3", int64(2)))
	rec("insert", tr.ExecSql(`INSERT INTO "dr_t" (id, n, b, at, j, s, f) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		int64(10), "12.50", `\x0001ff`, "2026-01-02 03:04:05.5", `{"b":1,"a":[1,"x"]}`, "é", "true"))
	rec("insert-null", tr.ExecSql(`INSERT INTO "dr_t" (id, n) VALUES (?, ?)`, int64(11), int64(0)))
	id, err := tr.MaxID("dr_t")
	rec("maxid", id, err)

	ins := NewStatement(`INSERT INTO "dr_t" (id, n, s) VALUES (?, ?, ?)`)
	rec("stmt-insert", tr.ExecStatement(ins, int64(12), "7", "s12"))
	rec("stmt-insert-again", tr.ExecStatement(ins, int64(13), "8", "s13"))
	one := NewStatement(`SELECT n, s, b, at, j, f FROM "dr_t" WHERE id = ?`)
	for _, id := range []int64{1, 10, 11, 12, 99} {
		row, err := tr.OneRowStatement(one, id).String()
		rec("stmt-one", id, row, err)
	}
	upd := NewStatement(`UPDATE "dr_t" SET s = ?, f = ? WHERE id = ?`)
	rec("stmt-update", tr.ExecStatement(upd, "u", "f", int64(12)))
	sel := NewStatement(`SELECT id, n, b, at, j, s, f FROM "dr_t" WHERE n >= $1 ORDER BY id`)
	for _, arg := range []any{"0", "7", int64(100)} {
		rows, err := tr.QueryStatement(sel, []any{arg})
		rec("stmt-query", arg, err)
		if err == nil {
			readRows(rows, rec)
		}
	}
	rows, err := tr.QueryText(all, nil)
	rec("text", err)
	if err == nil {
		readRows(rows, rec)
	}
	res, err = tr.GetAllTransaction(all, -1)
	rec("all-after", res, err)
}

// errorScript runs a statement PostgreSQL fails, as the first of a view: one after a write of the
// view runs it again
func errorScript(tr *DbTransaction, w *strings.Builder) {
	res, err := tr.GetAllTransaction(`SELECT * FROM "dr_missing" WHERE id = ?`, -1, int64(1))
	fmt.Fprintf(w, "missing: %#v error(%v)\n", res, err)
	row, err := tr.OneRowStatement(NewStatement(`SELECT n FROM "dr_t" WHERE id = ?`), int64(1)).String()
	fmt.Fprintf(w, "after: %#v error(%v)\n", row, err)
}

// readRows reads the rows as each way of reading them: by type, by bytes, and past their end
func readRows(rows *Rows, rec func(string, ...any)) {
	names, err := rows.Columns()
	rec("columns", names, err)
	types, err := rows.DatabaseTypeNames()
	rec("types", types, err)
	rec("scan-first", fmt.Sprint(rows.Scan(new(any))))
	n := 0
	for rows.Next() {
		if n%2 == 0 {
			dest := make([][]byte, len(names))
			err := rows.ScanBytes(dest)
			rec("bytes", dest, err)
			for _, hexBytea := range []bool{false, true} {
				texts, null := make([]string, len(names)), make([]bool, len(names))
				err := rows.ScanText(texts, null, hexBytea)
				rec("texts", hexBytea, texts, null, err)
			}
		} else {
			var id int64
			var num string
			var b []byte
			var at, j, s, f any
			err := rows.Scan(&id, &num, &b, &at, &j, &s, &f)
			if at, ok := at.(time.Time); ok {
				rec("at", at.String())
			}
			rec("scan", id, num, b, j, s, f, err)
			rec("scan-count", fmt.Sprint(rows.Scan(&id)))
			var bad int64
			rec("scan-bad", fmt.Sprint(rows.Scan(&id, &bad, &b, &at, &j, &s, &f)))
		}
		n++
	}
	rec("err", rows.Err())
	rec("close", rows.Close(), rows.Close())
	rec("next-closed", rows.Next())
	rec("scan-closed", fmt.Sprint(rows.Scan(new(any))))
	_, err = rows.Columns()
	rec("columns-closed", fmt.Sprint(err))
}

// TestDirectRunMatchesSQL (T2): the statements of a view run without database/sql return the
// rows, values, errors and counts they return through it, and leave the same digest of the view
func TestDirectRunMatchesSQL(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	ctx := context.Background()
	plain := testConn(t)
	for _, q := range []string{
		`DROP TABLE IF EXISTS dr_t`,
		`CREATE TABLE dr_t (id bigint PRIMARY KEY, n numeric NOT NULL DEFAULT 0 CHECK (n >= 0), b bytea,
			at timestamp, j jsonb, s text NOT NULL DEFAULT '', f bool)`,
		`INSERT INTO dr_t VALUES (1, 10, '\x00ff', '2020-01-02 03:04:05', '{"z": 1, "a": [true, null]}', 'one', true),
			(2, 3.25, '', '2021-06-07 08:09:10.123456', '[]', '', false), (3, 0, NULL, NULL, NULL, 'three', NULL)`,
	} {
		_, err := plain.ExecContext(ctx, q)
		require.NoError(t, err)
	}
	t.Cleanup(func() { plain.ExecContext(ctx, `DROP TABLE IF EXISTS dr_t`) })
	bumpSchema()

	db, err := openDB(dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)
	saved := DBConn
	DBConn = gdb
	t.Cleanup(func() { DBConn = saved })

	var viaSQL, traced atomic.Int64
	StatementTrace = func(string, []any) {
		traced.Add(1)
		if viaDatabaseSQL() {
			viaSQL.Add(1)
		}
	}
	t.Cleanup(func() { StatementTrace = nil })

	tx := gdb.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { tx.Rollback() })
	st, err := NewBlockState(NewDbTransaction(tx), []string{""})
	require.NoError(t, err)
	t.Cleanup(st.End)
	pconn, err := db.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pconn.Close() })

	// run is what the script returned in a new view, the counts of its statements and the digest of
	// the view; direct tells it runs as views run, else through database/sql
	run := func(ordered, direct bool, script func(*DbTransaction, *strings.Builder)) (string, TxStats, string) {
		var v *TxView
		if ordered {
			v = st.NewOrderedView(0, "")
		} else {
			v = st.NewParallelView(0, "", pconn)
		}
		vtx := v.Transaction()
		if ordered {
			require.NoError(t, vtx.Savepoint("run"))
		}
		tr := vtx
		if !direct {
			tr = &DbTransaction{conn: vtx.conn, statements: vtx.statements, cache: vtx.cache}
		}
		viaSQL.Store(0)
		traced.Store(0)
		var w strings.Builder
		script(tr, &w)
		if direct {
			require.Zero(t, viaSQL.Load(), "statements of the view ran through database/sql")
		} else {
			require.Equal(t, traced.Load(), viaSQL.Load(), "statements ran without database/sql")
		}
		require.NotZero(t, traced.Load())
		stats := vtx.Stats()
		stats.Time = 0
		digest := v.Digest()
		_, reason := v.NeedsOrder()
		require.False(t, v.Restart(), reason)
		if ordered {
			require.NoError(t, vtx.RollbackSavepoint("run"))
		}
		return w.String(), stats, digest
	}
	for _, ordered := range []bool{true, false} {
		for pass := range 3 {
			script := directScript
			if pass == 2 {
				script = errorScript
			}
			// the error of PostgreSQL ends the transaction of the block, past the savepoints of
			// the view
			pgRun := func(direct bool) (string, TxStats, string) {
				if pass == 2 && ordered {
					require.NoError(t, tx.Exec(`SAVEPOINT dr_err`).Error)
					defer func() { require.NoError(t, tx.Exec(`ROLLBACK TO SAVEPOINT dr_err`).Error) }()
				}
				return run(ordered, direct, script)
			}
			want, wantStats, wantDigest := pgRun(false)
			got, gotStats, gotDigest := pgRun(true)
			require.Equal(t, want, got, "ordered %v pass %d", ordered, pass)
			require.Equal(t, wantStats, gotStats, "ordered %v pass %d", ordered, pass)
			require.Equal(t, wantDigest, gotDigest, "ordered %v pass %d", ordered, pass)
			if pass == 0 {
				t.Logf("ordered %v: %d statements, %d rows read, %d written", ordered,
					gotStats.Statements, gotStats.Read, gotStats.Written)
			}
		}
	}
}

// FuzzBytesOf checks bytesOf against database/sql scanning the value into a *[]byte
func FuzzBytesOf(f *testing.F) {
	f.Add(uint8(0), "", int64(0), 0.0, false, int64(0))
	f.Add(uint8(1), "\x00\xff", int64(-1), math.Inf(1), true, int64(1e18))
	f.Add(uint8(5), "x", int64(math.MinInt64), math.NaN(), false, int64(-62135596800))
	f.Fuzz(func(t *testing.T, kind uint8, s string, i int64, fl float64, b bool, sec int64) {
		var v driver.Value
		switch kind % 8 {
		case 0:
			v = s
		case 1:
			v = []byte(s)
		case 2:
			v = []byte(nil)
		case 3:
			v = i
		case 4:
			v = fl
		case 5:
			v = b
		case 6:
			v = time.Unix(sec%(1<<40), i%1e9).In(time.FixedZone("z", int(i%50000)))
		case 7:
			v = nil
		}
		want := []byte("prior")
		werr := sql.ConvertAssign(driver.ScanContext{}, &want, v)
		for _, buf := range [][]byte{nil, make([]byte, 0, 4), []byte("junk")[:0]} {
			got, err := bytesOf(buf, v)
			require.Equal(t, werr, err)
			require.Equal(t, want == nil, got == nil, "%T %v", v, v)
			require.Equal(t, string(want), string(got))
		}
	})
}
