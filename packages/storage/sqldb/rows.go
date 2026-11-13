/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
)

var (
	errRowsClosed = errors.New("sql: Rows are closed")
	errNoRowsYet  = errors.New("sql: Scan called without calling Next")
)

// Rows are the rows of a select: those database/sql read, or those of a statement a view ran
// without it. Rows read without database/sql read and convert the values as database/sql does:
// Next, Scan, Err and Close tell what those of database/sql tell.
type Rows struct {
	std *sql.Rows
	drv driver.Rows
	// vals are the values of the row read; err the last error, io.EOF once the rows are read
	vals   []driver.Value
	err    error
	next   bool
	closed bool
}

// StdRows are the rows database/sql read
func StdRows(r *sql.Rows) *Rows { return &Rows{std: r} }

func driverRows(r driver.Rows) *Rows { return &Rows{drv: r} }

// Columns are the names of the columns
func (r *Rows) Columns() ([]string, error) {
	if r.std != nil {
		return r.std.Columns()
	}
	if r.closed {
		return nil, r.lastOr(errRowsClosed)
	}
	return r.drv.Columns(), nil
}

// DatabaseTypeNames are the names of the data types of the columns, as the column types of
// database/sql tell them
func (r *Rows) DatabaseTypeNames() ([]string, error) {
	if r.std != nil {
		types, err := r.std.ColumnTypes()
		if err != nil {
			return nil, err
		}
		names := make([]string, len(types))
		for i, t := range types {
			names[i] = t.DatabaseTypeName()
		}
		return names, nil
	}
	if r.closed {
		return nil, r.lastOr(errRowsClosed)
	}
	names := make([]string, len(r.drv.Columns()))
	if t, ok := r.drv.(driver.RowsColumnTypeDatabaseTypeName); ok {
		for i := range names {
			names[i] = t.ColumnTypeDatabaseTypeName(i)
		}
	}
	return names, nil
}

func (r *Rows) lastOr(err error) error {
	if r.err != nil && r.err != io.EOF {
		return r.err
	}
	return err
}

// Next reads the next row; false when there is none or reading failed, which closes the rows
func (r *Rows) Next() bool {
	if r.std != nil {
		return r.std.Next()
	}
	if r.closed {
		return false
	}
	if !r.next {
		r.next = true
		r.vals = make([]driver.Value, len(r.drv.Columns()))
	}
	r.err = r.drv.Next(r.vals)
	if r.err != nil {
		r.Close()
		return false
	}
	return true
}

// Scan reads the columns of the row into dest, as database/sql converts them
func (r *Rows) Scan(dest ...any) error {
	if r.std != nil {
		return r.std.Scan(dest...)
	}
	if err := r.scannable(len(dest)); err != nil {
		return err
	}
	for i, d := range dest {
		var err error
		if b, ok := d.(*[]byte); ok && b != nil {
			*b, err = bytesOf(nil, r.vals[i])
		} else {
			err = sql.ConvertAssign(driver.ScanContext{}, d, r.vals[i])
		}
		if err != nil {
			return r.scanError(i, err)
		}
	}
	return nil
}

// ScanBytes reads the columns of the row as Scan reads them into a *[]byte each, appending each
// to the bytes of dest it replaces; a NULL is nil
func (r *Rows) ScanBytes(dest [][]byte) error {
	if r.std != nil {
		args := make([]any, len(dest))
		for i := range dest {
			args[i] = &dest[i]
		}
		return r.std.Scan(args...)
	}
	if err := r.scannable(len(dest)); err != nil {
		return err
	}
	for i, v := range r.vals {
		b, err := bytesOf(dest[i][:0], v)
		if err != nil {
			return r.scanError(i, err)
		}
		dest[i] = b
	}
	return nil
}

func (r *Rows) scannable(n int) error {
	if r.err != nil && r.err != io.EOF {
		return r.err
	}
	if r.closed {
		return r.lastOr(errRowsClosed)
	}
	if !r.next {
		return errNoRowsYet
	}
	if n != len(r.vals) {
		return fmt.Errorf("sql: expected %d destination arguments in Scan, not %d", len(r.vals), n)
	}
	return nil
}

func (r *Rows) scanError(i int, err error) error {
	return fmt.Errorf(`sql: Scan error on column index %d, name %q: %w`, i, r.drv.Columns()[i], err)
}

// noBytes is the empty bytes that are not NULL
var noBytes = []byte{}

// bytesOf is the value database/sql scans into a *[]byte, appended to buf; nil for NULL
func bytesOf(buf []byte, v driver.Value) ([]byte, error) {
	if buf == nil {
		buf = noBytes[:0:0]
	}
	switch s := v.(type) {
	case string:
		return append(buf, s...), nil
	case []byte:
		if s == nil {
			return nil, nil
		}
		return append(buf, s...), nil
	case time.Time:
		return s.AppendFormat(buf, time.RFC3339Nano), nil
	case nil:
		return nil, nil
	case int64:
		return strconv.AppendInt(buf, s, 10), nil
	case float64:
		return strconv.AppendFloat(buf, s, 'g', -1, 64), nil
	case bool:
		return strconv.AppendBool(buf, s), nil
	}
	var b []byte
	err := sql.ConvertAssign(driver.ScanContext{}, &b, v)
	return b, err
}

// Err is the error that ended the reading of the rows, nil when they were all read
func (r *Rows) Err() error {
	if r.std != nil {
		return r.std.Err()
	}
	return r.lastOr(nil)
}

// Close closes the rows; the error of the first close
func (r *Rows) Close() error {
	if r.std != nil {
		return r.std.Close()
	}
	if r.closed {
		return nil
	}
	r.closed = true
	err := r.drv.Close()
	r.err = r.lastOr(err)
	return err
}
