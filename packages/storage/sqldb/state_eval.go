/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql/driver"
	"math/big"
	"regexp"
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
	oids := map[string]uint32{
		"int2": pgtype.Int2OID, "int4": pgtype.Int4OID, "int8": pgtype.Int8OID, "numeric": pgtype.NumericOID,
		"text": pgtype.TextOID, "varchar": pgtype.VarcharOID, "bool": pgtype.BoolOID, "bytea": pgtype.ByteaOID,
		"jsonb": pgtype.JSONBOID, "timestamp": pgtype.TimestampOID, "timestamptz": pgtype.TimestamptzOID,
		"uuid": pgtype.UUIDOID,
	}
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
	t := newColType(oids[name], name, typmod)
	return t, t.supported()
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
func convert(v tval, t colType) (tval, bool) {
	if v.s == nil {
		return tval{t: t}, true
	}
	s := *v.s
	switch {
	case v.t.kind == t.kind && t.kind != kOther:
	case v.t.numberLike() && t.numberLike():
		if t.kind == kInt && v.t.kind == kNumeric {
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
	out, ok := t.inputText(s)
	if !ok {
		return tval{}, false
	}
	return tval{t: t, s: out}, true
}

// binder binds the values of the placeholders of a statement to their types
type binder struct {
	args []driver.NamedValue
	// types are the names of the types the placeholders are bound to, by placeholder, "" for one
	// not bound yet; in small for the statements of few placeholders
	types []string
	small [8]string
	row   *tableMeta
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
	s, ok := t.input(b.args[n].Value)
	return tval{t: t, s: s}, ok
}

// bexpr is a bound expression: a constant, a column of a row, or an operation on bound expressions
type bexpr struct {
	op   exprOp
	v    tval
	col  int
	t    colType
	kids []*bexpr
}

func constant(v tval) *bexpr { return &bexpr{op: eString, v: v, t: v.t} }

// bindExpr binds an expression whose value goes to the type want; a placeholder alone takes it
func (b *binder) bindExpr(e *exprNode, want colType) (*bexpr, bool) {
	switch e.op {
	case eParam:
		v, ok := b.param(e.n, want)
		if !ok {
			return nil, false
		}
		return constant(v), true
	case eString:
		if !want.supported() {
			return nil, false
		}
		s, ok := want.base().inputText(e.text)
		if !ok {
			return nil, false
		}
		return constant(tval{t: want.base(), s: s}), true
	case eNumber:
		t := numberType(e.text)
		s, ok := t.inputText(e.text)
		if !ok {
			return nil, false
		}
		return constant(tval{t: t, s: s}), true
	case eNull:
		return constant(tval{t: typeUnknown}), true
	case eBool:
		s := boolText(e.text == "true")
		return constant(tval{t: typeBool, s: &s}), true
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
			s, ok := t.inputText(kid.text)
			if !ok {
				return nil, false
			}
			return constant(tval{t: t, s: s}), true
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
			return constant(v), true
		}
		return &bexpr{op: eCast, t: t, kids: []*bexpr{k}}, true
	case eDecode:
		k, ok := b.bindExpr(e.kids[0], typeText)
		if !ok || k.op != eString || k.t.kind != kText {
			return nil, false
		}
		if k.v.s == nil {
			return constant(tval{t: typeBytea}), true
		}
		raw, ok := decodeHex(*k.v.s)
		if !ok {
			return nil, false
		}
		s := byteaText(raw)
		return constant(tval{t: typeBytea, s: &s}), true
	case eToTimestamp:
		kid := e.kids[0]
		var text string
		switch kid.op {
		case eParam:
			if n := kid.n; n < len(b.args) {
				if b.args[n].Value == nil {
					return constant(tval{t: typeTimestamptz}), true
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
		return constant(tval{t: typeTimestamptz, s: &s}), true
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
			return constant(v), true
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
	case eAdd, eSub:
		a, ok := x.kids[0].eval(r)
		if !ok {
			return tval{}, false
		}
		b, ok := x.kids[1].eval(r)
		if !ok {
			return tval{}, false
		}
		if a.s == nil || b.s == nil {
			return tval{t: x.t}, true
		}
		if x.t.kind == kInt {
			p, err1 := strconv.ParseInt(*a.s, 10, 64)
			q, err2 := strconv.ParseInt(*b.s, 10, 64)
			if err1 != nil || err2 != nil {
				return tval{}, false
			}
			if x.op == eSub {
				q = -q
			}
			sum := new(big.Int).Add(big.NewInt(p), big.NewInt(q))
			if !sum.IsInt64() {
				return tval{}, false
			}
			s, ok := x.t.inputText(sum.String())
			return tval{t: x.t, s: s}, ok
		}
		p, ok1 := parseNumeric(*a.s)
		q, ok2 := parseNumeric(*b.s)
		if !ok1 || !ok2 {
			return tval{}, false
		}
		if x.op == eSub {
			q = q.neg()
		}
		s := p.add(q).String()
		return tval{t: x.t, s: &s}, true
	case eConcat:
		a, ok := x.kids[0].eval(r)
		if !ok {
			return tval{}, false
		}
		b, ok := x.kids[1].eval(r)
		if !ok {
			return tval{}, false
		}
		if a.s == nil || b.s == nil {
			return tval{t: x.t}, true
		}
		s, ok := concatJSONB(*a.s, *b.s)
		return tval{t: x.t, s: &s}, ok
	}
	return tval{}, false
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
}

// bindCond binds a condition on the columns of the table
func (b *binder) bindCond(c *condNode) (*bcond, bool) {
	switch c.op {
	case cAnd, cOr:
		x := &bcond{op: c.op}
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
	x := &bcond{op: c.op, col: i, cmp: c.cmp}
	if c.op == cIsNull || c.op == cNotNull {
		return x, true
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
		if v.s != nil && fam == kNumeric && v.t.kind == kNumeric {
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
func (c *bcond) equalities() map[int]*string {
	if c == nil {
		return nil
	}
	terms := []*bcond{c}
	if c.op == cAnd {
		terms = c.kids
	}
	eq := make(map[int]*string)
	for _, t := range terms {
		if t.op == cCmp && t.cmp == "=" && t.vals[0] != nil {
			eq[t.col] = t.vals[0]
		} else if t.op == cIn && len(t.vals) == 1 && t.vals[0] != nil {
			eq[t.col] = t.vals[0]
		}
	}
	return eq
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
