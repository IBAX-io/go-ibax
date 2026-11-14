/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql/driver"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// The state of a block evaluates the conditions and expressions of its plans on the text of the
// values. It binds the values of the placeholders to the types PostgreSQL infers for them, as
// PostgreSQL converts them before it runs a statement. Whatever it cannot evaluate exactly as
// PostgreSQL would, an error of PostgreSQL included, it does not bind, and the statement then
// runs in PostgreSQL, which gives the result or the error.

// tval is a value of a type: the text PostgreSQL outputs for it, nil for NULL
type tval struct {
	t colType
	s *string
}

var (
	typeInt2        = newColType(pgtype.Int2OID, "int2", -1)
	typeInt4        = newColType(pgtype.Int4OID, "int4", -1)
	typeInt8        = newColType(pgtype.Int8OID, "int8", -1)
	typeNumeric     = newColType(pgtype.NumericOID, "numeric", -1)
	typeText        = newColType(pgtype.TextOID, "text", -1)
	typeBool        = newColType(pgtype.BoolOID, "bool", -1)
	typeBytea       = newColType(pgtype.ByteaOID, "bytea", -1)
	typeJSONB       = newColType(pgtype.JSONBOID, "jsonb", -1)
	typeTimestamptz = newColType(pgtype.TimestamptzOID, "timestamptz", -1)
	// typeUnknown is the type of a NULL with no type
	typeUnknown = colType{name: "unknown", length: -1, precision: -1, scale: -1}
)

// namedType is the type of a cast
func namedType(name string, mods []int) (colType, bool) {
	typmod := int32(-1)
	switch {
	case len(mods) == 0:
	case name == "numeric" && len(mods) <= 2:
		p, s := mods[0], 0
		if len(mods) == 2 {
			s = mods[1]
		}
		if p < 1 || p > 1000 || s < 0 || s > p {
			return colType{}, false
		}
		typmod = int32(p<<16|s) + 4
	case name == "varchar" && len(mods) == 1:
		if mods[0] < 1 {
			return colType{}, false
		}
		typmod = int32(mods[0]) + 4
	default:
		return colType{}, false
	}
	t := newColType(namedOIDs[name], name, typmod)
	return t, t.supported()
}

// namedOIDs are the oids of the types a cast names
var namedOIDs = map[string]uint32{
	"int2": pgtype.Int2OID, "int4": pgtype.Int4OID, "int8": pgtype.Int8OID, "numeric": pgtype.NumericOID,
	"text": pgtype.TextOID, "varchar": pgtype.VarcharOID, "bool": pgtype.BoolOID, "bytea": pgtype.ByteaOID,
	"jsonb": pgtype.JSONBOID, "timestamp": pgtype.TimestampOID, "timestamptz": pgtype.TimestamptzOID,
	"uuid": pgtype.UUIDOID,
}

// base is the type without its modifiers: the type of a placeholder PostgreSQL infers from it
func (t colType) base() colType { return newColType(t.oid, t.name, -1) }

func (t colType) numberLike() bool { return t.kind == kInt || t.kind == kNumeric }

func (t colType) textLike() bool { return t.kind == kText || t.kind == kVarchar || t.kind == kBpchar }

// numberType is the type of a number literal: int4, int8 or numeric by its value
func numberType(s string) colType {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n >= -1<<31 && n < 1<<31 {
			return typeInt4
		}
		return typeInt8
	}
	return typeNumeric
}

// convert is the value v of a type as a value of the type t, as an assignment or a cast converts
// it; false for a conversion the state does not make, or one that fails
func convert(v tval, t colType) (tval, bool) { return convertInto(v, t, nil) }

// convertInto is convert, writing a text the conversion changes into buf: a new text when nil
func convertInto(v tval, t colType, buf *string) (tval, bool) {
	if v.s == nil {
		return tval{t: t}, true
	}
	s := *v.s
	switch {
	case v.t.kind == t.kind && t.kind != kOther:
	case v.t.numberLike() && t.numberLike():
		if t.kind == kInt && v.t.kind == kNumeric && !canonInt(s) {
			// numeric to an integer rounds half away from zero
			n, ok := parseNumeric(s)
			if !ok {
				return tval{}, false
			}
			s = n.rescale(0).String()
		}
	case v.t.textLike() && t.textLike():
		if v.t.kind == kBpchar {
			s = strings.TrimRight(s, " ")
		}
	case v.t.kind == kJSONB && t.textLike():
	case v.t.textLike() && t.kind == kJSONB:
	case v.t.kind == kTimestamptz && t.kind == kTimestamp:
		s = strings.TrimSuffix(s, "+00")
	case v.t.kind == kTimestamp && t.kind == kTimestamptz:
	case v.t.kind == kOther && v.t.name == "unknown":
	default:
		return tval{}, false
	}
	out, ok := t.inputValue(s)
	if !ok {
		return tval{}, false
	}
	if out == *v.s {
		// the text stays as it is: the value keeps its text
		return tval{t: t, s: v.s}, true
	}
	if buf == nil {
		buf = new(string)
	}
	*buf = out
	return tval{t: t, s: buf}, true
}

