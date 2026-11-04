/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package dataquery

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// countDB points the reads at a schema of their own on the database IBAX_TEST_DSN names, holding
// 1_tables with rows definitions of ecosystem 1, and drops it after the test. Its statements are
// not prepared, as a test changes the columns of 1_tables.
func countDB(t *testing.T, rows int) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	schema := fmt.Sprintf("dq_count_%d", time.Now().UnixNano())
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, admin.Exec(`CREATE SCHEMA `+schema).Error)
	conn, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn + " search_path=" + schema, PreferSimpleProtocol: true}),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	saved := sqldb.DBConn
	sqldb.DBConn = conn
	t.Cleanup(func() {
		sqldb.DBConn = saved
		if db, err := conn.DB(); err == nil {
			db.Close()
		}
		admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		if db, err := admin.DB(); err == nil {
			db.Close()
		}
	})
	require.NoError(t, conn.Exec(`CREATE TABLE "1_tables" (id bigint PRIMARY KEY, name varchar(100) NOT NULL,
		permissions jsonb NOT NULL, columns text NOT NULL, conditions text NOT NULL, app_id bigint NOT NULL,
		ecosystem bigint NOT NULL)`).Error)
	require.NoError(t, conn.Exec(`INSERT INTO "1_tables" SELECT i, 'table' || i, '{}', '{}', 'true', 1, 1
		FROM generate_series(1, ?) i`, rows).Error)
}

// A query counts the rows its filter matches unless it says not to; one that does not count
// reads the page alone, and tells whether rows follow it
func TestQueryCount(t *testing.T) {
	countDB(t, 5)
	table, err := Open(Reader{KeyID: 42, Ecosystem: 1}, "tables", 1)
	require.NoError(t, err)
	limit := func(n int) *int { return &n }
	no, yes := false, true

	for _, count := range []*bool{nil, &yes} {
		result, err := table.Query(Query{Limit: limit(2), Count: count})
		require.NoError(t, err)
		require.NotNil(t, result.Count)
		require.Equal(t, int64(5), *result.Count)
		require.False(t, result.More)
		require.Len(t, result.List, 2)
	}

	for _, page := range []struct {
		offset int
		ids    []string
		more   bool
	}{
		{0, []string{"1", "2"}, true},
		{2, []string{"3", "4"}, true},
		{3, []string{"4", "5"}, false},
		{4, []string{"5"}, false},
		{5, nil, false},
	} {
		result, err := table.Query(Query{Limit: limit(2), Offset: page.offset, Count: &no})
		require.NoError(t, err)
		require.Nil(t, result.Count, "offset %d", page.offset)
		require.Equal(t, page.more, result.More, "offset %d", page.offset)
		var ids []string
		for _, row := range result.List {
			ids = append(ids, fmt.Sprint(row["id"]))
		}
		require.Equal(t, page.ids, ids, "offset %d", page.offset)
	}
}

// A table is described once until a block commits: a column added outside a block is not seen
// until one does
func TestDescribeOnceABlock(t *testing.T) {
	countDB(t, 1)
	columns := func() []string {
		table, err := Describe(Reader{KeyID: 42, Ecosystem: 1}, "tables", 1)
		require.NoError(t, err)
		return table.ordered
	}
	before := columns()
	require.NotContains(t, before, "extra")
	require.NoError(t, sqldb.DBConn.Exec(`ALTER TABLE "1_tables" ADD COLUMN extra text`).Error)
	require.Equal(t, before, columns())

	tx, err := sqldb.StartTransaction()
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.Contains(t, columns(), "extra")
}
