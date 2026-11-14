/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql/driver"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// bindMetas are a table of a column of each type the state keeps, with a default the state knows
// and, in the second, one it does not
var bindMetas = func() []*tableMeta {
	seven := "7"
	var out []*tableMeta
	for _, defOK := range []bool{true, false} {
		m := &tableMeta{name: "t", byName: map[string]*colMeta{}, pk: []string{"id"}, uniques: [][]string{{"id"}},
			readable: true, writable: true}
		for _, c := range []struct {
			name, typ string
			oid       uint32
			typmod    int32
		}{
			{"id", "int8", pgtype.Int8OID, -1}, {"i4", "int4", pgtype.Int4OID, -1}, {"i2", "int2", pgtype.Int2OID, -1},
			{"n", "numeric", pgtype.NumericOID, -1}, {"n2", "numeric", pgtype.NumericOID, 10<<16 | 2 + 4},
			{"tx", "text", pgtype.TextOID, -1}, {"vc", "varchar", pgtype.VarcharOID, 5 + 4}, {"bo", "bool", pgtype.BoolOID, -1},
			{"by", "bytea", pgtype.ByteaOID, -1}, {"j", "jsonb", pgtype.JSONBOID, -1},
			{"tz", "timestamptz", pgtype.TimestamptzOID, -1}, {"tn", "timestamp", pgtype.TimestampOID, -1},
			{"nd", "int8", pgtype.Int8OID, -1},
		} {
			cm := &colMeta{name: c.name, typ: newColType(c.oid, c.typ, c.typmod), typmod: c.typmod}
			if c.name == "nd" {
				cm.def, cm.hasDef, cm.defOK = &seven, true, defOK
			}
			m.cols = append(m.cols, cm)
			m.byName[c.name] = cm
		}
		out = append(out, m)
	}
	return out
}()