// binder binds the values of the placeholders of a statement to their types
type binder struct {
	args []driver.NamedValue
	// types are the names of the types the placeholders are bound to, by placeholder, "" for one
	// not bound yet; in small for the statements of few placeholders
	types []string
	small [8]string
	row   *tableMeta
	// strs, exprs and conds hold the texts, expressions and conditions the binder makes, a chunk
	// at a time
	strs  []string
	exprs []bexpr
	conds []bcond
}

// chunk is the number of texts, expressions or conditions a binder makes at a time: as many as
// the statement has values, up to 8
func (b *binder) chunk() int { return min(max(len(b.args), 2), 8) }

// str is a text the binder keeps
func (b *binder) str(s string) *string {
	if len(b.strs) == cap(b.strs) {
		b.strs = make([]string, 0, b.chunk())
	}
	b.strs = append(b.strs, s)
	return &b.strs[len(b.strs)-1]
}

// constant is a constant expression the binder keeps
func (b *binder) constant(v tval) *bexpr {
	if len(b.exprs) == cap(b.exprs) {
		b.exprs = make([]bexpr, 0, b.chunk())
	}
	b.exprs = append(b.exprs, bexpr{op: eString, v: v, t: v.t})
	return &b.exprs[len(b.exprs)-1]
}

// cond is a condition the binder keeps
func (b *binder) cond(c bcond) *bcond {
	if len(b.conds) == cap(b.conds) {
		b.conds = make([]bcond, 0, b.chunk())
	}
	b.conds = append(b.conds, c)
	return &b.conds[len(b.conds)-1]
}

// text is the text the type stores of a text given to its input function, kept by the binder
func (b *binder) text(t colType, s string) (*string, bool) {
	out, ok := t.inputValue(s)
	if !ok {
		return nil, false
	}
	return b.str(out), true
}

// bindType binds the placeholder n to the type name, false if it is bound to another
func (b *binder) bindType(n int, name string) bool {
	if b.types == nil {
		if len(b.args) <= len(b.small) {
			b.types = b.small[:len(b.args)]
		} else {
			b.types = make([]string, len(b.args))
		}
	}
	if prev := b.types[n]; prev != "" && prev != name {
		return false
	}
	b.types[n] = name
	return true
}

// param is the value of the placeholder n bound to the type t. A placeholder has one type, which
// PostgreSQL infers where the statement first uses it; the state takes no statement that uses a
// placeholder with two.
func (b *binder) param(n int, t colType) (tval, bool) {
	t = t.base()
	if !t.supported() || n >= len(b.args) {
		return tval{}, false
	}
	if !b.bindType(n, t.name) {
		return tval{}, false
	}
	s, null, ok := t.inputArg(b.args[n].Value)
	if !ok || null {
		return tval{t: t}, ok
	}
	return tval{t: t, s: b.str(s)}, true
}

// bexpr is a bound expression: a constant, a column of a row, or an operation on bound expressions
type bexpr struct {
	op   exprOp
	v    tval
	col  int
	t    colType
	kids []*bexpr
}

