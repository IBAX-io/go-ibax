/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

// decodedRows hide the type of the rows of the state: Rows read them as they read any driver.Rows,
// by memRows.Next, which decodes each value
type decodedRows struct{ driver.Rows }

func (r decodedRows) ColumnTypeDatabaseTypeName(i int) string {
	return r.Rows.(driver.RowsColumnTypeDatabaseTypeName).ColumnTypeDatabaseTypeName(i)
}

// cellColumns are the columns of each type the encoders of the cells serve, and of the types they
// leave to decodeText, with values of them as PostgreSQL reads them
var cellColumns = []struct {
	typ  string
	vals []string
}{
	{"int2", []string{"-32768", "0", "32767"}},
	{"int4", []string{"-2147483648", "7", "2147483647"}},
	{"int8", []string{"-9223372036854775808", "-1", "9223372036854775807"}},
	{"numeric", []string{"0", "-0.5", "1.50", "123456789012345678901234567890.123", "NaN", "1e-20", "-12"}},
	{"numeric(10,2)", []string{"1", "-3.5", "0"}},
	{"text", []string{"", "é", "a\nb", "NULL"}},
	{"varchar(10)", []string{"", "ab", "x y"}},
	{"bpchar(5)", []string{"ab", "", "abcde"}},
	{"bool", []string{"t", "f", "yes"}},
	{"bytea", []string{"", `\x00ff`, `\xDEADBEEF`, "abc"}},
	{"jsonb", []string{"{}", `{"b": 1, "a": [1, "x", null]}`, "null", `"s"`, "1.50"}},
	{"json", []string{` { "a" : 1 } `, "[]", "2"}},
	{"timestamp", []string{"2020-01-02 03:04:05", "2021-06-07 08:09:10.123456", "infinity"}},
	{"timestamptz", []string{"2020-01-02 03:04:05+02", "1999-12-31 23:59:59.5+00"}},
	{"date", []string{"2020-02-29", "1970-01-01"}},
	{"float8", []string{"1.5", "NaN", "-Infinity"}},
	{"uuid", []string{"A0EEBC99-9C0B-4EF8-BB6D-6BB9BD380A11"}},
}

// cellRows are the rows of a table of a column of each type of cellColumns, the last of NULLs, as
// database/sql reads them and as the state keeps them
func cellRows(t *testing.T) (conn *sql.Conn, query string, mem func() *memRows) {
	ctx := context.Background()
	conn = testConn(t)
	var cols, defs []string
	width := 0
	for i, c := range cellColumns {
		cols = append(cols, fmt.Sprintf("c%d", i))
		defs = append(defs, fmt.Sprintf("c%d %s", i, c.typ))
		width = max(width, len(c.vals))
	}
	_, err := conn.ExecContext(ctx, `CREATE TEMP TABLE ce_t (id int8 PRIMARY KEY, `+strings.Join(defs, ", ")+`)`)
	require.NoError(t, err)
	for r := 0; r <= width; r++ {
		args := []any{int64(r)}
		marks := []string{"$1"}
		for _, c := range cellColumns {
			var v any
			if r < width {
				v = c.vals[r%len(c.vals)]
			}
			args = append(args, v)
			marks = append(marks, fmt.Sprintf("$%d", len(args)))
		}
		_, err := conn.ExecContext(ctx, `INSERT INTO ce_t VALUES (`+strings.Join(marks, ", ")+`)`, args...)
		require.NoError(t, err, "row %d", r)
	}
	query = `SELECT ` + strings.Join(cols, ", ") + ` FROM ce_t ORDER BY id`

	head := &memRows{names: cols}
	trows, err := conn.QueryContext(ctx, `SELECT a.atttypid, t.typname, a.atttypmod FROM pg_attribute a
		JOIN pg_type t ON t.oid = a.atttypid WHERE a.attrelid = 'ce_t'::regclass AND a.attnum > 1 ORDER BY a.attnum`)
	require.NoError(t, err)
	for trows.Next() {
		var oid uint32
		var name string
		var mod int32
		require.NoError(t, trows.Scan(&oid, &name, &mod))
		head.types = append(head.types, newColType(oid, name, mod))
		head.mods = append(head.mods, mod)
	}
	require.NoError(t, trows.Err())
	texts := make([]string, len(cols))
	for i, c := range cols {
		// the text of the output function of the type, as the state reads it: c::text trims a bpchar
		texts[i] = "CASE WHEN " + c + " IS NULL THEN NULL ELSE concat(" + c + ") END"
	}
	vrows, err := conn.QueryContext(ctx, `SELECT `+strings.Join(texts, ", ")+` FROM ce_t ORDER BY id`)
	require.NoError(t, err)
	var cells [][]*string
	for vrows.Next() {
		row := make([]*string, len(cols))
		dest := make([]any, len(cols))
		for i := range row {
			dest[i] = &row[i]
		}
		require.NoError(t, vrows.Scan(dest...))
		cells = append(cells, row)
	}
	require.NoError(t, vrows.Err())
	return conn, query, func() *memRows {
		return &memRows{names: head.names, types: head.types, mods: head.mods, rows: cells}
	}
}

