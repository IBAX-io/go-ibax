/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// testConn is a connection to the database of IBAX_TEST_DSN in the time zone of the nodes
func testConn(t *testing.T) *sql.Conn {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	conn, err := db.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	_, err = conn.ExecContext(context.Background(), `SET TimeZone = 'UTC'`)
	require.NoError(t, err)
	return conn
}

// typeInputs are texts for the input function of each type, valid and invalid
var typeInputs = map[string][]string{
	"int2":          {"0", "-32768", "32767", "32768", " 12 ", "1.0", "", "abc", "+5"},
	"int4":          {"0", "-2147483648", "2147483647", "2147483648", "007", "1e3"},
	"int8":          {"0", "9223372036854775807", "-9223372036854775808", "9223372036854775808", "12345678901"},
	"numeric":       {"0", "-0", "1.50", "0001.2300", "1e3", "1.5e-3", "-12.345e2", ".5", "5.", "+.5e1", "1e1001", "NaN", "abc", "", "123456789012345678901234567890.123456789"},
	"numeric(30,0)": {"0", "1.5", "2.5", "-2.5", "123456789012345678901234567890", "1234567890123456789012345678901", "999999999999999999999999999999.4", "999999999999999999999999999999.5"},
	"numeric(10,2)": {"1.005", "1.004", "-1.005", "99999999.99", "99999999.995", "0.001", "12"},
	"text":          {"", "abc", "é中文", " lead and trail ", "a\tb\nc"},
	"varchar(5)":    {"abcde", "abcdef", "abc  ", "abcde   ", "abcde x", "中文中文中", "中文中文中 "},
	"varchar":       {"anything at all"},
	"bpchar(4)":     {"ab", "abcd", "abcd  ", "abcde", "", " a"},
	"bool":          {"t", "f", "true", "FALSE", "yes", "no", "on", "off", "of", "o", "1", "0", " t ", "tr", "y", "n", "2", ""},
	"bytea":         {`\x`, `\x00ff`, `\xABcd`, `\x0`, `\xzz`, `abc`, `\000`},
	"jsonb": {`{}`, `[]`, `null`, `true`, `1`, `1.0`, `1e2`, `-0`, `1.5E-3`, `"s"`, `{"b":1,"a":2}`, `{"aa":1,"b":2,"a":[1,2,{"z":null}]}`,
		`{"a":1,"a":2}`, `{"k":"é\n\"\\/"}`, `"\u0001\u001f"`, `{"x": 1} x`, `1 2`, `{"a":}`, `"\ud800"`, `"😀"`, `"\u0000"`,
		` { "a" : [ 1 , 2 ] } `, `{"long key":1,"short":2,"s":3}`, `123456789012345678901234567890.50`},
	"timestamp":   {"2026-10-23 12:34:56", "2026-10-23 12:34:56.5", "2026-10-23 12:34:56.123456", "2026-10-23 12:34:56.1234567", "2026-10-23", "2026-10-23T01:02:03", "0001-01-01 00:00:00", "10000-01-01", "2026-13-01", "now"},
	"timestamptz": {"2026-10-23 12:34:56", "2026-10-23 12:34:56.000100", "2026-10-23"},
	"uuid": {"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "A0EEBC99-9C0B-4EF8-BB6D-6BB9BD380A11",
		"{a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11}", "a0eebc999c0b4ef8bb6d6bb9bd380a11",
		"a0ee-bc99-9c0b-4ef8-bb6d-6bb9-bd38-0a11", "{a0eebc999c0b4ef8bb6d6bb9bd380a11}",
		"00000000-0000-0000-0000-000000000000", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a1",
		"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11 ", " a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
		"g0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "a0eebc9-99c0b-4ef8-bb6d-6bb9bd380a11",
		"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11-", "a0eebc99--9c0b-4ef8-bb6d-6bb9bd380a11",
		"{a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11}",
		"-a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11a0", ""},
}