// bindExpr binds an expression whose value goes to the type want; a placeholder alone takes it
func (b *binder) bindExpr(e *exprNode, want colType) (*bexpr, bool) {
	switch e.op {
	case eParam:
		v, ok := b.param(e.n, want)
		if !ok {
			return nil, false
		}
		return b.constant(v), true
	case eString:
		if !want.supported() {
			return nil, false
		}
		s, ok := b.text(want.base(), e.text)
		if !ok {
			return nil, false
		}
		return b.constant(tval{t: want.base(), s: s}), true
	case eNumber:
		t := numberType(e.text)
		s, ok := b.text(t, e.text)
		if !ok {
			return nil, false
		}
		return b.constant(tval{t: t, s: s}), true
	case eNull:
		return b.constant(tval{t: typeUnknown}), true
	case eBool:
		return b.constant(tval{t: typeBool, s: b.str(boolText(e.text == "true"))}), true
	case eCol:
		if b.row == nil {
			return nil, false
		}
		i := b.row.colIndex(e.text)
		if i < 0 {
			return nil, false
		}
		return &bexpr{op: eCol, col: i, t: b.row.cols[i].typ}, true
	case eCast:
		t, ok := namedType(e.text, e.mods)
		if !ok {
			return nil, false
		}
		kid := e.kids[0]
		if kid.op == eString {
			// a literal takes the type of its cast
			s, ok := b.text(t, kid.text)
			if !ok {
				return nil, false
			}
			return b.constant(tval{t: t, s: s}), true
		}
		k, ok := b.bindExpr(kid, t)
		if !ok {
			return nil, false
		}
		if k.op == eString {
			v, ok := convert(k.v, t)
			if !ok {
				return nil, false
			}
			return b.constant(v), true
		}
		return &bexpr{op: eCast, t: t, kids: []*bexpr{k}}, true
	case eDecode:
		k, ok := b.bindExpr(e.kids[0], typeText)
		if !ok || k.op != eString || k.t.kind != kText {
			return nil, false
		}
		if k.v.s == nil {
			return b.constant(tval{t: typeBytea}), true
		}
		raw, ok := decodeHex(*k.v.s)
		if !ok {
			return nil, false
		}
		s := byteaText(raw)
		return b.constant(tval{t: typeBytea, s: &s}), true
	case eToTimestamp:
		kid := e.kids[0]
		var text string
		switch kid.op {
		case eParam:
			if n := kid.n; n < len(b.args) {
				if b.args[n].Value == nil {
					return b.constant(tval{t: typeTimestamptz}), true
				}
				s, binary, ok := argText(b.args[n].Value)
				if !ok || binary {
					return nil, false
				}
				if !b.bindType(n, "float8") {
					return nil, false
				}
				text = s
			} else {
				return nil, false
			}
		case eNumber:
			text = kid.text
		default:
			return nil, false
		}
		if !floatRe.MatchString(text) {
			return nil, false
		}
		tm, ok := unixTimestamp(text)
		if !ok {
			return nil, false
		}
		s := typeTimestamptz.timestampText(tm)
		return b.constant(tval{t: typeTimestamptz, s: &s}), true
	case eAdd, eSub, eConcat:
		l, r := e.kids[0], e.kids[1]
		var left, right *bexpr
		var ok bool
		// a placeholder takes the type of the other operand
		if l.op == eParam || l.op == eString {
			if right, ok = b.bindExpr(r, typeUnknown); !ok {
				return nil, false
			}
			if left, ok = b.bindExpr(l, right.t); !ok {
				return nil, false
			}
		} else {
			if left, ok = b.bindExpr(l, typeUnknown); !ok {
				return nil, false
			}
			if right, ok = b.bindExpr(r, left.t); !ok {
				return nil, false
			}
		}
		x := &bexpr{op: e.op, kids: []*bexpr{left, right}}
		switch {
		case e.op == eConcat && left.t.kind == kJSONB && right.t.kind == kJSONB:
			x.t = typeJSONB
		case e.op != eConcat && left.t.numberLike() && right.t.numberLike():
			x.t = typeNumeric
			if left.t.kind == kInt && right.t.kind == kInt {
				x.t = left.t
				if right.t.bits > left.t.bits {
					x.t = right.t
				}
			}
		default:
			return nil, false
		}
		if left.op == eString && right.op == eString {
			v, ok := x.eval(nil)
			if !ok {
				return nil, false
			}
			return b.constant(v), true
		}
		return x, true
	}
	return nil, false
}

// floatRe is the text of a float8 the state reads as PostgreSQL does
var floatRe = regexp.MustCompile(`^\s*[-+]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][-+]?[0-9]+)?\s*$`)

// eval is the value of the expression for a row; false when the state cannot evaluate it
func (x *bexpr) eval(r stateRow) (tval, bool) {
	switch x.op {
	case eString:
		return x.v, true
	case eCol:
		return tval{t: x.t, s: r[x.col]}, true
	case eCast:
		v, ok := x.kids[0].eval(r)
		if !ok {
			return tval{}, false
		}
		return convert(v, x.t)
	case eAdd, eSub, eConcat:
		a, ok := x.kids[0].eval(r)
		if !ok {
			return tval{}, false
		}
		b, ok := x.kids[1].eval(r)
		if !ok {
			return tval{}, false
		}
		return fold(x.op, x.t, a, b)
	}
	return tval{}, false
}