// readEach is what each way of reading the rows reads of them
func readEach(t *testing.T, open func() *Rows) string {
	var w strings.Builder
	for mode := range 4 {
		rows := open()
		cols, err := rows.Columns()
		require.NoError(t, err)
		for rows.Next() {
			switch mode {
			case 0:
				dest := make([][]byte, len(cols))
				err := rows.ScanBytes(dest)
				fmt.Fprintf(&w, "bytes %#v %v\n", dest, err)
			case 1, 2:
				dest, null := make([]string, len(cols)), make([]bool, len(cols))
				err := rows.ScanText(dest, null, mode == 2)
				fmt.Fprintf(&w, "text %v %q %v %v\n", mode == 2, dest, null, err)
			case 3:
				dest := make([]any, len(cols))
				strs := make([]any, len(cols))
				for i := range dest {
					dest[i] = new([]byte)
					strs[i] = new(any)
				}
				err := rows.Scan(dest...)
				fmt.Fprintf(&w, "scan-bytes %v", err)
				for _, d := range dest {
					fmt.Fprintf(&w, " %q", *d.(*[]byte))
				}
				err = rows.Scan(strs...)
				fmt.Fprintf(&w, "\nscan-any %v", err)
				for _, d := range strs {
					fmt.Fprintf(&w, " %T(%v)", *d.(*any), *d.(*any))
				}
				w.WriteByte('\n')
			}
		}
		fmt.Fprintf(&w, "err %v close %v\n", rows.Err(), rows.Close())
	}
	return w.String()
}

// TestCellEncodersMatchSQL (T3): the values of each type read from the rows of the state by the
// encoders of the cells are those read through database/sql and those memRows.Next decodes, by
// each way of reading them, and GetAllTransaction makes the same maps of them
func TestCellEncodersMatchSQL(t *testing.T) {
	ctx := context.Background()
	db, query, mem := cellRows(t)
	viaSQL := readEach(t, func() *Rows {
		rows, err := db.QueryContext(ctx, query)
		require.NoError(t, err)
		return StdRows(rows)
	})
	viaDecode := readEach(t, func() *Rows { return driverRows(decodedRows{mem()}) })
	viaEncode := readEach(t, func() *Rows { return driverRows(mem()) })
	stats := &txStats{}
	viaTimed := readEach(t, func() *Rows {
		r, _ := timeQuery(stats, time.Now(), mem(), nil)
		rows := driverRows(r)
		require.NotNil(t, rows.mem)
		return rows
	})
	if os.Getenv("B7CELLDUMP") != "" {
		os.WriteFile("/tmp/b7impl/cell_decode.txt", []byte(viaDecode), 0o644)
		os.WriteFile("/tmp/b7impl/cell_encode.txt", []byte(viaEncode), 0o644)
		os.WriteFile("/tmp/b7impl/cell_sql.txt", []byte(viaSQL), 0o644)
	}
	require.Equal(t, viaDecode, viaEncode)
	require.Equal(t, viaEncode, viaTimed)
	require.Equal(t, int64(4*len(mem().rows)), stats.read.Load())
	require.Equal(t, strings.Split(viaSQL, "\n"), strings.Split(viaEncode, "\n"))

	rows, err := db.QueryContext(ctx, query)
	require.NoError(t, err)
	want, err := getResult(StdRows(rows), -1)
	require.NoError(t, err)
	got, err := getResult(driverRows(mem()), -1)
	require.NoError(t, err)
	require.Equal(t, want, got)
	old, err := getResult(driverRows(decodedRows{mem()}), -1)
	require.NoError(t, err)
	require.Equal(t, old, got)
	for _, limit := range []int{1, 2} {
		got, err := getResult(driverRows(mem()), limit)
		require.NoError(t, err)
		require.Equal(t, want[:limit], got)
	}
}

