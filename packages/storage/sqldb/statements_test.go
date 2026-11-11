/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql/driver"
	"io"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// plainRows are rows that tell nothing of their columns
type plainRows struct{ left int }

func (r *plainRows) Columns() []string { return []string{"a"} }
func (r *plainRows) Close() error      { return nil }
func (r *plainRows) Next(dest []driver.Value) error {
	if r.left == 0 {
		return io.EOF
	}
	r.left--
	dest[0] = int64(r.left)
	return nil
}

// scanRows are plainRows read by NextRow and ScanColumn
type scanRows struct {
	plainRows
	v int64
}

func (r *scanRows) NextRow() error {
	dest := make([]driver.Value, 1)
	err := r.Next(dest)
	if err == nil {
		r.v = dest[0].(int64)
	}
	return err
}

func (r *scanRows) ScanColumn(_ driver.ScanContext, _ int, dest any) error {
	*dest.(*int64) = r.v
	return nil
}

// TestTimedScanRows: rows read by NextRow and ScanColumn stay such rows timed, and count the rows
// read
func TestTimedScanRows(t *testing.T) {
	n := new(txStats)
	rows, err := timeQuery(n, time.Now(), &scanRows{plainRows: plainRows{left: 3}}, nil)
	require.NoError(t, err)
	s, ok := rows.(driver.RowsColumnScanner)
	require.True(t, ok)
	var sum, v int64
	for s.NextRow() == nil {
		require.NoError(t, s.ScanColumn(driver.ScanContext{}, 0, &v))
		sum += v
	}
	require.EqualValues(t, 3, sum)
	require.EqualValues(t, 3, n.read.Load())
}

// TestTimedRows: timed rows count the rows read, and of rows that tell nothing of their columns
// tell what database/sql takes of them
func TestTimedRows(t *testing.T) {
	n := new(txStats)
	rows, err := timeQuery(n, time.Now(), &plainRows{left: 2}, nil)
	require.NoError(t, err)
	tr := rows.(*timedRows)
	require.Equal(t, reflect.TypeFor[any](), tr.ColumnTypeScanType(0))
	require.Equal(t, "", tr.ColumnTypeDatabaseTypeName(0))
	_, ok := tr.ColumnTypeLength(0)
	require.False(t, ok)
	_, ok = tr.ColumnTypeNullable(0)
	require.False(t, ok)
	_, _, ok = tr.ColumnTypePrecisionScale(0)
	require.False(t, ok)
	dest := make([]driver.Value, 1)
	for tr.Next(dest) == nil {
	}
	require.EqualValues(t, 2, n.read.Load())
	require.Positive(t, n.nanos.Load())
}

// TestTxStats: the statements of a transaction count in its stats with the rows they read and
// wrote, by gorm and by the statements run without it
func TestTxStats(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	db, err := openDB(dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, registerStatementCount(gdb))

	ctx, n := withStatementCount()
	tr := &DbTransaction{conn: gdb.WithContext(ctx).Begin(), statements: n}
	require.NoError(t, tr.conn.Error)
	defer tr.Rollback()
	require.NoError(t, tr.conn.Exec(`CREATE TEMP TABLE stats_test (id bigint, v text)`).Error)
	require.NoError(t, tr.conn.Exec(`INSERT INTO stats_test VALUES (1, 'a'), (2, 'b'), (3, 'c')`).Error)
	before := tr.Stats()
	require.EqualValues(t, 2, before.Statements)
	require.EqualValues(t, 3, before.Written)

	rows, err := tr.queryRaw(`SELECT id FROM stats_test WHERE id > ?`, int64(0))
	require.NoError(t, err)
	got := 0
	for rows.Next() {
		got++
	}
	require.NoError(t, rows.Close())
	require.Equal(t, 3, got)
	var found []struct{ ID int64 }
	require.NoError(t, tr.conn.Raw(`SELECT id FROM stats_test WHERE id < 3`).Scan(&found).Error)
	require.Len(t, found, 2)
	require.NoError(t, tr.conn.Exec(`UPDATE stats_test SET v = 'x' WHERE id >= 2`).Error)

	d := tr.Stats().Since(before)
	require.EqualValues(t, 3, d.Statements)
	require.EqualValues(t, 5, d.Read)
	require.EqualValues(t, 2, d.Written)
	require.Positive(t, d.Time)
	require.Equal(t, tr.Stats().Statements, tr.Statements())
}
