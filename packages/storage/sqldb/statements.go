/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql/driver"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/metrics"

	"gorm.io/gorm"
)

// statementsKey keys, in the context of a database transaction, what its statements did
type statementsKey struct{}

// txStats is what the statements of a database transaction did
type txStats struct {
	statements atomic.Int64
	// nanos is the time the statements took: their run, and the reading of the rows they returned
	nanos   atomic.Int64
	read    atomic.Int64 // rows the statements returned
	written atomic.Int64 // rows the statements wrote
}

// TxStats is what the statements of a database transaction did
type TxStats struct {
	Statements int64
	// Time is the time the statements took: their run, and the reading of the rows they returned
	Time    time.Duration
	Read    int64 // rows the statements returned
	Written int64 // rows the statements wrote
}

func statsOf(ctx context.Context) *txStats {
	if ctx == nil {
		return nil
	}
	n, _ := ctx.Value(statementsKey{}).(*txStats)
	return n
}

// registerStatementCount counts each statement gorm sends: in the metric of its operation, and in
// the count of the database transaction it runs in, if StartTransaction began it
func registerStatementCount(db *gorm.DB) error {
	count := func(op string) func(*gorm.DB) {
		c := metrics.DBStatements.WithLabelValues(op)
		return func(db *gorm.DB) {
			c.Inc()
			if n := statsOf(db.Statement.Context); n != nil {
				n.statements.Add(1)
			}
		}
	}
	cb := db.Callback()
	for _, err := range []error{
		cb.Create().After("gorm:create").Register("ibax:count_create", count("create")),
		cb.Query().After("gorm:query").Register("ibax:count_query", count("query")),
		cb.Update().After("gorm:update").Register("ibax:count_update", count("update")),
		cb.Delete().After("gorm:delete").Register("ibax:count_delete", count("delete")),
		cb.Row().After("gorm:row").Register("ibax:count_row", count("row")),
		cb.Raw().After("gorm:raw").Register("ibax:count_raw", count("raw")),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

// withStatementCount is a context keeping what the statements of a database transaction did
func withStatementCount() (context.Context, *txStats) {
	n := new(txStats)
	return context.WithValue(context.Background(), statementsKey{}, n), n
}

// Statements is the number of statements the transaction has sent; 0 for a transaction
// StartTransaction or a view did not begin
func (tr *DbTransaction) Statements() int64 {
	if tr == nil || tr.statements == nil {
		return 0
	}
	return tr.statements.statements.Load()
}

// Stats is what the statements the transaction has sent did; zero for a transaction
// StartTransaction or a view did not begin
func (tr *DbTransaction) Stats() TxStats {
	if tr == nil || tr.statements == nil {
		return TxStats{}
	}
	n := tr.statements
	return TxStats{Statements: n.statements.Load(), Time: time.Duration(n.nanos.Load()),
		Read: n.read.Load(), Written: n.written.Load()}
}

// Since is what the statements did after before
func (s TxStats) Since(before TxStats) TxStats {
	return TxStats{Statements: s.Statements - before.Statements, Time: s.Time - before.Time,
		Read: s.Read - before.Read, Written: s.Written - before.Written}
}

// timeQuery keeps in n the time of a query that began at start, and the time and the count of the
// rows it returns as they are read
func timeQuery(n *txStats, start time.Time, rows driver.Rows, err error) (driver.Rows, error) {
	n.nanos.Add(int64(time.Since(start)))
	if err != nil {
		return rows, err
	}
	if _, ok := rows.(driver.RowsNextResultSet); ok {
		// rows of more than one result set are read only through their own type
		return rows, nil
	}
	t := &timedRows{Rows: rows, n: n}
	if s, ok := rows.(driver.RowsColumnScanner); ok {
		// database/sql reads them by NextRow and ScanColumn, not by Next
		return &timedScanRows{timedRows: t, s: s}, nil
	}
	return t, nil
}

// timeExec keeps in n the time of a statement that began at start, and the rows it wrote
func timeExec(n *txStats, start time.Time, res driver.Result, err error) (driver.Result, error) {
	n.nanos.Add(int64(time.Since(start)))
	if err == nil && res != nil {
		if w, errN := res.RowsAffected(); errN == nil {
			n.written.Add(w)
		}
	}
	return res, err
}

// timedRows are rows whose reading counts in what the statements of a transaction did. They tell
// of their columns what the rows they wrap tell, and what database/sql takes of rows that tell
// nothing otherwise.
type timedRows struct {
	driver.Rows
	n *txStats
}

func (r *timedRows) Next(dest []driver.Value) error {
	start := time.Now()
	err := r.Rows.Next(dest)
	r.n.nanos.Add(int64(time.Since(start)))
	if err == nil {
		r.n.read.Add(1)
	}
	return err
}

// timedScanRows are timedRows database/sql reads by NextRow and ScanColumn
type timedScanRows struct {
	*timedRows
	s driver.RowsColumnScanner
}

func (r *timedScanRows) NextRow() error {
	start := time.Now()
	err := r.s.NextRow()
	r.n.nanos.Add(int64(time.Since(start)))
	if err == nil {
		r.n.read.Add(1)
	}
	return err
}

func (r *timedScanRows) ScanColumn(ctx driver.ScanContext, index int, dest any) error {
	start := time.Now()
	err := r.s.ScanColumn(ctx, index, dest)
	r.n.nanos.Add(int64(time.Since(start)))
	return err
}

func (r *timedRows) ColumnTypeScanType(index int) reflect.Type {
	if c, ok := r.Rows.(driver.RowsColumnTypeScanType); ok {
		return c.ColumnTypeScanType(index)
	}
	return reflect.TypeFor[any]()
}

func (r *timedRows) ColumnTypeDatabaseTypeName(index int) string {
	if c, ok := r.Rows.(driver.RowsColumnTypeDatabaseTypeName); ok {
		return c.ColumnTypeDatabaseTypeName(index)
	}
	return ""
}

func (r *timedRows) ColumnTypeLength(index int) (int64, bool) {
	if c, ok := r.Rows.(driver.RowsColumnTypeLength); ok {
		return c.ColumnTypeLength(index)
	}
	return 0, false
}

func (r *timedRows) ColumnTypeNullable(index int) (nullable, ok bool) {
	if c, ok := r.Rows.(driver.RowsColumnTypeNullable); ok {
		return c.ColumnTypeNullable(index)
	}
	return false, false
}

func (r *timedRows) ColumnTypePrecisionScale(index int) (precision, scale int64, ok bool) {
	if c, ok := r.Rows.(driver.RowsColumnTypePrecisionScale); ok {
		return c.ColumnTypePrecisionScale(index)
	}
	return 0, 0, false
}