// FuzzCellEncode checks the text fastCell makes of a value of each type against the value
// decodeText makes of it, as bytesOf and GetAllTransaction write it
func FuzzCellEncode(f *testing.F) {
	for _, c := range cellColumns {
		for _, v := range c.vals {
			f.Add(v)
		}
	}
	for _, s := range []string{"-0", "00", "1.", ".5", "-0.0", "-0.5", "+1", "1e3", `\x`, `\x0`, `\x0g`, `\X00`,
		"true", "false", "T", "{", "", " ", "\xff", "9223372036854775808", "32768", "-32769", "0.000"} {
		f.Add(s)
	}
	oids := []uint32{pgtype.Int2OID, pgtype.Int4OID, pgtype.Int8OID, pgtype.NumericOID, pgtype.TextOID,
		pgtype.VarcharOID, pgtype.BPCharOID, pgtype.BoolOID, pgtype.ByteaOID, pgtype.JSONBOID, pgtype.JSONOID,
		pgtype.TimestampOID, pgtype.TimestamptzOID, pgtype.DateOID, pgtype.Float8OID, pgtype.UUIDOID}
	m := pgtype.NewMap()
	f.Fuzz(func(t *testing.T, s string) {
		for _, oid := range oids {
			out, ok := fastCell(oid, s)
			if !ok {
				continue
			}
			d, err := decodeText(m, oid, s)
			if err != nil {
				t.Fatalf("oid %d %q: fastCell %q, decodeText %v", oid, s, out, err)
			}
			b, err := bytesOf(nil, d)
			if err != nil || b == nil || string(b) != out {
				t.Fatalf("oid %d %q: fastCell %q, bytesOf %q %v", oid, s, out, b, err)
			}
			if oid == pgtype.ByteaOID && hex.EncodeToString(b) != s[2:] {
				t.Fatalf("bytea %q: hex %q", s, hex.EncodeToString(b))
			}
		}
	})
}

// timestampZones are zones of time.Local with offsets of both signs, of minutes, and of seconds in
// their past
var timestampZones = []string{"UTC", "Asia/Shanghai", "America/St_Johns", "Asia/Kathmandu", "Europe/London",
	"Pacific/Chatham", "America/Los_Angeles", "Asia/Kolkata", "Europe/Amsterdam", "Africa/Monrovia"}

// inLocal runs f with time.Local the zone name
func inLocal(t testing.TB, name string, f func()) {
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	prev := time.Local
	time.Local = loc
	defer func() { time.Local = prev }()
	f()
}

// checkTimestamp checks fastCell of the text s of a timestamp of each kind against decodeText and
// bytesOf, and is whether it makes the text of each kind
func checkTimestamp(t testing.TB, m *pgtype.Map, s string) (tz, plain bool) {
	for _, oid := range []uint32{pgtype.TimestamptzOID, pgtype.TimestampOID} {
		out, ok := fastCell(oid, s)
		if !ok {
			continue
		}
		d, err := decodeText(m, oid, s)
		if err != nil {
			t.Fatalf("oid %d %q in %s: fastCell %q, decodeText %v", oid, s, time.Local, out, err)
		}
		if b, err := bytesOf(nil, d); err != nil || string(b) != out {
			t.Fatalf("oid %d %q in %s: fastCell %q, bytesOf %q %v", oid, s, time.Local, out, b, err)
		}
		if oid == pgtype.TimestamptzOID {
			tz = true
		} else {
			plain = true
		}
	}
	return tz, plain
}

// fastTimestamps are texts PostgreSQL outputs of timestamps of years of four digits, with an
// offset and without
var fastTimestamps = []string{"2020-01-02 03:04:05", "0001-01-01 00:00:00", "9999-12-31 23:59:59.999999",
	"2024-02-29 12:00:00.5", "2000-02-29 00:00:00.000001", "1970-01-01 00:00:00.12", "0099-06-30 23:59:59"}

// fastOffsets are offsets PostgreSQL outputs
var fastOffsets = []string{"+00", "+08", "-03:30", "+05:45", "+12:45", "-00:44:30", "+05:53:28", "+14", "-12", "+00:19:32"}

// slowTimestamps are texts fastCell leaves to decodeText, of each kind
var slowTimestamps = []string{"infinity", "-infinity", "2020-01-02 03:04:05 BC", "2020-01-02 03:04:05+00 BC",
	"10000-01-01 00:00:00", "10000-01-01 00:00:00+00", "2020-01-02 03:04:05.1234567", "2020-01-02 03:04:05.1234567+00",
	"2020-01-02 03:04:05.", "2020-01-02 03:04:05.+00", "2020-01-02 03:04:05Z", "2020-01-02 03:04:05+100",
	"2020-01-02 03:04:05+1", "2020-01-02 03:04:05+05:3", "2020-01-02 03:04:05+05:30:1", "2020-01-02 03:04:05+05:60",
	"2020-01-02 03:04:05+05:30:60", "2020-01-02 03:04:05+05:30:00:00", "2020-01-02 03:04:05+05-30",
	"2021-02-29 00:00:00", "1900-02-29 00:00:00+00", "2020-04-31 00:00:00", "2020-13-01 00:00:00", "2020-00-01 00:00:00",
	"2020-01-00 00:00:00", "2020-01-02 24:00:00", "2020-01-02 23:60:00", "2020-01-02 23:59:60", "0000-01-01 00:00:00",
	"2020-01-02T03:04:05", "2020-01-02 03:04", "2020-01-02", "2020-1-02 03:04:05", "２020-01-02 03:04:05",
	"2020-01-02 03:04:05 ", "2020-01-02 03:04:05 +00", "2020-01-02 03:04:05-", "2020/01/02 03:04:05", "",
	"2020-11-31 00:00:00", "2020-06-31 00:00:00+00", "2020-01-02 03:04:05*08", "2020-01-02 03:04:05.5 08"}

