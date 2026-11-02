/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql/driver"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxCleanReads bounds the reads of PostgreSQL a block keeps for its transactions
const maxCleanReads = 4096

// cleanRead is the answer of PostgreSQL to a select of tables the block has not changed. The
// tables stay as they were when the block began until the block changes them, and a view only
// takes the answer while they are, so every transaction of the block shares it.
type cleanRead struct {
	names     []string
	typeNames []string
	lengths   []colLength
	scales    []colScale
	scanTypes []reflect.Type
	nullable  []colNullable
	rows      [][]driver.Value
}

type colLength struct {
	n  int64
	ok bool
}

type colScale struct {
	precision, scale int64
	ok               bool
}

type colNullable struct {
	nullable, ok bool
}

// readKey is the key of a select with its arguments
func readKey(query string, args []driver.NamedValue) string {
	var b strings.Builder
	b.WriteString(query)
	for _, a := range args {
		b.WriteByte(0)
		b.WriteString(a.Name)
		switch x := a.Value.(type) {
		case []byte:
			fmt.Fprintf(&b, "%d:[]byte:%x", a.Ordinal, x)
		case time.Time:
			fmt.Fprintf(&b, "%d:time:%s", a.Ordinal, x.Format(time.RFC3339Nano))
		default:
			fmt.Fprintf(&b, "%d:%T:%v", a.Ordinal, x, x)
		}
	}
	return b.String()
}

// captureRows reads the rows of PostgreSQL to the end
func captureRows(rows driver.Rows) (*cleanRead, error) {
	defer rows.Close()
	names := rows.Columns()
	r := &cleanRead{names: names}
	n := len(names)
	r.typeNames = make([]string, n)
	r.lengths = make([]colLength, n)
	r.scales = make([]colScale, n)
	r.scanTypes = make([]reflect.Type, n)
	r.nullable = make([]colNullable, n)
	for i := range n {
		if t, ok := rows.(driver.RowsColumnTypeDatabaseTypeName); ok {
			r.typeNames[i] = t.ColumnTypeDatabaseTypeName(i)
		}
		if t, ok := rows.(driver.RowsColumnTypeLength); ok {
			r.lengths[i].n, r.lengths[i].ok = t.ColumnTypeLength(i)
		}
		if t, ok := rows.(driver.RowsColumnTypePrecisionScale); ok {
			r.scales[i].precision, r.scales[i].scale, r.scales[i].ok = t.ColumnTypePrecisionScale(i)
		}
		if t, ok := rows.(driver.RowsColumnTypeScanType); ok {
			r.scanTypes[i] = t.ColumnTypeScanType(i)
		} else {
			r.scanTypes[i] = reflect.TypeFor[any]()
		}
		if t, ok := rows.(driver.RowsColumnTypeNullable); ok {
			r.nullable[i].nullable, r.nullable[i].ok = t.ColumnTypeNullable(i)
		}
	}
	for {
		dest := make([]driver.Value, n)
		err := rows.Next(dest)
		if err == io.EOF {
			return r, nil
		}
		if err != nil {
			return nil, err
		}
		for i, v := range dest {
			if b, ok := v.([]byte); ok {
				dest[i] = append([]byte(nil), b...)
			}
		}
		r.rows = append(r.rows, dest)
	}
}

// cleanRows replays the rows of a clean read
type cleanRows struct {
	r *cleanRead
	i int
}

var (
	_ driver.Rows                           = (*cleanRows)(nil)
	_ driver.RowsColumnTypeDatabaseTypeName = (*cleanRows)(nil)
	_ driver.RowsColumnTypeLength           = (*cleanRows)(nil)
	_ driver.RowsColumnTypePrecisionScale   = (*cleanRows)(nil)
	_ driver.RowsColumnTypeScanType         = (*cleanRows)(nil)
	_ driver.RowsColumnTypeNullable         = (*cleanRows)(nil)
)

func (c *cleanRows) Columns() []string { return c.r.names }

func (c *cleanRows) Close() error { return nil }

func (c *cleanRows) Next(dest []driver.Value) error {
	if c.i >= len(c.r.rows) {
		return io.EOF
	}
	for j, v := range c.r.rows[c.i] {
		if b, ok := v.([]byte); ok {
			v = append([]byte(nil), b...)
		}
		dest[j] = v
	}
	c.i++
	return nil
}

func (c *cleanRows) ColumnTypeDatabaseTypeName(i int) string { return c.r.typeNames[i] }

func (c *cleanRows) ColumnTypeLength(i int) (int64, bool) {
	return c.r.lengths[i].n, c.r.lengths[i].ok
}

func (c *cleanRows) ColumnTypePrecisionScale(i int) (int64, int64, bool) {
	s := c.r.scales[i]
	return s.precision, s.scale, s.ok
}

func (c *cleanRows) ColumnTypeScanType(i int) reflect.Type { return c.r.scanTypes[i] }