// fold is the value of the operation op of the type t on the values a and b; false when the
// state cannot evaluate it
func fold(op exprOp, t colType, a, b tval) (tval, bool) {
	if a.s == nil || b.s == nil {
		return tval{t: t}, true
	}
	if op == eConcat {
		s, ok := concatJSONB(*a.s, *b.s)
		return tval{t: t, s: &s}, ok
	}
	if t.kind == kInt {
		p, err1 := strconv.ParseInt(*a.s, 10, 64)
		q, err2 := strconv.ParseInt(*b.s, 10, 64)
		if err1 != nil || err2 != nil {
			return tval{}, false
		}
		if op == eSub {
			q = -q
		}
		sum, ok := addInt64Checked(p, q)
		if !ok {
			return tval{}, false
		}
		s, ok := t.inputText(strconv.FormatInt(sum, 10))
		return tval{t: t, s: s}, ok
	}
	if s, ok := addInt64(*a.s, *b.s, op == eSub); ok {
		return tval{t: t, s: &s}, true
	}
	p, ok1 := parseNumeric(*a.s)
	q, ok2 := parseNumeric(*b.s)
	if !ok1 || !ok2 {
		return tval{}, false
	}
	if op == eSub {
		q = q.neg()
	}
	s := p.add(q).String()
	return tval{t: t, s: &s}, true
}

// tri is a value of the logic of SQL: true, false or NULL
type tri int8

const (
	triFalse tri = iota
	triTrue
	triNull
)

// bcond is a bound condition: on the columns of the rows of a table, with the values it compares
// them with as the text of the type of the comparison
type bcond struct {
	op   condOp
	kids []*bcond
	col  int
	cmp  string
	// fam is the kind of the comparison: kNumeric for numbers of any type
	fam  colKind
	vals []*string
	// one holds the value of a condition of one
	one [1]*string
}

// bindCond binds a condition on the columns of the table
func (b *binder) bindCond(c *condNode) (*bcond, bool) {
	switch c.op {
	case cAnd, cOr:
		x := b.cond(bcond{op: c.op, kids: make([]*bcond, 0, len(c.kids))})
		for _, k := range c.kids {
			bk, ok := b.bindCond(k)
			if !ok {
				return nil, false
			}
			x.kids = append(x.kids, bk)
		}
		return x, true
	}
	i := b.row.colIndex(c.col)
	if i < 0 {
		return nil, false
	}
	ct := b.row.cols[i].typ
	x := b.cond(bcond{op: c.op, col: i, cmp: c.cmp})
	if c.op == cIsNull || c.op == cNotNull {
		return x, true
	}
	if len(c.vals) == 1 {
		x.vals = x.one[:0:1]
	}
	for _, e := range c.vals {
		be, ok := b.bindExpr(e, ct)
		if !ok || be.op != eString {
			return nil, false
		}
		v := be.v
		fam, ok := family(ct, v.t)
		if !ok {
			return nil, false
		}
		if c.op == cCmp && c.cmp != "=" && c.cmp != "<>" && !ordered(fam) {
			return nil, false
		}
		if len(x.vals) > 0 && x.fam != fam {
			return nil, false
		}
		x.fam = fam
		if v.s != nil && fam == kNumeric && v.t.kind == kNumeric && !canonInt(*v.s) {
			n, _ := parseNumeric(*v.s)
			s := n.normal()
			v.s = &s
		}
		x.vals = append(x.vals, v.s)
	}
	return x, true
}

// family is the kind of the comparison of a column of the type c with a value of the type v;
// false for a comparison the state does not make
func family(c, v colType) (colKind, bool) {
	switch {
	case c.numberLike() && v.numberLike():
		return kNumeric, true
	case c.kind == kBpchar || v.kind == kBpchar:
		if c.kind == v.kind {
			return kBpchar, true
		}
		return 0, false
	case c.textLike() && v.textLike():
		return kText, true
	case c.kind == v.kind:
		switch c.kind {
		case kBool, kBytea, kTimestamp, kTimestamptz, kUUID:
			return c.kind, true
		}
	}
	return 0, false
}

// ordered tells the state orders the values of a kind as PostgreSQL does; text has the order of
// the collation of the database, which it leaves to PostgreSQL
func ordered(k colKind) bool {
	switch k {
	case kNumeric, kInt, kBool, kBytea, kTimestamp, kTimestamptz, kUUID:
		return true
	}
	return false
}