// TestFastTimestamp: fastCell makes the texts of the timestamps PostgreSQL outputs of years of
// four digits as decodeText and bytesOf make them in each time.Local, and leaves the others
func TestFastTimestamp(t *testing.T) {
	m := pgtype.NewMap()
	for _, zone := range timestampZones {
		inLocal(t, zone, func() {
			for _, s := range fastTimestamps {
				if tz, plain := checkTimestamp(t, m, s); tz || !plain {
					t.Fatalf("%q: timestamptz %v, timestamp %v", s, tz, plain)
				}
				for _, off := range fastOffsets {
					if tz, plain := checkTimestamp(t, m, s+off); !tz || plain {
						t.Fatalf("%q: timestamptz %v, timestamp %v", s+off, tz, plain)
					}
				}
			}
			for _, s := range slowTimestamps {
				if tz, plain := checkTimestamp(t, m, s); tz || plain {
					t.Fatalf("%q: timestamptz %v, timestamp %v", s, tz, plain)
				}
			}
		})
	}
}

// TestFastTimestampOutputs: fastCell makes the text of each timestamp PostgreSQL outputs from
// year 1 to year 7800 in sessions of zones of both signs and of seconds as decodeText and bytesOf
// make it in each time.Local, and leaves to decodeText the texts of BC of year 1 in zones west of
// UTC
func TestFastTimestampOutputs(t *testing.T) {
	conn := testConn(t)
	ctx := context.Background()
	m := pgtype.NewMap()
	n, bc := 0, 0
	for _, session := range []string{"UTC", "Asia/Kolkata", "America/St_Johns", "Europe/Amsterdam", "Africa/Monrovia",
		"Pacific/Chatham", "America/Los_Angeles"} {
		_, err := conn.ExecContext(ctx, `SET TimeZone = '`+session+`'`)
		require.NoError(t, err)
		rows, err := conn.QueryContext(ctx, `SELECT to_timestamp(e)::text, (to_timestamp(e) AT TIME ZONE 'UTC')::text
			FROM (SELECT -62135596800 + i::float8 * 1234567.891234 + CASE WHEN i % 3 = 0 THEN 0 ELSE (i % 1000) * 0.000001 END
			AS e FROM generate_series(0, 200000, 13) i) s`)
		require.NoError(t, err)
		var texts []string
		for rows.Next() {
			var a, b string
			require.NoError(t, rows.Scan(&a, &b))
			texts = append(texts, a, b)
		}
		require.NoError(t, rows.Err())
		rows.Close()
		for _, zone := range []string{"UTC", "Asia/Kathmandu", "America/St_Johns", "Africa/Monrovia"} {
			inLocal(t, zone, func() {
				for k, s := range texts {
					tz, plain := checkTimestamp(t, m, s)
					if strings.HasSuffix(s, " BC") {
						// year 1 in a zone of a negative offset
						if tz || plain {
							t.Fatalf("%s %q: timestamptz %v, timestamp %v", session, s, tz, plain)
						}
						bc++
						continue
					}
					if k%2 == 0 && (!tz || plain) || k%2 == 1 && (tz || !plain) {
						t.Fatalf("%s %q: timestamptz %v, timestamp %v", session, s, tz, plain)
					}
					n++
				}
			})
		}
	}
	if n < 400000 || bc == 0 {
		t.Fatalf("%d texts, %d of BC", n, bc)
	}
}

// FuzzFastTimestamp checks fastCell of texts of timestamps in zones of time.Local
func FuzzFastTimestamp(f *testing.F) {
	for _, s := range fastTimestamps {
		for _, off := range fastOffsets {
			f.Add(s + off)
		}
		f.Add(s)
	}
	for _, s := range slowTimestamps {
		f.Add(s)
	}
	m := pgtype.NewMap()
	f.Fuzz(func(t *testing.T, s string) {
		for _, zone := range []string{"UTC", "America/St_Johns", "Asia/Kathmandu", "Africa/Monrovia"} {
			inLocal(t, zone, func() { checkTimestamp(t, m, s) })
		}
	})
}
