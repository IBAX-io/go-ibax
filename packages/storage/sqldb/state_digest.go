/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql/driver"
	"fmt"
	"slices"
	"strings"
)

// StatementTrace, when set, is called with each statement a transaction runs, its text and the
// values of its placeholders. Tests set it to compare the statements of two ways of running the
// same contract; a node never sets it.
var StatementTrace func(query string, args []any)

func traceNamed(query string, args []driver.NamedValue) {
	values := make([]any, len(args))
	for i, a := range args {
		values[i] = a.Value
	}
	StatementTrace(query, values)
}

// Digest is the text of what the view read and wrote: its reads, the highest ids it read, the
// rows it wrote with the rows they had before, and its undo log. Two views that ran the same
// statements on the same state have the same digest.
func (v *TxView) Digest() string {
	var b strings.Builder
	str := func(p *string) string {
		if p == nil {
			return "<nil>"
		}
		return fmt.Sprintf("%q", *p)
	}
	row := func(r stateRow) string {
		if r == nil {
			return "<none>"
		}
		parts := make([]string, len(r))
		for i, c := range r {
			parts[i] = str(c)
		}
		return "[" + strings.Join(parts, " ") + "]"
	}
	var cond func(c *bcond) string
	cond = func(c *bcond) string {
		if c == nil {
			return "<all>"
		}
		vals := make([]string, len(c.vals))
		for i, p := range c.vals {
			vals[i] = str(p)
		}
		kids := make([]string, len(c.kids))
		for i, k := range c.kids {
			kids[i] = cond(k)
		}
		return fmt.Sprintf("{op %d col %d cmp %q fam %d vals %v kids %v}", c.op, c.col, c.cmp, c.fam, vals, kids)
	}
	name := func(ts *tableState) string {
		if ts == nil {
			return "<every table>"
		}
		return ts.meta.name
	}
	fmt.Fprintf(&b, "start %d ordered %t needsOrder %t restart %t reason %q\n", v.start, v.ordered, v.needsOrder, v.restart, v.reason)
	for _, r := range v.reads {
		fmt.Fprintf(&b, "read %s seq %d point %t ix %d key %q cond %s\n", name(r.ts), r.seq, r.point, r.ix, r.key, cond(r.cond))
	}
	for _, r := range v.ids {
		fmt.Fprintf(&b, "id %s own %d value %d\n", name(r.ts), r.own, r.value)
	}
	for _, r := range v.utxo {
		fmt.Fprintf(&b, "utxo %v seq %d\n", r.key, r.seq)
	}
	for _, t := range v.order {
		vt := v.own[t]
		fmt.Fprintf(&b, "table %s\n", t)
		for _, pk := range vt.order {
			vc := vt.rows[pk]
			if vc == nil {
				fmt.Fprintf(&b, "  row %q <gone>\n", pk)
				continue
			}
			fmt.Fprintf(&b, "  row %q prev %s now %s\n", pk, row(vc.prev), row(vc.row))
		}
		for j, u := range vt.uniq {
			keys := make([]string, 0, len(u))
			for k, pk := range u {
				keys = append(keys, fmt.Sprintf("%q=%q", k, pk))
			}
			slices.Sort(keys)
			fmt.Fprintf(&b, "  uniq %d %v\n", j, keys)
		}
	}
	for _, u := range v.undo {
		fmt.Fprintf(&b, "undo %s pk %q had %t prev %s row %s order %d uniq %v\n", u.vt.ts.meta.name, u.pk, u.had,
			row(u.old.prev), row(u.old.row), u.orderLen, u.uniq)
	}
	for _, m := range v.marks {
		fmt.Fprintf(&b, "mark %q at %d pg %t\n", m.name, m.at, m.pg)
	}
	return b.String()
}
