/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"io"
	"regexp"
	"slices"
	"strings"
)

// tableMeta is what the state of a block knows of a table: its columns in their order, its
// primary key, its unique keys, and whether the state may write it
type tableMeta struct {
	name    string
	cols    []*colMeta
	byName  map[string]*colMeta
	pk      []string
	uniques [][]string
	checks  []check
	// readable tells every column has a type the state keeps; writable that the state knows
	// every rule PostgreSQL checks on a write: no foreign key, trigger, generated column, or
	// check it cannot evaluate
	readable, writable bool
}

type colMeta struct {
	name    string
	typ     colType
	typmod  int32
	notNull bool
	// def is the text of the default; hasDef tells there is a default, defOK that the state
	// knows its value
	def           *string
	hasDef, defOK bool
}

// check is a check constraint col >= bound or col > bound on a numeric column
type check struct {
	col    string
	strict bool
	bound  numeric
}

var checkRe = regexp.MustCompile(`^CHECK \(\(\(?"?([a-z_][a-z0-9_]*)"?\)? (>=|>) \(?'?(-?[0-9]+(?:\.[0-9]+)?)'?\)?(?:::numeric)?\)\)$`)

// metaQuery reads the columns, keys, constraints and triggers of a table as one row of json
const metaQuery = `SELECT c.relkind::text,
	coalesce((SELECT json_agg(json_build_object('name', a.attname, 'oid', a.atttypid::int8, 'type', t.typname,
		'typmod', a.atttypmod, 'notnull', a.attnotnull, 'generated', a.attgenerated::text <> '' OR a.attidentity::text <> '',
		'default', pg_get_expr(d.adbin, d.adrelid)) ORDER BY a.attnum)
		FROM pg_attribute a JOIN pg_type t ON t.oid = a.atttypid
		LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped), '[]'),
	coalesce((SELECT json_agg(json_build_object('primary', x.indisprimary,
		'cols', (SELECT json_agg(a.attname ORDER BY k.ord) FROM unnest(x.indkey) WITH ORDINALITY k(attnum, ord)
			JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum)) ORDER BY x.indexrelid)
		FROM pg_index x WHERE x.indrelid = c.oid AND x.indisunique AND x.indpred IS NULL AND x.indexprs IS NULL), '[]'),
	coalesce((SELECT json_agg(json_build_object('type', k.contype::text, 'def', pg_get_constraintdef(k.oid)))
		FROM pg_constraint k WHERE k.conrelid = c.oid AND k.contype IN ('c', 'f', 'x', 't')), '[]'),
	(SELECT count(*) FROM pg_trigger g WHERE g.tgrelid = c.oid AND NOT g.tgisinternal)
	+ (SELECT count(*) FROM pg_rewrite r WHERE r.ev_class = c.oid AND r.rulename <> '_RETURN')
	FROM pg_class c WHERE c.relname = $1 AND c.relnamespace = current_schema()::regnamespace AND c.relkind IN ('r', 'p')`