// compareText compares two values of a kind of comparison
func compareText(fam colKind, a, b string) int {
	switch fam {
	case kNumeric, kInt:
		if x, err := strconv.ParseInt(a, 10, 64); err == nil {
			if y, err := strconv.ParseInt(b, 10, 64); err == nil {
				switch {
				case x < y:
					return -1
				case x > y:
					return 1
				}
				return 0
			}
		}
		x, _ := parseNumeric(a)
		y, _ := parseNumeric(b)
		if x.v == nil || y.v == nil {
			return strings.Compare(a, b)
		}
		return x.add(y.neg()).sign()
	case kBpchar:
		return strings.Compare(strings.TrimRight(a, " "), strings.TrimRight(b, " "))
	}
	// the text of bool, bytea in hex, timestamps and uuids orders as their values do
	return strings.Compare(a, b)
}

// eval is the value of the condition for a row
func (c *bcond) eval(r stateRow) tri {
	switch c.op {
	case cAnd:
		res := triTrue
		for _, k := range c.kids {
			switch k.eval(r) {
			case triFalse:
				return triFalse
			case triNull:
				res = triNull
			}
		}
		return res
	case cOr:
		res := triFalse
		for _, k := range c.kids {
			switch k.eval(r) {
			case triTrue:
				return triTrue
			case triNull:
				res = triNull
			}
		}
		return res
	}
	v := r[c.col]
	switch c.op {
	case cIsNull:
		return triOf(v == nil)
	case cNotNull:
		return triOf(v != nil)
	}
	if v == nil {
		return triNull
	}
	if c.op == cCmp {
		if c.vals[0] == nil {
			return triNull
		}
		d := compareText(c.fam, *v, *c.vals[0])
		switch c.cmp {
		case "=":
			return triOf(d == 0)
		case "<>":
			return triOf(d != 0)
		case "<":
			return triOf(d < 0)
		case "<=":
			return triOf(d <= 0)
		case ">":
			return triOf(d > 0)
		}
		return triOf(d >= 0)
	}
	// IN and NOT IN
	res := triFalse
	for _, x := range c.vals {
		if x == nil {
			res = triNull
		} else if compareText(c.fam, *v, *x) == 0 {
			res = triTrue
			break
		}
	}
	if c.op == cNotIn {
		switch res {
		case triTrue:
			return triFalse
		case triFalse:
			return triTrue
		}
	}
	return res
}

func triOf(b bool) tri {
	if b {
		return triTrue
	}
	return triFalse
}

// matches tells a row holds a condition; a nil condition holds for every row
func (c *bcond) matches(r stateRow) bool { return c == nil || c.eval(r) == triTrue }

// equalities are the values a condition requires of columns: col = value terms of its top
// conjunction
func (c *bcond) equalities() eqSet {
	var eq eqSet
	if c == nil {
		return eq
	}
	add := func(t *bcond) {
		if t.op == cCmp && t.cmp == "=" && t.vals[0] != nil {
			eq.set(t.col, t.vals[0])
		} else if t.op == cIn && len(t.vals) == 1 && t.vals[0] != nil {
			eq.set(t.col, t.vals[0])
		}
	}
	if c.op == cAnd {
		for _, t := range c.kids {
			add(t)
		}
	} else {
		add(c)
	}
	return eq
}

// eqSet is the value a condition requires of each column, by equality; the last for a column
// required twice
type eqSet struct {
	n    int
	cols [8]int
	vals [8]*string
	more []eqCol
}

type eqCol struct {
	col int
	v   *string
}

func (e *eqSet) set(col int, v *string) {
	if e.n < len(e.cols) {
		e.cols[e.n], e.vals[e.n] = col, v
	} else {
		e.more = append(e.more, eqCol{col, v})
	}
	e.n++
}

// get is the value required of the column; false for none
func (e *eqSet) get(col int) (*string, bool) {
	for i := len(e.more) - 1; i >= 0; i-- {
		if e.more[i].col == col {
			return e.more[i].v, true
		}
	}
	for i := min(e.n, len(e.cols)) - 1; i >= 0; i-- {
		if e.cols[i] == col {
			return e.vals[i], true
		}
	}
	return nil, false
}

// border is an order of rows by columns, bound to the table
type border struct {
	col        int
	kind       colKind
	desc       bool
	nullsFirst bool
}