// bindTexts are statements of each kind the state runs, with each expression the binder binds,
// and with what the binder rejects whatever the values
var bindTexts = []string{
	`SELECT * FROM t WHERE id = $1`,
	`SELECT * FROM t WHERE i4 = $1 AND tx = $2 ORDER BY id LIMIT $3 OFFSET $4`,
	`SELECT * FROM t WHERE n > $1 OR n2 <= $2 ORDER BY n DESC, id LIMIT 10 OFFSET 2`,
	`SELECT * FROM t WHERE id IN ($1, $2, $3)`,
	`SELECT * FROM t WHERE id IN ($1, $2) AND tx IN ($3, 'a', $4)`,
	`INSERT INTO t (id, i4, tn, i2) VALUES ($1, $2::numeric, $3::timestamptz, $4::int8)`,
	`SELECT * FROM t WHERE tx IN ($1)`,
	`SELECT * FROM t WHERE id NOT IN ($1, 5)`,
	`SELECT * FROM t WHERE bo IS NULL AND tx IS NOT NULL`,
	`SELECT * FROM t WHERE n = $1::numeric AND n2 = $2::numeric(5,1)`,
	`SELECT * FROM t WHERE n = $1 + $2`,
	`SELECT * FROM t WHERE n = 1.50 AND n2 = '3.10' AND id = 3 + $1`,
	`SELECT * FROM t WHERE n IN (1.50, $1, '2.0')`,
	`SELECT * FROM t WHERE n2 = $1 + 1.5 AND i4 = $2 - 1`,
	`SELECT * FROM t WHERE by = decode($1, 'hex')`,
	`SELECT * FROM t WHERE by = decode('00ff', 'hex') AND tz >= to_timestamp(1700000000.5)`,
	`SELECT * FROM t WHERE tz = to_timestamp($1)`,
	`SELECT * FROM t WHERE i4 = $1 AND id = $1`,
	`SELECT * FROM t WHERE tz = to_timestamp($1) AND id = $1`,
	`SELECT * FROM t WHERE id = $1 AND tz = to_timestamp($1)`,
	`SELECT * FROM t WHERE tz = to_timestamp($1) AND tz < to_timestamp($1)`,
	`SELECT id, tx FROM t WHERE id = $1`,
	`SELECT * FROM t LIMIT 5`,
	`SELECT * FROM t WHERE id > $1 LIMIT $2`,
	`SELECT * FROM t WHERE j = $1::jsonb AND vc = $2 AND bo = $3 AND tn = $4::timestamp`,
	`SELECT * FROM t WHERE id = $1::int4 AND i2 = $2::int2 AND tx = $3::varchar(3)`,
	`SELECT * FROM t WHERE (id = $1 OR id = $2) AND tx <> $3`,
	`SELECT * FROM t WHERE tx = 'abc' AND bo = true AND n2 = NULL`,
	`SELECT * FROM t WHERE tx < $1`,
	`SELECT * FROM t WHERE vc = $1 AND tx = $1`,
	`SELECT * FROM t WHERE j = $1::jsonb || $2::jsonb`,
	`SELECT * FROM t WHERE tz = $1::timestamptz AND tn = $2`,
	`SELECT * FROM t WHERE i2 = $1 ORDER BY tx LIMIT 1`,
	`SELECT * FROM t WHERE id = $2`,
	`SELECT id FROM t ORDER BY id DESC LIMIT 1`,
	`SELECT * FROM t WHERE id = $1 LIMIT $1`,
	`INSERT INTO t (id, i4, i2, n, n2, tx, vc, bo, by, j, tz, tn, nd) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
	`INSERT INTO t (id, n, by, tz, j) VALUES ($1, $2::numeric, decode($3, 'hex'), to_timestamp($4), $5::jsonb || '{"a":1}'::jsonb)`,
	`INSERT INTO t (id, i4, nd, n2) VALUES ($1, 5 + $2, NULL, $3 - 0.005)`,
	`INSERT INTO t (id, tx, vc) VALUES ($1, 'abc', 'abcdef')`,
	`INSERT INTO t (id, n2) VALUES ($1, 12345678.999)`,
	`INSERT INTO t (id, id) VALUES ($1, $2)`,
	`INSERT INTO t (id, nope) VALUES ($1, $2)`,
	`INSERT INTO t (id, tz, tn, nd) VALUES ($1, to_timestamp($2), $3, $2)`,
	`INSERT INTO t (id, bo, by) VALUES ($1, $2, $3)`,
	`UPDATE t SET n = n + $1 WHERE id = $2`,
	`UPDATE t SET n = n - $1::numeric, tx = $2, j = j || $3::jsonb WHERE id = $4 AND i4 = $5`,
	`UPDATE t SET tz = to_timestamp($1), by = decode($2, 'hex') WHERE id IN ($3, $4)`,
	`UPDATE t SET id = $1 WHERE id = $2`,
	`UPDATE t SET i4 = i2 + i4, i2 = $1 WHERE tx = $2`,
	`UPDATE t SET n2 = n2::numeric + $1, i4 = 3 + 4, vc = 'toolongvalue'`,
	`UPDATE t SET i4 = $1, i4 = $2`,
	`UPDATE t SET n = $1 + n, j = '{"b":2}'::jsonb || j WHERE n2 > $2`,
	`UPDATE t SET n = (n + $1)::numeric(12,3) WHERE id = $2`,
	`DELETE FROM t WHERE id = $1`,
	`DELETE FROM t WHERE n2 > $1 AND bo = $2`,
	`DELETE FROM t`,
	`SELECT * FROM t WHERE i4 = $1 LIMIT $1`,
	`SELECT * FROM t WHERE i4 = $1 LIMIT 3 OFFSET $1`,
	`SELECT * FROM t WHERE nope = 1`,
	`SELECT * FROM t WHERE tx = 'a' || 'b'`,
	`SELECT * FROM t WHERE n = $1::numeric(0)`,
	`SELECT * FROM t WHERE i4 = 'abc'::int4`,
	`SELECT * FROM t WHERE by = decode(5, 'hex')`,
	`SELECT * FROM t WHERE by = decode(NULL::text, 'hex')`,
	`SELECT * FROM t WHERE by = decode('zz', 'hex')`,
	`SELECT * FROM t WHERE tz = to_timestamp(1e400)`,
	`SELECT * FROM t WHERE tz = to_timestamp('1')`,
	`SELECT * FROM t WHERE id = 9223372036854775807 + 1`,
	`SELECT * FROM t WHERE n = 9223372036854775807 + 1`,
	`SELECT * FROM t WHERE bo = $1 + 1`,
	`SELECT * FROM t WHERE i4 = '7' + 1`,
	`UPDATE t SET i4 = nope`,
	`UPDATE t SET i4 = nope + 1`,
	`UPDATE t SET i4 = i4 + 'abc'`,
	`UPDATE t SET i4 = bo + 1`,
	`UPDATE t SET by = decode(tx, 'hex')`,
	`UPDATE t SET tz = to_timestamp(i4)`,
	`UPDATE t SET n = (n + 1)::varchar(0)`,
	`UPDATE t SET n = $1 + by`,
	`INSERT INTO t (id, i4) VALUES ($1, 'abc')`,
	`SELECT * FROM t WHERE i4 = $1::int8 AND tx = $1`,
	`UPDATE t SET i4 = 'abc' + i4`,
	`INSERT INTO t (id, i4) VALUES ($1, $2::numeric(2000))`,
}

// bindStrings are texts the values of the tests take
var bindStrings = []string{"1", "-5", "3.10", "abc", `\x00ff`, "00ff", "1700000000.25", `{"a":1}`, "t", "2023-01-01 00:00:00",
	"99999999999999999999", "", " 12 ", "1e3", "0.005", "-0", "abcdef", "2023-01-01 00:00:00+00", "nan", "\xff", "32768", "-1"}

// bindArgs are the values of a test: by kinds, three bits a value
func bindArgs(n int, kinds uint64, i int64, a, b string) []driver.NamedValue {
	args := make([]driver.NamedValue, n)
	for k := range args {
		var v any
		sel := int(kinds>>(3*k)) & 7
		str := bindStrings[(int(kinds>>40)+k*5)%len(bindStrings)]
		switch sel {
		case 0:
			v = nil
		case 1:
			v = i + int64(k)
		case 2:
			v = a
		case 3:
			v = b
		case 4:
			v = str
		case 5:
			v = []byte(a)
		case 6:
			v = decimal.RequireFromString("12.5").Add(decimal.NewFromInt(i % 1000))
		case 7:
			if k%2 == 0 {
				v = int(i >> 3)
			} else {
				v = uint64(i)
			}
		}
		args[k] = driver.NamedValue{Ordinal: k + 1, Value: v}
	}
	return args
}

// bindPlans are the plans of bindTexts
var bindPlans = func() []*stmt {
	cat := &stateCatalog{rels: map[string]bool{"t": true}}
	var out []*stmt
	for _, text := range bindTexts {
		out = append(out, parseStmt(text, cat))
	}
	return out
}()

// checkBindings checks the program of each plan for each table against the binder with the values
// of kinds and the texts, running each program three times to check that a run leaves nothing to
// the next
func checkBindings(t *testing.T, kinds uint64, i int64, a, b string) (bound int) {
	for k, p := range bindPlans {
		for _, m := range bindMetas {
			prog := compileBind(p, m)
			for r, ks := range []uint64{kinds, kinds ^ 0x924924924, kinds} {
				for _, n := range []int{prog.need, 13} {
					args := bindArgs(n, ks, i, a, b)
					want, wantOK := bindPlan(p, m, args)
					got, ok := prog.run(args)
					if d := diffBinding(want, wantOK, got, ok); d != "" {
						t.Fatalf("%s (defOK %v, run %d, %d values %v): %s", bindTexts[k], m.cols[12].defOK, r, n, args, d)
					}
					if ok {
						bound++
					}
				}
			}
			args := bindArgs(max(prog.need-1, 0), kinds, i, a, b)
			_, wantOK := bindPlan(p, m, args)
			if _, ok := prog.run(args); ok != wantOK {
				t.Fatalf("%s with %d values: bound %v, want %v", bindTexts[k], len(args), ok, wantOK)
			}
		}
	}
	return bound
}

// TestBindProgramMatchesBinder (T1): the program of each plan binds the values of each kind as the
// binder does, and the plans take the kinds of the state
func TestBindProgramMatchesBinder(t *testing.T) {
	kinds := map[stmtKind]int{}
	for k, p := range bindPlans {
		switch p.kind {
		case stSelect, stMaxID, stInsert, stUpdate, stDelete:
			kinds[p.kind]++
		default:
			t.Errorf("%s: kind %d", bindTexts[k], p.kind)
		}
	}
	if len(kinds) != 5 {
		t.Fatalf("kinds %v", kinds)
	}
	bad, good := 0, 0
	for _, p := range bindPlans {
		if compileBind(p, bindMetas[0]).bad {
			bad++
		} else {
			good++
		}
	}
	if bad < 5 || good < 40 {
		t.Fatalf("%d plans the binder rejects, %d it binds", bad, good)
	}
	bound := 0
	for s := uint64(0); s < 1024; s++ {
		kinds := s*0x9E3779B97F4A7C15 ^ s<<7
		for _, ab := range [][2]string{{"1", "2"}, {"3.10", "abc"}, {`\x00ff`, "1700000000"}, {`{"a":1}`, "t"}, {"", " -7 "}} {
			bound += checkBindings(t, kinds, int64(s)*7919-5000, ab[0], ab[1])
		}
	}
	if bound < 20000 {
		t.Fatalf("%d runs bound", bound)
	}
}

// TestBindProgramPerTable: a plan keeps the program of the table it last ran on, and makes another
// for another table
func TestBindProgramPerTable(t *testing.T) {
	p := parseStmt(`INSERT INTO t (id) VALUES ($1)`, &stateCatalog{rels: map[string]bool{"t": true}})
	args := []driver.NamedValue{{Ordinal: 1, Value: int64(3)}}
	prog, bd, ok := bindOf(p, bindMetas[0], args)
	if !ok || prog.meta != bindMetas[0] || *bd.row[0] != "3" || *bd.row[12] != "7" {
		t.Fatalf("first table: %v %v", ok, bd.row)
	}
	if again, _, _ := bindOf(p, bindMetas[0], args); again != prog {
		t.Fatal("the program is made again for the same table")
	}
	if _, _, ok := bindOf(p, bindMetas[1], args); ok {
		t.Fatal("bound with a default the state does not know")
	}
	if p.prog.Load().meta != bindMetas[1] {
		t.Fatal("the program of the other table is not kept")
	}
}

// FuzzBindProgram checks the program of each plan against the binder with random values
func FuzzBindProgram(f *testing.F) {
	for _, s := range bindStrings {
		f.Add(uint64(len(s))*0x1234567, int64(len(s)), s, "1")
	}
	f.Fuzz(func(t *testing.T, kinds uint64, i int64, a, b string) {
		checkBindings(t, kinds, i, a, b)
	})
}

// TestDiffBinding: diffBinding tells each difference of two bindings
func TestDiffBinding(t *testing.T) {
	cat := &stateCatalog{rels: map[string]bool{"t": true}}
	m := bindMetas[0]
	bind := func(text string, vals ...any) planBinding {
		args := make([]driver.NamedValue, len(vals))
		for i, v := range vals {
			args[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
		}
		bd, ok := bindPlan(parseStmt(text, cat), m, args)
		if !ok {
			t.Fatalf("%s: not bound", text)
		}
		return bd
	}
	sel := `SELECT * FROM t WHERE id IN ($1, $2) AND (tx = $3 OR tx IS NULL) ORDER BY n LIMIT $4 OFFSET $5`
	upd := `UPDATE t SET n = n + $1, tx = $2`
	ins := `INSERT INTO t (id, tx) VALUES ($1, $2)`
	if d := diffBinding(bind(sel, 1, 2, "a", 3, 4), true, bind(sel, 1, 2, "a", 3, 4), true); d != "" {
		t.Fatal(d)
	}
	other := planBinding{limit: -1}
	for name, c := range map[string][2]planBinding{
		"value":      {bind(sel, 1, 2, "a", 3, 4), bind(sel, 1, 5, "a", 3, 4)},
		"nested":     {bind(sel, 1, 2, "a", 3, 4), bind(sel, 1, 2, "b", 3, 4)},
		"null":       {bind(sel, 1, 2, "a", 3, 4), bind(sel, 1, nil, "a", 3, 4)},
		"limit":      {bind(sel, 1, 2, "a", 3, 4), bind(sel, 1, 2, "a", 6, 4)},
		"offset":     {bind(sel, 1, 2, "a", 3, 4), bind(sel, 1, 2, "a", 3, 7)},
		"cond":       {bind(sel, 1, 2, "a", 3, 4), other},
		"order":      {bind(sel, 1, 2, "a", 3, 4), bind(`SELECT * FROM t WHERE id IN ($1, $2) AND (tx = $3 OR tx IS NULL) ORDER BY id LIMIT $4 OFFSET $5`, 1, 2, "a", 3, 4)},
		"op":         {bind(sel, 1, 2, "a", 3, 4), bind(`SELECT * FROM t WHERE id IN ($1, $2) AND (tx <> $3 OR tx IS NULL) ORDER BY n LIMIT $4 OFFSET $5`, 1, 2, "a", 3, 4)},
		"kids":       {bind(sel, 1, 2, "a", 3, 4), bind(`SELECT * FROM t WHERE id IN ($1, $2) AND (tx = $3 OR tx IS NULL OR tx = 'b') ORDER BY n LIMIT $4 OFFSET $5`, 1, 2, "a", 3, 4)},
		"set value":  {bind(upd, 1, "a"), bind(upd, 2, "a")},
		"set const":  {bind(upd, 1, "a"), bind(upd, 1, nil)},
		"set node":   {bind(upd, 1, "a"), bind(`UPDATE t SET n = n - $1, tx = $2`, 1, "a")},
		"set kids":   {bind(upd, 1, "a"), bind(`UPDATE t SET n = $1, tx = $2`, 1, "a")},
		"set column": {bind(upd, 1, "a"), bind(`UPDATE t SET n2 = n2 + $1, tx = $2`, 1, "a")},
		"sets":       {bind(upd, 1, "a"), bind(`UPDATE t SET n = n + $1`, 1)},
		"row":        {bind(ins, 1, "a"), bind(ins, 1, "b")},
		"row null":   {bind(ins, 1, "a"), bind(ins, 1, nil)},
		"no row":     {bind(ins, 1, "a"), other},
	} {
		if diffBinding(c[0], true, c[1], true) == "" || diffBinding(c[1], true, c[0], true) == "" {
			t.Errorf("%s: no difference", name)
		}
	}
	if diffBinding(other, true, other, false) == "" || diffBinding(other, false, other, false) != "" {
		t.Error("bound")
	}
}

// stableTypes are a type of each kind stableInput tells of, with and without a modifier
var stableTypes = []colType{
	typeInt8, newColType(pgtype.Int4OID, "int4", -1), newColType(pgtype.Int2OID, "int2", -1),
	typeNumeric, newColType(pgtype.NumericOID, "numeric", 10<<16|2+4), newColType(pgtype.NumericOID, "numeric", 3<<16|0+4),
	typeText, newColType(pgtype.VarcharOID, "varchar", -1), newColType(pgtype.VarcharOID, "varchar", 5+4),
	newColType(pgtype.BPCharOID, "bpchar", 3+4), typeBool, typeBytea, typeJSONB, newColType(pgtype.UUIDOID, "uuid", -1),
}

// checkStableInput: the input of a type stableInput tells of takes each text it makes of s back
// as it is, so converting a value of the type to the type leaves it as it is
func checkStableInput(t *testing.T, s string) {
	for _, ct := range stableTypes {
		if !stableInput(ct) {
			t.Fatalf("%s: not stable", ct.name)
		}
		out, ok := ct.inputValue(s)
		if !ok {
			continue
		}
		again, ok := ct.inputValue(out)
		if !ok || again != out {
			t.Fatalf("%s %q: input %q, then %q %v", ct.name, s, out, again, ok)
		}
		v, ok := convert(tval{t: ct, s: &out}, ct)
		if !ok || v.t != ct || v.s == nil || *v.s != out {
			t.Fatalf("%s %q: %q converts to %+v %v", ct.name, s, out, v, ok)
		}
	}
}

// TestStableInput checks stableInput of the texts of the tests, and that it tells of no other kind
func TestStableInput(t *testing.T) {
	extra := []string{"  42", "+7", "1.250", "-0.000", "1E+2", " abc ", "a\x00", "TRUE", "yes", "off",
		`\\x41`, `\101`, `{"b": [1, 2.50, {"a": null}], "a": "xé"}`, `[1e2, -0.0]`, `"😀"`,
		"{A0EEBC99-9C0B-4EF8-BB6D-6BB9BD380A11}", "a0eebc999c0b4ef8bb6d6bb9bd380a11", "ab   ", "abcdef  ", "é"}
	for _, s := range append(append([]string{}, bindStrings...), extra...) {
		checkStableInput(t, s)
	}
	for _, ct := range []colType{typeTimestamptz, newColType(pgtype.TimestampOID, "timestamp", -1), typeUnknown} {
		if stableInput(ct) {
			t.Fatalf("%s: stable", ct.name)
		}
	}
}

// FuzzStableInput is TestStableInput of random texts
func FuzzStableInput(f *testing.F) {
	for _, s := range bindStrings {
		f.Add(s)
	}
	f.Fuzz(checkStableInput)
}

// TestInputArgInt64: the input of an int64 is the input of its text for each integer and numeric
// type, at the bounds of each
func TestInputArgInt64(t *testing.T) {
	types := []colType{typeInt8, newColType(pgtype.Int4OID, "int4", -1), newColType(pgtype.Int2OID, "int2", -1),
		typeNumeric, newColType(pgtype.NumericOID, "numeric", 10<<16|2+4), newColType(pgtype.NumericOID, "numeric", 3<<16|0+4),
		typeText}
	var xs []int64
	for _, b := range []int64{math.MaxInt16, math.MaxInt32, 999, 99999999} {
		xs = append(xs, b-1, b, b+1, -b-2, -b-1, -b)
	}
	xs = append(xs, 0, -1, 1, math.MaxInt64, math.MinInt64, math.MinInt64+1, math.MaxInt64-1)
	for _, ct := range types {
		for _, x := range xs {
			s, null, ok := ct.inputArg(x)
			want, wantOK := ct.inputValue(strconv.FormatInt(x, 10))
			if null || ok != wantOK || ok && s != want {
				t.Fatalf("%s %d: %q %v %v, want %q %v", ct.name, x, s, null, ok, want, wantOK)
			}
		}
	}
}

// renderedScan is the text and the indexes of the values of the statement merge rendered of the
// plan p at each read before the statements were of the programs
func renderedScan(p *stmt, m *tableMeta, limit, offset, extra int64) (string, []int) {
	bounded := limit >= 0 || offset > 0
	var b strings.Builder
	b.WriteString("SELECT * FROM ")
	b.WriteString(quoteIdent(m.name))
	if p.alias != "" {
		b.WriteString(" AS ")
		b.WriteString(quoteIdent(p.alias))
	}
	params := make(map[int]int)
	var porder []int
	if len(p.whereToks) > 0 {
		b.WriteString(" WHERE ")
		renderTokens(&b, p.whereToks, params, &porder)
	}
	if bounded {
		b.WriteString(" ORDER BY ")
		if len(p.orderToks) > 0 {
			renderTokens(&b, p.orderToks, params, &porder)
			b.WriteString(", ")
		}
		for i, col := range m.pk {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(quoteIdent(col))
		}
		if limit >= 0 {
			b.WriteString(" LIMIT ")
			b.WriteString(strconv.FormatInt(limit+offset+extra, 10))
		}
	}
	return b.String(), porder
}

// scanTexts are statements of the shapes merge reads, with an alias, quotes, placeholders out of
// their order and orders of more than one column
var scanTexts = []string{
	`SELECT * FROM t AS x WHERE x.id = $2 AND x.tx = 'it''s' ORDER BY x.n DESC LIMIT $1`,
	`SELECT * FROM t x WHERE "tx" = $3 AND id > $1 ORDER BY i4 DESC, x.n LIMIT 5 OFFSET $2`,
	`SELECT * FROM t WHERE id = $1 ORDER BY id LIMIT 1`,
	`SELECT * FROM t WHERE id = $1 ORDER BY i4 NULLS FIRST OFFSET 3`,
	`SELECT * FROM t ORDER BY n, "id" DESC LIMIT 2`,
	`SELECT id, tx FROM t WHERE id = $1`,
	`UPDATE t AS u SET n = $1 WHERE u.id = $2`,
	`DELETE FROM t AS d WHERE d.tx = 'a' AND d.id = $1`,
}

// scanBounds are bounds of selects: limit, offset and the rows the state changed
var scanBounds = [][3]int64{{-1, 0, 0}, {-1, 3, 0}, {-1, 3, 4}, {0, 0, 0}, {5, 0, 0}, {5, 2, 7}, {1, 0, 9}, {-1, -1, 2}}

// scanMetas are tables with a quoted name and a primary key of two columns
var scanMetas = func() []*tableMeta {
	var out []*tableMeta
	for _, c := range []struct {
		name string
		pk   []string
	}{{"t", []string{"id"}}, {`a"b`, []string{"id", "i4"}}, {"t", []string{`q"k`}}} {
		src := bindMetas[0]
		m := &tableMeta{name: c.name, cols: src.cols, byName: src.byName, pk: c.pk, uniques: [][]string{c.pk},
			readable: true, writable: true}
		out = append(out, m)
	}
	return out
}()

// scanPlans are the plans of the selects, updates and deletes of bindTexts and scanTexts
var scanPlans = func() []*stmt {
	cat := &stateCatalog{rels: map[string]bool{"t": true}}
	var out []*stmt
	for _, text := range append(slices.Clone(bindTexts), scanTexts...) {
		switch p := parseStmt(text, cat); p.kind {
		case stSelect, stUpdate, stDelete:
			out = append(out, p)
		}
	}
	return out
}()

// TestScanMatchesRendered: the statement of a program is the text and the values merge rendered
// of its plan for each bound, and plans of one statement share it
func TestScanMatchesRendered(t *testing.T) {
	if len(scanPlans) < 60 {
		t.Fatalf("%d plans", len(scanPlans))
	}
	shared := 0
	for _, m := range scanMetas {
		texts := map[string]*string{}
		for _, p := range scanPlans {
			sc := compileBind(p, m).scan
			if sc == nil {
				// the binder rejects the plan, so merge never reads it
				if !compileBind(p, m).bad {
					t.Fatalf("%+v: no statement", p)
				}
				continue
			}
			if again := scanOf(p, m); again.where != sc.where || again.ordered != sc.ordered {
				t.Fatalf("%+v: other texts for the same plan", p)
			}
			for _, bd := range scanBounds {
				want, wantArgs := renderedScan(p, m, bd[0], bd[1], bd[2])
				args := bindArgs(13, 0x249249249, 100, "a", "b")
				key, keep, values, ok := sc.read(args, bd[0], bd[1], bd[2])
				wantValues := make([]any, len(wantArgs))
				for i, k := range wantArgs {
					wantValues[i] = args[k].Value
				}
				if got := key.query(); !ok || !keep || got != want || !slices.Equal(values, wantValues) {
					t.Fatalf("bounds %v: %q %v %v %v, rendered %q %v", bd, got, values, keep, ok, want, wantValues)
				}
				if len(wantArgs) > 0 {
					if _, _, _, ok := sc.read(args[:slices.Max(wantArgs)], bd[0], bd[1], bd[2]); ok {
						t.Fatalf("bounds %v: read without value %d", bd, slices.Max(wantArgs))
					}
				}
				if key.n < 0 {
					if prev, ok := texts[want]; ok && prev != key.text {
						t.Fatalf("%q: two texts", want)
					} else if ok {
						shared++
					}
					texts[want] = key.text
				}
			}
		}
	}
	if shared < 10 {
		t.Fatalf("%d texts shared", shared)
	}
	if compileBind(bindPlans[5], bindMetas[0]).scan != nil {
		t.Fatal("an insert has a statement")
	}
}

// scanKeyOf is the key of a read of merge and the key of its text and values before
func scanKeyOf(p *stmt, m *tableMeta, bd [3]int64, args []driver.NamedValue) (shapeScanKey, string, bool) {
	key, keep, _, _ := compileBind(p, m).scan.read(args, bd[0], bd[1], bd[2])
	text, porder := renderedScan(p, m, bd[0], bd[1], bd[2])
	values := make([]any, len(porder))
	for i, k := range porder {
		values[i] = args[k].Value
	}
	vals, _ := valuesKey(values)
	return key, text + vals, keep
}

// checkScanKeys checks that two reads have equal keys exactly when merge sent PostgreSQL the same
// text with the same values
func checkScanKeys(t *testing.T, i, j int, bi, bj int, ki, kj uint64, a, b string) {
	m := scanMetas[0]
	pi, pj := scanPlans[i%len(scanPlans)], scanPlans[j%len(scanPlans)]
	if compileBind(pi, m).bad || compileBind(pj, m).bad {
		return
	}
	ai, aj := bindArgs(13, ki, 7, a, b), bindArgs(13, kj, 7, a, b)
	key1, old1, keep1 := scanKeyOf(pi, m, scanBounds[bi%len(scanBounds)], ai)
	key2, old2, keep2 := scanKeyOf(pj, m, scanBounds[bj%len(scanBounds)], aj)
	if keep1 && keep2 && (key1 == key2) != (old1 == old2) {
		t.Fatalf("keys %v %v equal %v, texts %q %q", key1, key2, key1 == key2, old1, old2)
	}
}

// TestScanKeys: two reads of merge share the rows of PostgreSQL exactly when they send it the same
// text with the same values
func TestScanKeys(t *testing.T) {
	for i := range scanPlans {
		for j := range scanPlans {
			for _, bd := range [][2]int{{0, 0}, {1, 1}, {4, 4}, {4, 5}, {0, 1}, {5, 5}} {
				for _, ks := range [][2]uint64{{0x924924924, 0x924924924}, {0x924924924, 0x492492492}, {1, 1}} {
					checkScanKeys(t, i, j, bd[0], bd[1], ks[0], ks[1], "1", "2")
				}
			}
		}
	}
}

// FuzzScanKeys checks the keys of reads of merge with random plans, bounds and values
func FuzzScanKeys(f *testing.F) {
	f.Add(0, 1, 0, 0, uint64(0x924924924), uint64(0x924924924), "1", "1")
	f.Add(3, 3, 5, 5, uint64(0x249249249), uint64(0x249249249), "abc", "abd")
	f.Fuzz(func(t *testing.T, i, j, bi, bj int, ki, kj uint64, a, b string) {
		if i < 0 || j < 0 || bi < 0 || bj < 0 {
			return
		}
		checkScanKeys(t, i, j, bi, bj, ki, kj, a, b)
	})
}

// TestMergeOtherMeta: merge leaves to PostgreSQL a read of a table of a meta other than the meta
// of the program
func TestMergeOtherMeta(t *testing.T) {
	p := parseStmt(`SELECT * FROM t WHERE id = $1`, &stateCatalog{rels: map[string]bool{"t": true}})
	args := []driver.NamedValue{{Ordinal: 1, Value: int64(3)}}
	ts := &tableState{meta: bindMetas[1]}
	if _, err := (&TxView{}).merge(t.Context(), nil, ts, compileBind(p, bindMetas[0]), args, nil, nil, -1, 0); err != errUnsupported {
		t.Fatalf("merge: %v", err)
	}
}