// loadMeta reads the table from the catalog with a query on the connection; nil for a relation
// that is not a table of the schema
func loadMeta(ctx context.Context, q driver.QueryerContext, table string) (*tableMeta, error) {
	rows, err := q.QueryContext(ctx, metaQuery, []driver.NamedValue{{Ordinal: 1, Value: table}})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dest := make([]driver.Value, 5)
	if err = rows.Next(dest); err == io.EOF {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	text := func(v driver.Value) []byte {
		switch x := v.(type) {
		case []byte:
			return x
		case string:
			return []byte(x)
		}
		return nil
	}
	var cols []struct {
		Name      string
		OID       uint32
		Type      string
		Typmod    int32
		NotNull   bool
		Generated bool
		Default   *string
	}
	var idx []struct {
		Primary bool
		Cols    []string
	}
	var cons []struct {
		Type string
		Def  string
	}
	if err = json.Unmarshal(text(dest[1]), &cols); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(text(dest[2]), &idx); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(text(dest[3]), &cons); err != nil {
		return nil, err
	}
	m := &tableMeta{name: table, byName: make(map[string]*colMeta), readable: true, writable: true}
	if n, _ := dest[4].(int64); n > 0 {
		m.writable = false
	}
	for _, c := range cols {
		cm := &colMeta{name: c.Name, typ: newColType(c.OID, c.Type, c.Typmod), typmod: c.Typmod, notNull: c.NotNull}
		if !cm.typ.supported() {
			m.readable, m.writable = false, false
		}
		if c.Generated {
			m.writable = false
		}
		if c.Default != nil {
			cm.hasDef = true
			cm.def, cm.defOK = defaultValue(cm.typ, *c.Default)
		}
		m.cols = append(m.cols, cm)
		m.byName[c.Name] = cm
	}
	for _, x := range idx {
		if x.Primary {
			m.pk = x.Cols
		}
		m.uniques = append(m.uniques, x.Cols)
	}
	if len(m.pk) == 0 {
		m.readable, m.writable = false, false
	}
	for _, u := range m.uniques {
		for _, c := range u {
			if cm := m.byName[c]; cm == nil || !cm.typ.comparable() {
				m.readable, m.writable = false, false
			}
		}
	}
	for _, k := range cons {
		if k.Type != "c" {
			m.writable = false
			continue
		}
		mt := checkRe.FindStringSubmatch(k.Def)
		if mt == nil {
			m.writable = false
			continue
		}
		cm := m.byName[mt[1]]
		bound, ok := parseNumeric(mt[3])
		if cm == nil || cm.typ.kind != kNumeric && cm.typ.kind != kInt || !ok {
			m.writable = false
			continue
		}
		m.checks = append(m.checks, check{col: mt[1], strict: mt[2] == ">", bound: bound})
	}
	return m, nil
}

var defaultRe = regexp.MustCompile(`^(?:'((?:[^']|'')*)'|(-?[0-9]+(?:\.[0-9]+)?)|\((-?[0-9]+(?:\.[0-9]+)?)\)|(true|false))(?:::[a-z ]+(?:\([0-9, ]+\))?)*$`)

// defaultValue is the text of a default that is a literal, possibly cast; false for others, such
// as a function
func defaultValue(t colType, expr string) (*string, bool) {
	if expr == "NULL" || strings.HasPrefix(expr, "NULL::") {
		return nil, true
	}
	mt := defaultRe.FindStringSubmatch(expr)
	if mt == nil {
		return nil, false
	}
	lit := mt[2] + mt[3] + mt[4]
	if strings.HasPrefix(expr, "'") {
		lit = strings.ReplaceAll(mt[1], "''", "'")
	}
	return t.inputText(lit)
}

// col is the column of the name, nil for none
func (m *tableMeta) col(name string) *colMeta { return m.byName[name] }

// colIndex is the position of the column of the name, -1 for none
func (m *tableMeta) colIndex(name string) int {
	return slices.IndexFunc(m.cols, func(c *colMeta) bool { return c.name == name })
}

// keyOf is the key of a row by the values of the columns: the text of each column as equality
// compares it; false when a column is NULL, as NULL names no row
func (m *tableMeta) keyOf(cols []string, val func(col string) *string) (string, bool) {
	var b strings.Builder
	for i, c := range cols {
		v := val(c)
		if v == nil {
			return "", false
		}
		if i > 0 {
			b.WriteByte(0)
		}
		b.WriteString(m.byName[c].typ.key(*v))
	}
	return b.String(), true
}

// checkRow tells a row holds the checks and the NOT NULL columns of the table
func (m *tableMeta) checkRow(r stateRow) bool {
	for i, c := range m.cols {
		if c.notNull && r[i] == nil {
			return false
		}
	}
	for _, k := range m.checks {
		v := r[m.colIndex(k.col)]
		if v == nil {
			continue
		}
		n, ok := parseNumeric(*v)
		if !ok {
			return false
		}
		d := n.add(k.bound.neg()).sign()
		if d < 0 || k.strict && d == 0 {
			return false
		}
	}
	return true
}

// stateRow is a row of a table: the text of each column in the order of the columns, nil for NULL
type stateRow []*string

func (r stateRow) clone() stateRow { return slices.Clone(r) }
