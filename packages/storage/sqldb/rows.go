/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
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
	// mem are the rows of the state drv reads, and stats what counts their reading when drv
	// counts it; cells are then the values of the row read, which Scan decodes as drv does
	mem   *memRows
	stats *txStats
	cells []memCell
	// vals are the values of the row read; err the last error, io.EOF once the rows are read
	vals   []driver.Value
	err    error
	next   bool
	closed bool
	// raw and types are the bytes and the names of the types of the columns ScanText reads by
	// ScanBytes
	raw   [][]byte
	types []string
}

// memCell is a value of a row of the state: its text, nil for NULL, and the text ScanBytes makes
// of it when fast, else the value decodeText makes of it once decoded
type memCell struct {
	raw     *string
	out     string
	fast    bool
	val     driver.Value
	decoded bool
}

// StdRows are the rows database/sql read
func StdRows(r *sql.Rows) *Rows { return &Rows{std: r} }

func driverRows(r driver.Rows) *Rows {
	switch d := r.(type) {
	case *memRows:
		return &Rows{drv: r, mem: d}
	case *timedRows:
		if m, ok := d.Rows.(*memRows); ok {
			return &Rows{drv: r, mem: m, stats: d.n}
		}
	}
	return &Rows{drv: r}
}

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
	if r.mem != nil {
		r.next = true
		r.err = r.nextMem()
	} else {
		if !r.next {
			r.next = true
			r.vals = make([]driver.Value, len(r.drv.Columns()))
		}
		r.err = r.drv.Next(r.vals)
	}
	if r.err != nil {
		r.Close()
		return false
	}
	return true
}

// nextMem reads the next row of the rows of the state as memRows.Next reads it: the values it
// does not decode in full are those whose text ScanBytes makes without decoding
func (r *Rows) nextMem() error {
	var start time.Time
	if r.stats != nil {
		start = time.Now()
	}
	err := r.readMem()
	if r.stats != nil {
		r.stats.nanos.Add(int64(time.Since(start)))
		if err == nil {
			r.stats.read.Add(1)
		}
	}
	return err
}

func (r *Rows) readMem() error {
	m := r.mem
	if m.i >= len(m.rows) {
		return io.EOF
	}
	if r.cells == nil {
		r.cells = make([]memCell, len(m.names))
	}
	row := m.rows[m.i]
	m.i++
	for j, v := range row {
		c := &r.cells[j]
		*c = memCell{raw: v}
		if v == nil {
			continue
		}
		if c.out, c.fast = fastCell(m.types[j].oid, *v); c.fast {
			continue
		}
		d, err := decodeText(m.typeMap(), m.types[j].oid, *v)
		if err != nil {
			return err
		}
		c.val, c.decoded = d, true
	}
	return nil
}

// valueAt is the value of the column i of the row read, as memRows.Next decodes it
func (r *Rows) valueAt(i int) driver.Value {
	if r.mem == nil {
		return r.vals[i]
	}
	c := &r.cells[i]
	if c.raw == nil {
		return nil
	}
	if !c.decoded {
		// a text fastCell reads decodes without error
		c.val, _ = decodeText(r.mem.typeMap(), r.mem.types[i].oid, *c.raw)
		c.decoded = true
	}
	return c.val
}

// fastCell is the text bytesOf makes of the value decodeText makes of the text s of a value of
// the type oid, for the texts PostgreSQL outputs of integers, numerics, texts, bools, byteas,
// jsonb and timestamps of years of four digits, which it makes without decoding them; false for
// other texts, which are decoded
func fastCell(oid uint32, s string) (string, bool) {
	switch oid {
	case pgtype.Int8OID:
		return s, canonInt(s) && fitsInt(s, 64)
	case pgtype.Int4OID:
		return s, canonInt(s) && fitsInt(s, 32)
	case pgtype.Int2OID:
		return s, canonInt(s) && fitsInt(s, 16)
	case pgtype.NumericOID:
		return s, canonDecimal(s)
	case pgtype.TextOID, pgtype.VarcharOID, pgtype.BPCharOID:
		return s, true
	case pgtype.JSONBOID, pgtype.JSONOID:
		return s, s != ""
	case pgtype.BoolOID:
		switch s {
		case "t":
			return "true", true
		case "f":
			return "false", true
		}
	case pgtype.ByteaOID:
		if !byteaHex(s) {
			return "", false
		}
		b := make([]byte, (len(s)-2)/2)
		hex.Decode(b, []byte(s[2:]))
		return string(b), true
	case pgtype.TimestamptzOID:
		return fastTimestamp(s, true)
	case pgtype.TimestampOID:
		return fastTimestamp(s, false)
	}
	return "", false
}

