/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestSelectSQL: a select is, text and values, the statement gorm makes of the same table,
// columns, condition, group, order, offset and limit
func TestSelectSQL(t *testing.T) {
	pool, err := sql.Open("pgx", "host=127.0.0.1 port=1")
	require.NoError(t, err)
	defer pool.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)
	if DBConn == nil {
		DBConn = db
		defer func() { DBConn = nil }()
	}
	cases := []Select{
		{Table: "1_keys", Columns: `"id","amount"`, Where: `id = ?`, Args: []any{"5"}, Order: "id", Limit: 1},
		{Table: "1_keys", Columns: "*", Limit: 25},
		{Table: "1_keys", Columns: "*", Limit: 25, Offset: 10},
		{Table: "1_keys", Columns: "*", Limit: -1},
		{Table: "1_keys", Columns: "*", Limit: -1, Offset: 3},
		{Table: "1_keys", Columns: "*", Limit: 25, Offset: -2},
		{Table: "2_my table", Columns: "*", Limit: 25},
		{Table: "ibax.1_keys", Columns: "*", Limit: 25},
		{Table: "1_history", Columns: `sum(amount)`, Where: `("sender_id" = ? or "recipient_id" = ?) and "ecosystem" in (?,?,?)`,
			Args: []any{"1", "2", "1", "2", "3"}, Group: "ecosystem", Order: "ecosystem desc", Limit: 100},
		{Table: "1_contracts", Columns: `"value",doc::jsonb->>'title' as "doc.title"`, Where: `"name" like ? and x = '?'`,
			Args: []any{"%a%"}, Order: `id desc, name`, Limit: 10, Offset: 5},
		{Table: "1_keys", Columns: "*", Where: `a = ? and b = ?`, Args: []any{"1"}, Limit: 25},
		{Table: "1_keys", Columns: "*", Where: `a = 1`, Limit: 25},
		{Table: "1_keys", Columns: "*", Group: "a, b", Limit: 25},
		{Table: "1_keys", Columns: "*", Group: "t.a", Limit: 25},
		{Table: "1_keys", Columns: "*", Group: "lower(name)", Limit: 25},
	}
	for _, c := range cases {
		q := db.Table(c.Table).Select(c.Columns).Where(c.Where, c.Args...)
		if c.Group != "" {
			q = q.Group(c.Group)
		}
		q = q.Order(c.Order)
		if c.Limit >= 0 || c.Offset != 0 {
			q = q.Offset(c.Offset).Limit(c.Limit)
		}
		stmt := q.Find(&[]map[string]any{}).Statement
		require.Equal(t, stmt.SQL.String(), c.SQL(), "%+v", c)
		want := stmt.Vars
		if len(want) == 0 {
			want = nil
		}
		got := c.Args
		if len(got) == 0 {
			got = nil
		}
		require.Equal(t, want, got, "%+v", c)
	}
}
