/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"bytes"
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/metrics"
)

// flushTable writes the rows of the table the state changed to PostgreSQL on the connection: the
// deleted rows, then the updated ones by the columns they changed, then the inserted ones. Each
// statement takes a column of values as an array of text. A row whose unique key changed is
// deleted and inserted again: PostgreSQL checks a unique key at each row an update writes, so
// rows that swap their keys would fail one update.
func flushTable(ctx context.Context, c *stateConn, ts *tableState) error {
	if ts.rows.len() == 0 {
		return nil
	}
	m := ts.meta
	pks := make([]string, 0, ts.rows.len())
	ts.rows.each(func(pk string, _ *rowChange) bool {
		pks = append(pks, pk)
		return true
	})
	slices.Sort(pks)
	pkIdx := make([]int, len(m.pk))
	for j, col := range m.pk {
		pkIdx[j] = m.colIndex(col)
	}
	uniqueCol := make([]bool, len(m.cols))
	for _, cols := range m.uniques {
		for _, col := range cols {
			uniqueCol[m.colIndex(col)] = true
		}
	}
	var deletes, inserts []stateRow
	updates := make(map[string][]stateRow)
	var sets []string
	for _, pk := range pks {
		rc, _ := ts.rows.load(pk)
		switch {
		case rc.base == nil && rc.row == nil:
		case rc.row == nil:
			deletes = append(deletes, rc.base)
		case rc.base == nil:
			inserts = append(inserts, rc.row)
		default:
			var cols []string
			rekey := false
			for i := range m.cols {
				if !sameValue(rc.base[i], rc.row[i]) {
					cols = append(cols, strconv.Itoa(i))
					rekey = rekey || uniqueCol[i]
				}
			}
			if len(cols) == 0 {
				continue
			}
			if rekey {
				deletes = append(deletes, rc.base)
				inserts = append(inserts, rc.row)
				continue
			}
			key := strings.Join(cols, ",")
			if _, ok := updates[key]; !ok {
				sets = append(sets, key)
			}
			updates[key] = append(updates[key], rc.row)
		}
	}
	if len(deletes) > 0 {
		if err := flushDelete(ctx, c, m, pkIdx, deletes); err != nil {
			return err
		}
		metrics.StateFlushRows.WithLabelValues("delete").Add(float64(len(deletes)))
	}
	for _, key := range sets {
		var cols []int
		for _, s := range strings.Split(key, ",") {
			i, _ := strconv.Atoi(s)
			cols = append(cols, i)
		}
		if err := flushUpdate(ctx, c, m, pkIdx, cols, updates[key]); err != nil {
			return err
		}
		metrics.StateFlushRows.WithLabelValues("update").Add(float64(len(updates[key])))
	}
	if len(inserts) > 0 {
		all := make([]int, len(m.cols))
		for i := range all {
			all[i] = i
		}
		if err := flushInsert(ctx, c, m, all, inserts); err != nil {
			return err
		}
		metrics.StateFlushRows.WithLabelValues("insert").Add(float64(len(inserts)))
	}
	return nil
}

func sameValue(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// unnest writes the unnest of the columns of the rows, from the parameter $1, and gives its
// arguments
func unnest(b *strings.Builder, m *tableMeta, cols []int, rows []stateRow) []any {
	args := make([]any, len(cols))
	b.WriteString("unnest(")
	for k, i := range cols {
		if k > 0 {
			b.WriteString(", ")
		}
		b.WriteString("$")
		b.WriteString(strconv.Itoa(k + 1))
		b.WriteString("::text[]::")
		b.WriteString(quoteIdent(m.cols[i].typ.name))
		b.WriteString("[]")
		vals := make([]*string, len(rows))
		for r, row := range rows {
			vals[r] = row[i]
		}
		args[k] = vals
	}
	b.WriteString(") AS u__(")
	for k := range cols {
		if k > 0 {
			b.WriteString(", ")
		}
		b.WriteString("c")
		b.WriteString(strconv.Itoa(k))
	}
	b.WriteString(")")
	return args
}

// pkMatch writes the condition of equality of the primary key of the table with the first
// columns of the unnest
func pkMatch(b *strings.Builder, m *tableMeta, pkIdx []int) {
	for k, i := range pkIdx {
		if k > 0 {
			b.WriteString(" AND ")
		}
		b.WriteString(quoteIdent(m.name))
		b.WriteString(".")
		b.WriteString(quoteIdent(m.cols[i].name))
		b.WriteString(" = u__.c")
		b.WriteString(strconv.Itoa(k))
	}
}

func flushExec(ctx context.Context, c *stateConn, query string, args []any) error {
	_, err := c.pg.Conn().Exec(ctx, query, args...)
	return err
}

func flushDelete(ctx context.Context, c *stateConn, m *tableMeta, pkIdx []int, rows []stateRow) error {
	var b strings.Builder
	b.WriteString("DELETE FROM ")
	b.WriteString(quoteIdent(m.name))
	b.WriteString(" USING ")
	args := unnest(&b, m, pkIdx, rows)
	b.WriteString(" WHERE ")
	pkMatch(&b, m, pkIdx)
	return flushExec(ctx, c, b.String(), args)
}

func flushUpdate(ctx context.Context, c *stateConn, m *tableMeta, pkIdx, cols []int, rows []stateRow) error {
	var b strings.Builder
	b.WriteString("UPDATE ")
	b.WriteString(quoteIdent(m.name))
	b.WriteString(" SET ")
	for k, i := range cols {
		if k > 0 {
			b.WriteString(", ")
		}
		b.WriteString(quoteIdent(m.cols[i].name))
		b.WriteString(" = u__.c")
		b.WriteString(strconv.Itoa(len(pkIdx) + k))
	}
	b.WriteString(" FROM ")
	args := unnest(&b, m, append(slices.Clone(pkIdx), cols...), rows)
	b.WriteString(" WHERE ")
	pkMatch(&b, m, pkIdx)
	return flushExec(ctx, c, b.String(), args)
}

// flushInsert copies the rows to the table: COPY reads the text of the values with the input
// functions of the types of the columns, as a cast of the text does
func flushInsert(ctx context.Context, c *stateConn, m *tableMeta, cols []int, rows []stateRow) error {
	var b strings.Builder
	b.WriteString("COPY ")
	b.WriteString(quoteIdent(m.name))
	b.WriteString(" (")
	for k, i := range cols {
		if k > 0 {
			b.WriteString(", ")
		}
		b.WriteString(quoteIdent(m.cols[i].name))
	}
	b.WriteString(") FROM STDIN")
	var data bytes.Buffer
	for _, row := range rows {
		for k, i := range cols {
			if k > 0 {
				data.WriteByte('\t')
			}
			copyText(&data, row[i])
		}
		data.WriteByte('\n')
	}
	_, err := c.pg.Conn().PgConn().CopyFrom(ctx, &data, b.String())
	return err
}

// copyText writes a value in the text format of COPY: \N for NULL, and the backslash and the
// characters that end a value or a line escaped
func copyText(w *bytes.Buffer, v *string) {
	if v == nil {
		w.WriteString(`\N`)
		return
	}
	s := *v
	for i := 0; i < len(s); i++ {
		switch ch := s[i]; ch {
		case '\\':
			w.WriteString(`\\`)
		case '\n':
			w.WriteString(`\n`)
		case '\r':
			w.WriteString(`\r`)
		case '\t':
			w.WriteString(`\t`)
		default:
			w.WriteByte(ch)
		}
	}
}