// bindOrder binds an ORDER BY; false for an order of a type the state does not order
func bindOrder(m *tableMeta, items []orderItem) ([]border, bool) {
	var out []border
	for _, it := range items {
		i := m.colIndex(it.col)
		if i < 0 {
			return nil, false
		}
		k := m.cols[i].typ.kind
		if !ordered(k) {
			return nil, false
		}
		if k == kInt {
			k = kNumeric
		}
		out = append(out, border{col: i, kind: k, desc: it.desc, nullsFirst: it.nullsFirst})
	}
	return out, true
}

// compareRows compares two rows by an order, and then by the primary key, which the state takes
// for the order of rows PostgreSQL leaves undefined
func compareRows(order []border, pk []int, kinds []colKind, a, b stateRow) int {
	for _, o := range order {
		x, y := a[o.col], b[o.col]
		var d int
		switch {
		case x == nil && y == nil:
			continue
		case x == nil:
			d = 1
			if o.nullsFirst {
				d = -1
			}
			return d
		case y == nil:
			d = -1
			if o.nullsFirst {
				d = 1
			}
			return d
		}
		d = compareText(o.kind, *x, *y)
		if o.desc {
			d = -d
		}
		if d != 0 {
			return d
		}
	}
	for j, i := range pk {
		x, y := a[i], b[i]
		if x == nil || y == nil {
			continue
		}
		if d := compareText(kinds[j], *x, *y); d != 0 {
			return d
		}
	}
	return 0
}

// planBinding is what binding the values of a plan to a table makes: the condition, the order
// and the bounds of a select, the row of an insert and the assignments of an update
type planBinding struct {
	cond          *bcond
	order         []border
	limit, offset int64
	row           stateRow
	sets          []assign
}

// assign is the assignment of an update of a column
type assign struct {
	col  int
	expr *bexpr
}

// bindPlan binds the values args to the plan p of a statement of the table m with the binder, as
// the state runs it: the reference of what the program of the plan makes; false when the state
// does not run the statement with the values
func bindPlan(p *stmt, m *tableMeta, args []driver.NamedValue) (planBinding, bool) {
	out := planBinding{limit: -1}
	b := &binder{args: args, row: m}
	if p.where != nil {
		var ok bool
		if out.cond, ok = b.bindCond(p.where); !ok {
			return planBinding{}, false
		}
	}
	switch p.kind {
	case stSelect:
		var ok bool
		if out.order, ok = bindOrder(m, p.order); !ok {
			return planBinding{}, false
		}
		if out.limit, ok = b.count(p.limit); !ok {
			return planBinding{}, false
		}
		if out.offset, ok = b.count(p.offset); !ok {
			return planBinding{}, false
		}
		if out.offset < 0 {
			out.offset = 0
		}
	case stInsert:
		row := make(stateRow, len(m.cols))
		set := make([]bool, len(m.cols))
		for k, name := range p.cols {
			i := m.colIndex(name)
			if i < 0 || set[i] {
				return planBinding{}, false
			}
			set[i] = true
			col := m.cols[i]
			e, ok := b.bindExpr(p.vals[k], col.typ)
			if !ok {
				return planBinding{}, false
			}
			val, ok := e.eval(nil)
			if !ok {
				return planBinding{}, false
			}
			if val, ok = convert(val, col.typ); !ok {
				return planBinding{}, false
			}
			row[i] = val.s
		}
		for i, col := range m.cols {
			if set[i] || !col.hasDef {
				continue
			}
			if !col.defOK {
				return planBinding{}, false
			}
			row[i] = col.def
		}
		out.row = row
	case stUpdate:
		for _, s := range p.sets {
			i := m.colIndex(s.col)
			if i < 0 || slices.Contains(m.pk, s.col) || slices.ContainsFunc(out.sets, func(a assign) bool { return a.col == i }) {
				return planBinding{}, false
			}
			e, ok := b.bindExpr(s.expr, m.cols[i].typ)
			if !ok {
				return planBinding{}, false
			}
			out.sets = append(out.sets, assign{col: i, expr: e})
		}
	}
	return out, true
}

// count is the value of a LIMIT or OFFSET: -1 for none
func (b *binder) count(e *exprNode) (int64, bool) {
	if e == nil {
		return -1, true
	}
	var s *string
	if e.op == eNumber {
		s = &e.text
	} else {
		v, ok := b.param(e.n, typeInt8)
		if !ok {
			return 0, false
		}
		s = v.s
	}
	if s == nil {
		return -1, true
	}
	n, err := strconv.ParseInt(*s, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