// fastTimestamp is the text bytesOf makes of the time decodeText makes of the text s of a
// timestamp, with an offset when tz, as PostgreSQL outputs one of a year of four digits in the
// style ISO: YYYY-MM-DD HH:MM:SS, up to six digits of a fraction, and for tz the offset
// +HH[:MM[:SS]]. decodeText makes a time of UTC of a timestamp, and of time.Local of a timestamp
// with an offset. false for other texts.
func fastTimestamp(s string, tz bool) (string, bool) {
	const head = len("2006-01-02 15:04:05")
	if len(s) < head || s[4] != '-' || s[7] != '-' || s[10] != ' ' || s[13] != ':' || s[16] != ':' {
		return "", false
	}
	year, ok1 := digitsAt(s, 0, 4)
	month, ok2 := digitsAt(s, 5, 2)
	day, ok3 := digitsAt(s, 8, 2)
	hour, ok4 := digitsAt(s, 11, 2)
	minute, ok5 := digitsAt(s, 14, 2)
	sec, ok6 := digitsAt(s, 17, 2)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || year < 1 || month < 1 || month > 12 || day < 1 ||
		day > daysIn(year, month) || hour > 23 || minute > 59 || sec > 59 {
		return "", false
	}
	i, nsec := head, 0
	if i < len(s) && s[i] == '.' {
		i++
		from := i
		for i < len(s) && i-from < 7 && s[i] >= '0' && s[i] <= '9' {
			nsec = nsec*10 + int(s[i]-'0')
			i++
		}
		if i == from || i-from > 6 {
			return "", false
		}
		for k := i - from; k < 9; k++ {
			nsec *= 10
		}
	}
	off := 0
	if tz {
		if i+3 > len(s) || s[i] != '+' && s[i] != '-' {
			return "", false
		}
		neg := s[i] == '-'
		h, ok := digitsAt(s, i+1, 2)
		if !ok {
			return "", false
		}
		off, i = h*3600, i+3
		for _, unit := range [2]int{60, 1} {
			if i == len(s) {
				break
			}
			v, ok := digitsAt(s, i+1, 2)
			if s[i] != ':' || !ok || v > 59 {
				return "", false
			}
			off, i = off+v*unit, i+3
		}
		if neg {
			off = -off
		}
	}
	if i != len(s) {
		return "", false
	}
	t := time.Date(year, time.Month(month), day, hour, minute, sec, nsec, time.UTC)
	if tz {
		t = t.Add(-time.Duration(off) * time.Second).In(time.Local)
	}
	var buf [len(time.RFC3339Nano) + 8]byte
	return string(t.AppendFormat(buf[:0], time.RFC3339Nano)), true
}

// digitsAt is the number of the n digits of s at i, false when they are not n digits
func digitsAt(s string, i, n int) (int, bool) {
	if i+n > len(s) {
		return 0, false
	}
	v := 0
	for _, c := range []byte(s[i : i+n]) {
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int(c-'0')
	}
	return v, true
}

// daysIn is the number of the days of the month of the year
func daysIn(year, month int) int {
	switch month {
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

// fitsInt tells the integer of the canonical text s is one of bits bits
func fitsInt(s string, bits int) bool {
	_, err := strconv.ParseInt(s, 10, bits)
	return err == nil
}

// canonDecimal tells the text is a numeric as PostgreSQL outputs a finite one: an integer in its
// canonical text, then a point and digits; -0 in front only with digits after the point
func canonDecimal(s string) bool {
	p := len(s)
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			p = i
			break
		}
	}
	if p == len(s) {
		return canonInt(s)
	}
	if p == len(s)-1 {
		return false
	}
	for i := p + 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return canonInt(s[:p]) || s[:p] == "-0"
}

// byteaHex tells the text is a bytea of some bytes as PostgreSQL outputs it in hex
func byteaHex(s string) bool {
	if len(s) < 4 || len(s)%2 != 0 || s[0] != '\\' || s[1] != 'x' {
		return false
	}
	for i := 2; i < len(s); i++ {
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// ScanText reads the columns of the row as ScanBytes reads them, as texts: null tells a NULL, and
// hexBytea writes the bytes of a column of bytea in hex, as GetAllTransaction writes them
func (r *Rows) ScanText(dest []string, null []bool, hexBytea bool) error {
	if r.mem == nil {
		if r.raw == nil {
			r.raw = make([][]byte, len(dest))
		}
		if err := r.ScanBytes(r.raw); err != nil {
			return err
		}
		if hexBytea && r.types == nil {
			types, err := r.DatabaseTypeNames()
			if err != nil {
				return err
			}
			r.types = types
		}
		for i, b := range r.raw {
			switch {
			case b == nil:
				dest[i], null[i] = "", true
			case hexBytea && r.types[i] == "BYTEA":
				dest[i], null[i] = hex.EncodeToString(b), false
			default:
				dest[i], null[i] = string(b), false
			}
		}
		return nil
	}
	if err := r.scannable(len(dest)); err != nil {
		return err
	}
	for i := range r.cells {
		c := &r.cells[i]
		bytea := hexBytea && r.mem.types[i].oid == pgtype.ByteaOID
		switch {
		case c.raw == nil:
			dest[i], null[i] = "", true
		case c.fast && bytea:
			dest[i], null[i] = (*c.raw)[2:], false
		case c.fast:
			dest[i], null[i] = c.out, false
		default:
			b, err := bytesOf(nil, c.val)
			if err != nil {
				return r.scanError(i, err)
			}
			switch {
			case b == nil:
				dest[i], null[i] = "", true
			case bytea:
				dest[i], null[i] = hex.EncodeToString(b), false
			default:
				dest[i], null[i] = string(b), false
			}
		}
	}
	return nil
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
			*b, err = bytesOf(nil, r.valueAt(i))
		} else {
			err = sql.ConvertAssign(driver.ScanContext{}, d, r.valueAt(i))
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
	for i := range dest {
		var b []byte
		var err error
		if r.mem != nil && r.cells[i].fast {
			b = append(dest[i][:0], r.cells[i].out...)
			if b == nil {
				b = noBytes[:0:0]
			}
		} else {
			b, err = bytesOf(dest[i][:0], r.valueAt(i))
		}
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
	width := len(r.vals)
	if r.mem != nil {
		width = len(r.cells)
	}
	if n != width {
		return fmt.Errorf("sql: expected %d destination arguments in Scan, not %d", width, n)
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
