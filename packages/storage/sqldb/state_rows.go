/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql/driver"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// memRows are the rows of a select the state of a block answers, as the pgx driver of
// database/sql returns them
type memRows struct {
	names []string
	types []colType
	mods  []int32
	rows  [][]*string
	i     int
	m     *pgtype.Map
}

var typeMaps = sync.Pool{New: func() any { return pgtype.NewMap() }}

var (
	_ driver.Rows                           = (*memRows)(nil)
	_ driver.RowsColumnTypeDatabaseTypeName = (*memRows)(nil)
	_ driver.RowsColumnTypeLength           = (*memRows)(nil)
	_ driver.RowsColumnTypePrecisionScale   = (*memRows)(nil)
	_ driver.RowsColumnTypeScanType         = (*memRows)(nil)
)

func (r *memRows) Columns() []string { return r.names }

func (r *memRows) Close() error {
	if r.m != nil {
		typeMaps.Put(r.m)
		r.m = nil
	}
	return nil
}

func (r *memRows) Next(dest []driver.Value) error {
	if r.i >= len(r.rows) {
		return io.EOF
	}
	if r.m == nil {
		r.m = typeMaps.Get().(*pgtype.Map)
	}
	row := r.rows[r.i]
	r.i++
	for j, v := range row {
		if v == nil {
			dest[j] = nil
			continue
		}
		d, err := decodeText(r.m, r.types[j].oid, *v)
		if err != nil {
			return err
		}
		dest[j] = d
	}
	return nil
}

func (r *memRows) ColumnTypeDatabaseTypeName(index int) string {
	return strings.ToUpper(r.types[index].name)
}

func (r *memRows) ColumnTypeLength(index int) (int64, bool) {
	switch r.types[index].oid {
	case pgtype.TextOID, pgtype.ByteaOID:
		return math.MaxInt64, true
	case pgtype.VarcharOID, pgtype.BPCharOID:
		return int64(r.mods[index] - 4), true
	}
	return 0, false
}

func (r *memRows) ColumnTypePrecisionScale(index int) (precision, scale int64, ok bool) {
	if r.types[index].oid != pgtype.NumericOID {
		return 0, 0, false
	}
	mod := r.mods[index] - 4
	return int64((mod >> 16) & 0xffff), int64(mod & 0xffff), true
}

func (r *memRows) ColumnTypeScanType(index int) reflect.Type {
	switch r.types[index].oid {
	case pgtype.Int8OID:
		return reflect.TypeFor[int64]()
	case pgtype.Int4OID:
		return reflect.TypeFor[int32]()
	case pgtype.Int2OID:
		return reflect.TypeFor[int16]()
	case pgtype.BoolOID:
		return reflect.TypeFor[bool]()
	case pgtype.NumericOID:
		return reflect.TypeFor[float64]()
	case pgtype.TimestampOID, pgtype.TimestamptzOID:
		return reflect.TypeFor[time.Time]()
	case pgtype.ByteaOID:
		return reflect.TypeFor[[]byte]()
	}
	return reflect.TypeFor[string]()
}

// project is the rows of a select of the table with the columns of the plan
func project(m *tableMeta, st *stmt, rows []stateRow) *memRows {
	h := st.head.Load()
	if h == nil || h.meta != m {
		h = newProjHead(m, st)
		st.head.Store(h)
	}
	out := &memRows{names: h.names, types: h.types, mods: h.mods}
	out.rows = make([][]*string, len(rows))
	cells := make([]*string, len(rows)*len(h.idx))
	for k, r := range rows {
		p := cells[k*len(h.idx) : (k+1)*len(h.idx) : (k+1)*len(h.idx)]
		for j, i := range h.idx {
			p[j] = r[i]
		}
		out.rows[k] = p
	}
	return out
}

// projHead is the projection of a select on a table: the columns it takes and their names and
// types. The rows of the select share it.
type projHead struct {
	meta  *tableMeta
	idx   []int
	names []string
	types []colType
	mods  []int32
}

func newProjHead(m *tableMeta, st *stmt) *projHead {
	h := &projHead{meta: m}
	if st.star {
		for i, c := range m.cols {
			h.idx = append(h.idx, i)
			h.names = append(h.names, c.name)
		}
	} else {
		for _, p := range st.proj {
			h.idx = append(h.idx, m.colIndex(p.col))
			h.names = append(h.names, p.as)
		}
	}
	for _, i := range h.idx {
		h.types = append(h.types, m.cols[i].typ)
		h.mods = append(h.mods, m.cols[i].typmod)
	}
	return h
}

// emptyRows are the rows of a statement that returns none
func emptyRows() *memRows { return &memRows{} }

// fetchRows runs a select of whole rows of a table in PostgreSQL on the connection, and reads
// them as text. A value of params is text, or binary for a []byte.
func fetchRows(ctx context.Context, c *stateConn, query string, params []any) ([]stateRow, error) {
	values := make([][]byte, len(params))
	formats := make([]int16, len(params))
	for i, p := range params {
		switch x := p.(type) {
		case nil:
		case []byte:
			values[i], formats[i] = x, 1
		case string:
			values[i] = []byte(x)
		case *string:
			if x != nil {
				values[i] = []byte(*x)
			}
		default:
			s, binary, ok := argText(x)
			if !ok {
				return nil, errUnsupported
			}
			values[i] = []byte(s)
			if binary {
				formats[i] = 1
			}
		}
	}
	res := c.pg.Conn().PgConn().ExecParams(ctx, query, values, nil, formats, nil)
	var rows []stateRow
	for res.NextRow() {
		raw := res.Values()
		r := make(stateRow, len(raw))
		for i, v := range raw {
			if v != nil {
				s := string(v)
				r[i] = &s
			}
		}
		rows = append(rows, r)
	}
	_, err := res.Close()
	return rows, err
}

// selectAll is the text of a select of whole rows of the table by equality of the columns
func selectAll(m *tableMeta, cols []string) string {
	var b strings.Builder
	b.WriteString("SELECT * FROM ")
	b.WriteString(quoteIdent(m.name))
	for i, c := range cols {
		if i == 0 {
			b.WriteString(" WHERE ")
		} else {
			b.WriteString(" AND ")
		}
		b.WriteString(quoteIdent(c))
		b.WriteString(" = $")
		b.WriteString(strconv.Itoa(i + 1))
	}
	return b.String()
}