func (c *cleanRows) ColumnTypeNullable(i int) (bool, bool) {
	return c.r.nullable[i].nullable, c.r.nullable[i].ok
}

// cleanRead is the answer the block keeps to the select
func (st *BlockState) cleanRead(key string) (*cleanRead, bool) {
	v, ok := st.cleanReads.Load(key)
	if !ok {
		return nil, false
	}
	return v.(*cleanRead), true
}

// keepCleanRead keeps the answer to the select for the transactions of the block
func (st *BlockState) keepCleanRead(key string, r *cleanRead) {
	if st.cleanCount.Load() >= maxCleanReads {
		return
	}
	if _, loaded := st.cleanReads.LoadOrStore(key, r); !loaded {
		st.cleanCount.Add(1)
	}
}

// cleanTable is the state of the table while the block and the view have not changed it, with
// the seq of the state
func (v *TxView) cleanTable(table string) (*tableState, uint64, bool) {
	x, ok := v.st.known.Load(table)
	if !ok || v.ownChanged(table) {
		return nil, 0, false
	}
	ts := x.(*tableState)
	st := v.st
	seq := st.pub.Load()
	if st.allPG.Load() || ts.pg.Load() || ts.changed() {
		return nil, 0, false
	}
	return ts, seq, true
}

// shared is the value of the key the transactions of the block decoded of the rows of the
// table, while the block has not changed the table: the view reads the table
func (v *TxView) shared(table, key string) (any, bool) {
	ts, seq, ok := v.cleanTable(table)
	if !ok {
		return nil, false
	}
	val, ok := v.st.shared.Load(table + "\x00" + key)
	if !ok {
		return nil, false
	}
	v.read(ts, seq, nil)
	return val, true
}

// share keeps for the transactions of the block the value of the key the view decoded of the
// rows of the table. The block changes a table for good, so a table it has not changed now is
// the table as it was when the block began, and so was it when the view read the value.
func (v *TxView) share(table, key string, val any) {
	if v.st.sharedCount.Load() >= maxCleanReads {
		return
	}
	if _, _, ok := v.cleanTable(table); !ok {
		return
	}
	if _, loaded := v.st.shared.LoadOrStore(table+"\x00"+key, val); !loaded {
		v.st.sharedCount.Add(1)
	}
}

// baseRows are the rows of PostgreSQL the block read of a table by a key of the primary key or
// of a unique index, nil for a key no row has. PostgreSQL keeps the rows as they were when the
// block began while the table is in the state, and a row the state changed is in its rows, so
// the transactions of the block share what one of them read.
type baseRows struct {
	mu   sync.RWMutex
	rows map[string]stateRow
	// scans are the rows of the statements the block read of the table, by stmtKey, and n the
	// rows they keep
	scans map[string][]stateRow
	n     int
}

// maxScanRows bounds the rows of the statements a block keeps of a table
const maxScanRows = 1 << 16

// stmtKey is the key of a statement and its values, false for a value it does not tell apart:
// equal keys send PostgreSQL the same statement with the same values
func stmtKey(query string, values []any) (string, bool) {
	var b strings.Builder
	b.Grow(len(query) + 16*len(values))
	b.WriteString(query)
	for _, v := range values {
		b.WriteByte(0)
		switch x := v.(type) {
		case nil:
			b.WriteByte('n')
		case int64:
			b.WriteByte('i')
			b.WriteString(strconv.FormatInt(x, 10))
		case bool:
			b.WriteByte('b')
			b.WriteString(strconv.FormatBool(x))
		case string:
			b.WriteByte('s')
			b.WriteString(strconv.Itoa(len(x)))
			b.WriteByte(':')
			b.WriteString(x)
		case []byte:
			if x == nil {
				b.WriteByte('n')
				break
			}
			b.WriteByte('x')
			b.WriteString(strconv.Itoa(len(x)))
			b.WriteByte(':')
			b.Write(x)
		default:
			return "", false
		}
	}
	return b.String(), true
}

// scan is the rows of PostgreSQL of a statement the block read, and whether it read it
func (b *baseRows) scan(key string) ([]stateRow, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	r, ok := b.scans[key]
	return r, ok
}

func (b *baseRows) putScan(key string, rows []stateRow) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.scans[key]; ok || b.n+len(rows)+1 > maxScanRows {
		return
	}
	if b.scans == nil {
		b.scans = make(map[string][]stateRow)
	}
	b.scans[key] = rows
	b.n += len(rows) + 1
}

func baseKey(j int, key string) string { return strconv.Itoa(j) + "\x00" + key }

// get is the row of PostgreSQL of the key of the primary key (j -1) or of the unique index j,
// and whether the block read it
func (b *baseRows) get(j int, key string) (stateRow, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	r, ok := b.rows[baseKey(j, key)]
	return r, ok
}

func (b *baseRows) put(j int, key string, r stateRow) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.rows == nil {
		b.rows = make(map[string]stateRow)
	}
	b.rows[baseKey(j, key)] = r
}