// probeTable creates in the transaction of the connection a table with a column of each type of
// typeInputs, and reads its meta
func probeTable(t *testing.T, conn *sql.Conn) (*tableMeta, []string) {
	ctx := context.Background()
	var types []string
	for typ := range typeInputs {
		types = append(types, typ)
	}
	slices.Sort(types)
	ddl := `CREATE TABLE state_probe (id int8 PRIMARY KEY`
	for i, typ := range types {
		ddl += fmt.Sprintf(", c%d %s", i, typ)
	}
	_, err := conn.ExecContext(ctx, ddl+`); INSERT INTO state_probe (id) VALUES (1)`)
	require.NoError(t, err)
	var meta *tableMeta
	require.NoError(t, conn.Raw(func(dc any) error {
		meta, err = loadMeta(ctx, dc.(driver.QueryerContext), "state_probe")
		return err
	}))
	require.NotNil(t, meta)
	return meta, types
}

// begin starts a transaction on the connection the test rolls back
func begin(t *testing.T, conn *sql.Conn) {
	_, err := conn.ExecContext(context.Background(), `BEGIN`)
	require.NoError(t, err)
	t.Cleanup(func() { conn.ExecContext(context.Background(), `ROLLBACK`) })
}

// pgAssign is the text PostgreSQL stores for a value assigned to the column, and the value the
// driver returns for it; false when PostgreSQL rejects it
func pgAssign(t *testing.T, conn *sql.Conn, col string, v any) (string, any, bool) {
	ctx := context.Background()
	_, err := conn.ExecContext(ctx, `SAVEPOINT probe`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `UPDATE state_probe SET "`+col+`" = $1 WHERE id = 1`, v)
	if err != nil {
		_, err = conn.ExecContext(ctx, `ROLLBACK TO SAVEPOINT probe`)
		require.NoError(t, err)
		return "", nil, false
	}
	var value any
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT "`+col+`" FROM state_probe WHERE id = 1`).Scan(&value))
	_, err = conn.ExecContext(ctx, `RELEASE SAVEPOINT probe`)
	require.NoError(t, err)
	text := pgText(t, conn, `SELECT "`+col+`" FROM state_probe WHERE id = 1`)
	require.NotNil(t, text)
	return *text, value, true
}

// pgText is the text PostgreSQL outputs for the value of a query of one row and column, nil for
// NULL
func pgText(t *testing.T, conn *sql.Conn, query string) *string {
	var out *string
	require.NoError(t, conn.Raw(func(dc any) error {
		res, err := dc.(*stdlib.Conn).Conn().PgConn().Exec(context.Background(), query).ReadAll()
		if err != nil {
			return err
		}
		if v := res[0].Rows[0][0]; v != nil {
			s := string(v)
			out = &s
		}
		return nil
	}))
	return out
}

func TestStateTypeInput(t *testing.T) {
	conn := testConn(t)
	begin(t, conn)
	meta, types := probeTable(t, conn)
	require.True(t, meta.readable && meta.writable)
	m := pgtype.NewMap()
	supported, total := 0, 0
	check := func(c *colMeta, v any) {
		total++
		ours, ok := c.typ.input(v)
		text, value, pgOK := pgAssign(t, conn, c.name, v)
		if !ok {
			if pgOK {
				t.Logf("%s %q: left to PostgreSQL, which stores %q", c.typ.name, v, text)
			}
			return
		}
		supported++
		require.True(t, pgOK, "%s %q: the state takes %q, PostgreSQL rejects it", c.typ.name, v, *ours)
		require.Equal(t, text, *ours, "%s %q", c.typ.name, v)
		got, err := decodeText(m, c.typ.oid, *ours)
		require.NoError(t, err)
		require.Equal(t, value, got, "%s %q", c.typ.name, v)
	}
	for i, typ := range types {
		c := meta.col(fmt.Sprintf("c%d", i))
		for _, in := range typeInputs[typ] {
			check(c, in)
		}
	}
	// values of the types of Go the contracts bind
	for i, typ := range types {
		c := meta.col(fmt.Sprintf("c%d", i))
		switch c.typ.kind {
		case kInt, kNumeric:
			for _, v := range []any{int64(-7), int64(1) << 40, decimal.RequireFromString("12.5"), decimal.RequireFromString("-0.005")} {
				check(c, v)
			}
		case kBytea:
			for _, v := range []any{[]byte{}, []byte{0, 1, 0xfe}} {
				check(c, v)
			}
		case kText, kVarchar:
			check(c, []byte("bytes as text"))
		}
		_ = typ
	}
	t.Logf("%d of %d values taken by the state", supported, total)
}

func TestStateTypeOps(t *testing.T) {
	conn := testConn(t)
	ctx := context.Background()
	nums := []string{"0", "1.5", "-2.25", "1e3", "0.000001", "123456789012345678901234567890.5", "-0.10"}
	for _, a := range nums {
		for _, b := range nums {
			x, _ := parseNumeric(a)
			y, _ := parseNumeric(b)
			var want string
			require.NoError(t, conn.QueryRowContext(ctx, `SELECT ($1::numeric + $2::numeric)::text`, a, b).Scan(&want))
			require.Equal(t, want, x.add(y).String(), "%s + %s", a, b)
			require.NoError(t, conn.QueryRowContext(ctx, `SELECT ($1::numeric - $2::numeric)::text`, a, b).Scan(&want))
			require.Equal(t, want, x.add(y.neg()).String(), "%s - %s", a, b)
		}
	}
	objs := []string{`{}`, `{"a":1}`, `{"a":{"x":1},"bb":[1]}`, `{"a":null,"c":"s"}`, `{"b": 2.50, "a": 1}`}
	for _, a := range objs {
		for _, b := range objs {
			var want string
			require.NoError(t, conn.QueryRowContext(ctx, `SELECT ($1::jsonb || $2::jsonb)::text`, a, b).Scan(&want))
			got, ok := concatJSONB(a, b)
			require.True(t, ok)
			require.Equal(t, want, got, "%s || %s", a, b)
		}
	}
	for _, s := range []string{"0", "1700000000", "1700000000.5", "1700000000.1234565", "1700000000.1234575", "-1.5", "253402300799"} {
		var want string
		require.NoError(t, conn.QueryRowContext(ctx, `SELECT to_timestamp($1::float8)::text`, s).Scan(&want))
		tm, ok := unixTimestamp(s)
		require.True(t, ok)
		require.Equal(t, want, colType{kind: kTimestamptz}.timestampText(tm), s)
	}
	uuids := []string{"00000000-0000-0000-0000-000000000000", "ffffffff-ffff-ffff-ffff-ffffffffffff",
		"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "A0EEBC99-9C0B-4EF8-BB6D-6BB9BD380A12", "0a0eebc99c0b4ef8bb6d6bb9bd380a11",
		"9fffffff-ffff-ffff-ffff-ffffffffffff", "{a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11}"}
	for _, a := range uuids {
		for _, b := range uuids {
			var want int
			require.NoError(t, conn.QueryRowContext(ctx,
				`SELECT CASE WHEN $1::uuid < $2::uuid THEN -1 WHEN $1::uuid > $2::uuid THEN 1 ELSE 0 END`, a, b).Scan(&want))
			x, _ := parseUUID(a)
			y, _ := parseUUID(b)
			require.Equal(t, want, compareText(kUUID, x, y), "%s <=> %s", a, b)
		}
	}
}

func TestStateMetaTables(t *testing.T) {
	conn := testConn(t)
	ctx := context.Background()
	rows, err := conn.QueryContext(ctx, `SELECT relname FROM pg_class WHERE relnamespace = current_schema()::regnamespace
		AND relkind IN ('r', 'p') AND NOT relispartition ORDER BY relname`)
	require.NoError(t, err)
	var tables []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		tables = append(tables, n)
	}
	require.NoError(t, rows.Close())
	if len(tables) == 0 {
		t.Skip("the database has no tables of a node")
	}
	for _, table := range tables {
		var meta *tableMeta
		require.NoError(t, conn.Raw(func(dc any) error {
			meta, err = loadMeta(ctx, dc.(driver.QueryerContext), table)
			return err
		}))
		require.NotNil(t, meta, table)
		var undefined []string
		for _, c := range meta.cols {
			if c.hasDef && !c.defOK {
				undefined = append(undefined, c.name)
			}
		}
		if os.Getenv("STATE_META_VERBOSE") != "" || !meta.writable {
			t.Logf("%-32s readable=%v writable=%v pk=%v uniques=%v checks=%d unknown defaults=%v", table, meta.readable, meta.writable, meta.pk, meta.uniques, len(meta.checks), undefined)
		}
	}
}
