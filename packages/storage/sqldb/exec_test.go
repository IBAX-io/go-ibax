/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TestBindPlain: the text of a statement bindPlain takes is, text and values, the statement gorm's
// Exec makes of it; the statements it does not take are those gorm makes another text of
func TestBindPlain(t *testing.T) {
	pool, err := sql.Open("pgx", "host=127.0.0.1 port=1")
	require.NoError(t, err)
	defer pool.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)
	now := time.Unix(1700000000, 0)
	cases := []struct {
		sql   string
		args  []any
		plain bool
	}{
		{`UPDATE "1_keys" SET amount = amount + ?::numeric WHERE id = ?`, []any{decimal.New(5, 0), int64(7)}, true},
		{`INSERT INTO "1_history" (id, sender_id, amount, comment, created_at, value_detail) VALUES (?, ?, ?, ?, to_timestamp(?), ?)`,
			[]any{"1", int64(2), decimal.RequireFromString("1.5"), "x?y", int64(1700000000), []byte(`{"a":1}`)}, true},
		{`INSERT INTO t VALUES (?, ?)`, []any{int64(1), []byte("ab")}, true},
		{`INSERT INTO t VALUES (?)`, []any{[]byte("ab")}, false},
		{`INSERT INTO t VALUES ( ?)`, []any{[]byte("ab")}, true},
		{`UPDATE t SET a = ?, b = ?, c = ?, d = ?, e = ?, f = ?`, []any{nil, true, 1.5, uint64(3), now, int32(4)}, true},
		{`UPDATE t SET a = ? WHERE b = ?`, []any{"1"}, true},
		{`UPDATE t SET a = 1`, nil, true},
		{`UPDATE t SET a = ?`, []any{"1", "2"}, true},
		{`DELETE FROM t WHERE id IN (?)`, []any{[]int64{1, 2}}, false},
		{`DELETE FROM t WHERE id IN ?`, []any{[]any{1, 2}}, false},
		{`UPDATE t SET a = @a`, []any{sql.Named("a", 1)}, false},
		{`UPDATE t SET a = ?`, []any{clause.Expr{SQL: "now()"}}, false},
		{`UPDATE t SET a = ?`, []any{[]string{"x"}}, false},
		{`ALTER TABLE "t" ADD COLUMN "c" text`, nil, true},
	}
	for _, c := range cases {
		text, ok := bindPlain(c.sql, c.args)
		require.Equal(t, c.plain, ok, c.sql)
		if !ok {
			continue
		}
		stmt := db.Exec(c.sql, c.args...).Statement
		require.Equal(t, stmt.SQL.String(), text, c.sql)
		want := stmt.Vars
		if len(want) == 0 {
			want = nil
		}
		got := c.args
		if len(got) == 0 {
			got = nil
		}
		require.Equal(t, want, got, c.sql)
	}
}
