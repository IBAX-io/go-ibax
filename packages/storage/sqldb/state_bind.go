/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unsafe"
)

// The binder binds the values of a statement to the columns of its table each time the statement
// runs. What it makes of the plan whatever the values are, the columns, the types of the
// placeholders, the constants and the checks, the program of the plan for the table makes once;
// a run of the program makes of the values only what depends on them, by instructions that write
// each value into a slot, and then the condition, the row or the assignments of the slots. The
// build tag b7check compares each run with the binder.

// bindProg is the program that binds the values of a plan to a table
type bindProg struct {
	meta *tableMeta
	// bad tells the binder binds no values to the plan; need is the number of values it reads
	bad  bool
	need int
	// consts are the texts of the slots a run starts with: the constants of the plan; types the
	// types of the values of the slots, which do not depend on the values bound
	consts []*string
	types  []colType
	ops    []bindOp
	// nstr is the number of texts the instructions make
	nstr int
	// conds are the nodes of the condition, the first its root; nkids and nvals the kids of the
	// nodes and the values of the nodes of more than one value
	conds        []condTmpl
	nkids, nvals int
	// the order, the bounds and whether the projection is of columns of the table of a select
	order         []border
	limit, offset countTmpl
	projOK        bool
	// the values of the columns of an insert, and the columns it leaves to their defaults
	cells []cellTmpl
	defs  []int
	// the assignments of an update, of the nodes of the expressions
	sets      []setTmpl
	exprs     []exprTmpl
	nexprKids int
	// frame is the runtime type of the block a run makes what it binds in, nil when it makes
	// none, and at the offsets of its parts in the block
	frame unsafe.Pointer
	at    frameAt
	// scan is the statement merge reads the rows of PostgreSQL of a select, an update or a delete
	// by, of the table, nil for an insert
	scan *scanTmpl
}

// scanTmpl is the statement of the rows of PostgreSQL of a plan: where its text of the condition,
// ordered that with the order of a bounded select, each one of each text of the table, and
// whereArgs and orderArgs the indexes of the values of their placeholders in their order
type scanTmpl struct {
	where, ordered       *string
	whereArgs, orderArgs []int
}

// scanOf is the statement of the rows of PostgreSQL of the plan p of the table m
func scanOf(p *stmt, m *tableMeta) *scanTmpl {
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
	where, whereArgs := b.String(), slices.Clip(porder)
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
	return &scanTmpl{where: m.scanText(where), ordered: m.scanText(b.String()), whereArgs: whereArgs, orderArgs: porder}
}

// scanText is the text s of a statement of the rows of PostgreSQL of the table, one of each text
func (m *tableMeta) scanText(s string) *string {
	got, _ := m.scans.LoadOrStore(s, &s)
	return got.(*string)
}

// read is the statement of the values args of a select of the bounds limit (-1 for none) and
// offset that reads extra rows of PostgreSQL the state changed: the key of its rows, whether the
// key tells its values apart, and its values; false when args lack a value
func (sc *scanTmpl) read(args []driver.NamedValue, limit, offset, extra int64) (key shapeScanKey, keep bool,
	values []any, ok bool) {
	key.text, key.n = sc.ordered, -1
	porder := sc.orderArgs
	switch {
	case limit < 0 && offset <= 0:
		key.text, porder = sc.where, sc.whereArgs
	case limit >= 0:
		key.n = limit + offset + extra
	}
	values = make([]any, len(porder))
	for i, k := range porder {
		if k >= len(args) {
			return key, false, nil, false
		}
		values[i] = args[k].Value
	}
	key.values, keep = valuesKey(values)
	return key, keep, values, true
}

// query is the text of the statement of the key
func (k shapeScanKey) query() string {
	if k.n < 0 {
		return *k.text
	}
	return *k.text + " LIMIT " + strconv.FormatInt(k.n, 10)
}

// frameAt are the offsets in the block of a run of its texts, of the values of the row and the
// nodes of the condition, of the nodes of the condition and their kids, and of the nodes of the
// expressions, their kids and the assignments
type frameAt struct {
	strs, texts, conds, ckids, xs, xkids, sets uintptr
}

type opCode uint8

const (
	// opParam is the value n bound to the type t
	opParam opCode = iota
	// opConvert is the value of the slot a as the type t
	opConvert
	// opDecode is the bytea of the hex of the slot a
	opDecode
	// opTimestamp is to_timestamp of the value n
	opTimestamp
	// opFold is the operation op of the type t on the slots a and b
	opFold
	// opNormal is the normal text of the numeric of the slot a
	opNormal
)

// bindOp is an instruction of a program: it writes the slot dst, and a text it makes into the
// text str of the run
type bindOp struct {
	code      opCode
	op        exprOp
	dst, a, b int
	n         int
	str       int
	t         colType
	// conflict tells the value of to_timestamp is bound to a type other than float8 elsewhere
	conflict bool
}

// countTmpl is a LIMIT or OFFSET: of the slot, or n (-1 for none) for slot -1
type countTmpl struct {
	slot int
	n    int64
}

// condTmpl is a node of a condition: kids are nodes, vals slots; k0 and v0 are where its kids
// and its values of a node of more than one value start in those of a run
type condTmpl struct {
	op     condOp
	col    int
	cmp    string
	fam    colKind
	kids   []int
	vals   []int
	k0, v0 int
}

// cellTmpl is the value of a column of an insert, of a slot
type cellTmpl struct{ col, slot int }

// setTmpl is the assignment of a column of an update, of a node of the expressions
type setTmpl struct{ col, node int }

// exprTmpl is a node of an expression on a row: a constant of a slot, a column, or an operation
// on nodes; k0 is where its kids start in those of a run
type exprTmpl struct {
	op   exprOp
	col  int
	slot int
	t    colType
	kids []int
	k0   int
}

// bindOf binds the values args to the plan p of a statement of the table m by the program of the
// plan for the table, which it makes when the plan has none for the table
func bindOf(p *stmt, m *tableMeta, args []driver.NamedValue) (*bindProg, planBinding, bool) {
	prog := p.prog.Load()
	if prog == nil || prog.meta != m {
		prog = compileBind(p, m)
		p.prog.Store(prog)
	}
	bd, ok := prog.run(args)
	if bindCheck {
		want, wantOK := bindPlan(p, m, args)
		if diff := diffBinding(want, wantOK, bd, ok); diff != "" {
			panic(fmt.Sprintf("b7check: the program of %+v binds %v other than the binder: %s", p, args, diff))
		}
	}
	return prog, bd, ok
}

// bindCompiler makes the program of a plan for a table as the binder binds the plan
type bindCompiler struct {
	prog *bindProg
	m    *tableMeta
	// static tells of each slot that its value is a constant of the program
	static []bool
	// types are the types the placeholders are bound to; stamps the instructions of to_timestamp
	// of a placeholder
	types  map[int]string
	stamps []int
}

// cexpr is a compiled expression: a value of the slot, or the node of an expression on a row
// for slot -1
type cexpr struct {
	t    colType
	slot int
	node int
}

// compileBind is the program of the plan p for the table m
func compileBind(p *stmt, m *tableMeta) *bindProg {
	none := countTmpl{slot: -1, n: -1}
	c := &bindCompiler{prog: &bindProg{meta: m, limit: none, offset: none}, m: m, types: map[int]string{}}
	if !c.compile(p) {
		return &bindProg{meta: m, bad: true}
	}
	prog := c.prog
	for _, i := range c.stamps {
		op := &prog.ops[i]
		if name := c.types[op.n]; name != "" && name != "float8" {
			op.conflict = true
		}
	}
	for i := range prog.conds {
		t := &prog.conds[i]
		t.k0 = prog.nkids
		prog.nkids += len(t.kids)
		if len(t.vals) > 1 {
			t.v0 = prog.nvals
			prog.nvals += len(t.vals)
		}
	}
	for i := range prog.exprs {
		t := &prog.exprs[i]
		t.k0 = prog.nexprKids
		prog.nexprKids += len(t.kids)
	}
	prog.layout()
	switch p.kind {
	case stSelect, stUpdate, stDelete:
		prog.scan = scanOf(p, m)
	}
	return prog
}

// layout makes the type of the block of a run: an array of each of its parts it has
func (prog *bindProg) layout() {
	var fields []reflect.StructField
	at := []*uintptr{}
	part := func(off *uintptr, n int, t reflect.Type) {
		if n == 0 {
			return
		}
		fields = append(fields, reflect.StructField{Name: fmt.Sprintf("P%d", len(fields)), Type: reflect.ArrayOf(n, t)})
		at = append(at, off)
	}
	nrow := 0
	if prog.cells != nil || prog.defs != nil {
		nrow = len(prog.meta.cols)
	}
	part(&prog.at.strs, prog.nstr, reflect.TypeFor[string]())
	part(&prog.at.texts, nrow+prog.nvals, reflect.TypeFor[*string]())
	part(&prog.at.conds, len(prog.conds), reflect.TypeFor[bcond]())
	part(&prog.at.ckids, prog.nkids, reflect.TypeFor[*bcond]())
	if len(prog.sets) > 0 {
		part(&prog.at.xs, len(prog.exprs), reflect.TypeFor[bexpr]())
		part(&prog.at.xkids, prog.nexprKids, reflect.TypeFor[*bexpr]())
		part(&prog.at.sets, len(prog.sets), reflect.TypeFor[assign]())
	}
	if len(fields) == 0 {
		return
	}
	t := reflect.StructOf(fields)
	for i, off := range at {
		*off = t.Field(i).Offset
	}
	prog.frame = typeOf(t)
}

// typeOf is the runtime type of t: the data word of the interface, which is the type reflect
// itself allocates by
func typeOf(t reflect.Type) unsafe.Pointer {
	return (*[2]unsafe.Pointer)(unsafe.Pointer(&t))[1]
}

// newFrame allocates a zeroed value of the runtime type t as reflect.New does, without the lookup
// of the type of a pointer to t that reflect.New makes for the Value it returns
//
//go:linkname newFrame reflect.unsafe_New
//go:noescape
func newFrame(t unsafe.Pointer) unsafe.Pointer

// framePart is the part of n values of the type T at the offset at of the block base
func framePart[T any](base unsafe.Pointer, at uintptr, n int) []T {
	if n == 0 {
		return nil
	}
	return unsafe.Slice((*T)(unsafe.Add(base, at)), n)
}

func (c *bindCompiler) compile(p *stmt) bool {
	prog, m := c.prog, c.m
	if p.where != nil {
		if _, ok := c.cond(p.where); !ok {
			return false
		}
	}
	switch p.kind {
	case stSelect:
		var ok bool
		if prog.order, ok = bindOrder(m, p.order); !ok {
			return false
		}
		if prog.limit, ok = c.count(p.limit); !ok {
			return false
		}
		if prog.offset, ok = c.count(p.offset); !ok {
			return false
		}
		prog.projOK = !slices.ContainsFunc(p.proj, func(pc projCol) bool { return m.colIndex(pc.col) < 0 })
	case stInsert:
		set := make([]bool, len(m.cols))
		for k, name := range p.cols {
			i := m.colIndex(name)
			if i < 0 || set[i] {
				return false
			}
			set[i] = true
			col := m.cols[i]
			e, ok := c.expr(p.vals[k], col.typ)
			if !ok || e.slot < 0 {
				return false
			}
			if e, ok = c.convert(e, col.typ); !ok {
				return false
			}
			prog.cells = append(prog.cells, cellTmpl{col: i, slot: e.slot})
		}
		for i, col := range m.cols {
			if set[i] || !col.hasDef {
				continue
			}
			if !col.defOK {
				return false
			}
			prog.defs = append(prog.defs, i)
		}
	case stUpdate:
		for _, s := range p.sets {
			i := m.colIndex(s.col)
			if i < 0 || slices.Contains(m.pk, s.col) || slices.ContainsFunc(prog.sets, func(a setTmpl) bool { return a.col == i }) {
				return false
			}
			e, ok := c.expr(s.expr, m.cols[i].typ)
			if !ok {
				return false
			}
			prog.sets = append(prog.sets, setTmpl{col: i, node: c.node(e)})
		}
	}
	return true
}

// constant is the expression of a constant the program keeps
func (c *bindCompiler) constant(v tval) cexpr {
	c.prog.consts = append(c.prog.consts, v.s)
	c.prog.types = append(c.prog.types, v.t)
	c.static = append(c.static, true)
	return cexpr{t: v.t, slot: len(c.prog.consts) - 1, node: -1}
}

// emit is the expression of the value of the type t an instruction writes
func (c *bindCompiler) emit(t colType, op bindOp) cexpr {
	c.prog.consts = append(c.prog.consts, nil)
	c.prog.types = append(c.prog.types, t)
	c.static = append(c.static, false)
	op.dst = len(c.prog.consts) - 1
	op.str = -1
	if op.code != opFold && op.code != opTimestamp {
		op.str = c.prog.nstr
		c.prog.nstr++
	}
	c.prog.ops = append(c.prog.ops, op)
	return cexpr{t: t, slot: op.dst, node: -1}
}

// value is the constant of a static expression
func (c *bindCompiler) value(e cexpr) (tval, bool) {
	if e.slot < 0 || !c.static[e.slot] {
		return tval{}, false
	}
	return tval{t: c.prog.types[e.slot], s: c.prog.consts[e.slot]}, true
}

// use records that the program reads the value n
func (c *bindCompiler) use(n int) {
	c.prog.need = max(c.prog.need, n+1)
}

// bindType is binder.bindType
func (c *bindCompiler) bindType(n int, name string) bool {
	if prev := c.types[n]; prev != "" && prev != name {
		return false
	}
	c.types[n] = name
	return true
}

// param is binder.param
func (c *bindCompiler) param(n int, t colType) (cexpr, bool) {
	t = t.base()
	if !t.supported() {
		return cexpr{}, false
	}
	c.use(n)
	if !c.bindType(n, t.name) {
		return cexpr{}, false
	}
	return c.emit(t, bindOp{code: opParam, n: n, t: t}), true
}

// convert is the value of the constant e as the type t
func (c *bindCompiler) convert(e cexpr, t colType) (cexpr, bool) {
	if v, ok := c.value(e); ok {
		if v, ok = convert(v, t); !ok {
			return cexpr{}, false
		}
		return c.constant(v), true
	}
	if e.t == t && stableInput(t) {
		return e, true
	}
	return c.emit(t, bindOp{code: opConvert, a: e.slot, t: t}), true
}

// stableInput tells the input of the type takes any text it makes back as it is, so that a value
// of the type converts to the type as it is (TestStableInput). Not so of the timestamps: their
// input rejects the offset of the text to_timestamp makes.
func stableInput(t colType) bool {
	switch t.kind {
	case kInt, kNumeric, kText, kVarchar, kBpchar, kBool, kBytea, kJSONB, kUUID:
		return true
	}
	return false
}

// text is the constant of the text the type stores of a text given to its input function
func (c *bindCompiler) text(t colType, s string) (cexpr, bool) {
	out, ok := t.inputValue(s)
	if !ok {
		return cexpr{}, false
	}
	return c.constant(tval{t: t, s: &out}), true
}

// node is the node of an expression on a row of the expression
func (c *bindCompiler) node(e cexpr) int {
	if e.slot < 0 {
		return e.node
	}
	c.prog.exprs = append(c.prog.exprs, exprTmpl{op: eString, slot: e.slot, t: e.t})
	return len(c.prog.exprs) - 1
}

// expr is binder.bindExpr
func (c *bindCompiler) expr(e *exprNode, want colType) (cexpr, bool) {
	switch e.op {
	case eParam:
		return c.param(e.n, want)
	case eString:
		if !want.supported() {
			return cexpr{}, false
		}
		return c.text(want.base(), e.text)
	case eNumber:
		return c.text(numberType(e.text), e.text)
	case eNull:
		return c.constant(tval{t: typeUnknown}), true
	case eBool:
		s := boolText(e.text == "true")
		return c.constant(tval{t: typeBool, s: &s}), true
	case eCol:
		i := c.m.colIndex(e.text)
		if i < 0 {
			return cexpr{}, false
		}
		c.prog.exprs = append(c.prog.exprs, exprTmpl{op: eCol, col: i, t: c.m.cols[i].typ})
		return cexpr{t: c.m.cols[i].typ, slot: -1, node: len(c.prog.exprs) - 1}, true
	case eCast:
		t, ok := namedType(e.text, e.mods)
		if !ok {
			return cexpr{}, false
		}
		kid := e.kids[0]
		if kid.op == eString {
			return c.text(t, kid.text)
		}
		k, ok := c.expr(kid, t)
		if !ok {
			return cexpr{}, false
		}
		if k.slot >= 0 {
			return c.convert(k, t)
		}
		c.prog.exprs = append(c.prog.exprs, exprTmpl{op: eCast, t: t, kids: []int{k.node}})
		return cexpr{t: t, slot: -1, node: len(c.prog.exprs) - 1}, true
	case eDecode:
		k, ok := c.expr(e.kids[0], typeText)
		if !ok || k.slot < 0 || k.t.kind != kText {
			return cexpr{}, false
		}
		if v, ok := c.value(k); ok {
			if v.s == nil {
				return c.constant(tval{t: typeBytea}), true
			}
			raw, ok := decodeHex(*v.s)
			if !ok {
				return cexpr{}, false
			}
			s := byteaText(raw)
			return c.constant(tval{t: typeBytea, s: &s}), true
		}
		return c.emit(typeBytea, bindOp{code: opDecode, a: k.slot}), true
	case eToTimestamp:
		kid := e.kids[0]
		switch kid.op {
		case eParam:
			c.use(kid.n)
			c.stamps = append(c.stamps, len(c.prog.ops))
			return c.emit(typeTimestamptz, bindOp{code: opTimestamp, n: kid.n}), true
		case eNumber:
			v, ok := timestampOf(kid.text)
			if !ok {
				return cexpr{}, false
			}
			return c.constant(v), true
		}
		return cexpr{}, false
	case eAdd, eSub, eConcat:
		l, r := e.kids[0], e.kids[1]
		var left, right cexpr
		var ok bool
		// a placeholder takes the type of the other operand
		if l.op == eParam || l.op == eString {
			if right, ok = c.expr(r, typeUnknown); !ok {
				return cexpr{}, false
			}
			if left, ok = c.expr(l, right.t); !ok {
				return cexpr{}, false
			}
		} else {
			if left, ok = c.expr(l, typeUnknown); !ok {
				return cexpr{}, false
			}
			if right, ok = c.expr(r, left.t); !ok {
				return cexpr{}, false
			}
		}
		t, ok := opType(e.op, left.t, right.t)
		if !ok {
			return cexpr{}, false
		}
		if left.slot >= 0 && right.slot >= 0 {
			a, ok1 := c.value(left)
			b, ok2 := c.value(right)
			if ok1 && ok2 {
				v, ok := fold(e.op, t, a, b)
				if !ok {
					return cexpr{}, false
				}
				return c.constant(v), true
			}
			return c.emit(t, bindOp{code: opFold, op: e.op, a: left.slot, b: right.slot, t: t}), true
		}
		kids := []int{c.node(left), c.node(right)}
		c.prog.exprs = append(c.prog.exprs, exprTmpl{op: e.op, t: t, kids: kids})
		return cexpr{t: t, slot: -1, node: len(c.prog.exprs) - 1}, true
	}
	return cexpr{}, false
}

// cond is binder.bindCond: the index of the node of the condition
func (c *bindCompiler) cond(n *condNode) (int, bool) {
	prog := c.prog
	at := len(prog.conds)
	prog.conds = append(prog.conds, condTmpl{op: n.op})
	switch n.op {
	case cAnd, cOr:
		kids := make([]int, 0, len(n.kids))
		for _, k := range n.kids {
			ki, ok := c.cond(k)
			if !ok {
				return 0, false
			}
			kids = append(kids, ki)
		}
		prog.conds[at].kids = kids
		return at, true
	}
	i := c.m.colIndex(n.col)
	if i < 0 {
		return 0, false
	}
	ct := c.m.cols[i].typ
	prog.conds[at].col, prog.conds[at].cmp = i, n.cmp
	if n.op == cIsNull || n.op == cNotNull {
		return at, true
	}
	var fam colKind
	vals := make([]int, 0, len(n.vals))
	for _, e := range n.vals {
		be, ok := c.expr(e, ct)
		if !ok || be.slot < 0 {
			return 0, false
		}
		f, ok := family(ct, be.t)
		if !ok {
			return 0, false
		}
		if n.op == cCmp && n.cmp != "=" && n.cmp != "<>" && !ordered(f) {
			return 0, false
		}
		if len(vals) > 0 && fam != f {
			return 0, false
		}
		fam = f
		if f == kNumeric && be.t.kind == kNumeric {
			if v, ok := c.value(be); ok {
				if v.s != nil && !canonInt(*v.s) {
					nv, _ := parseNumeric(*v.s)
					s := nv.normal()
					v.s = &s
				}
				be = c.constant(v)
			} else {
				be = c.emit(be.t, bindOp{code: opNormal, a: be.slot})
			}
		}
		vals = append(vals, be.slot)
	}
	prog.conds[at].fam = fam
	if len(vals) > 0 {
		prog.conds[at].vals = vals
	}
	return at, true
}

// count is binder.count
func (c *bindCompiler) count(e *exprNode) (countTmpl, bool) {
	if e == nil {
		return countTmpl{slot: -1, n: -1}, true
	}
	if e.op == eNumber {
		n, err := strconv.ParseInt(e.text, 10, 64)
		if err != nil || n < 0 {
			return countTmpl{}, false
		}
		return countTmpl{slot: -1, n: n}, true
	}
	v, ok := c.param(e.n, typeInt8)
	if !ok {
		return countTmpl{}, false
	}
	return countTmpl{slot: v.slot}, true
}

// opType is the type of an operation on values of the types l and r, as bindExpr types it
func opType(op exprOp, l, r colType) (colType, bool) {
	switch {
	case op == eConcat && l.kind == kJSONB && r.kind == kJSONB:
		return typeJSONB, true
	case op != eConcat && l.numberLike() && r.numberLike():
		if l.kind == kInt && r.kind == kInt {
			if r.bits > l.bits {
				return r, true
			}
			return l, true
		}
		return typeNumeric, true
	}
	return colType{}, false
}

// timestampOf is the value of to_timestamp of the text of a float8
func timestampOf(text string) (tval, bool) {
	if !floatRe.MatchString(text) {
		return tval{}, false
	}
	tm, ok := unixTimestamp(text)
	if !ok {
		return tval{}, false
	}
	s := typeTimestamptz.timestampText(tm)
	return tval{t: typeTimestamptz, s: &s}, true
}

// slotsInline is the number of slots a run keeps on its stack
const slotsInline = 16

// run binds the values args by the program
func (prog *bindProg) run(args []driver.NamedValue) (planBinding, bool) {
	if prog.bad || len(args) < prog.need {
		return planBinding{}, false
	}
	var inline [slotsInline]*string
	var slots []*string
	if len(prog.consts) <= slotsInline {
		slots = inline[:len(prog.consts)]
	} else {
		slots = make([]*string, len(prog.consts))
	}
	copy(slots, prog.consts)
	// what a run makes it makes in one block
	var base unsafe.Pointer
	if prog.frame != nil {
		base = newFrame(prog.frame)
	}
	strs := framePart[string](base, prog.at.strs, prog.nstr)
	for i := range prog.ops {
		op := &prog.ops[i]
		switch op.code {
		case opParam:
			s, null, ok := op.t.inputArg(args[op.n].Value)
			if !ok {
				return planBinding{}, false
			}
			if !null {
				strs[op.str] = s
				slots[op.dst] = &strs[op.str]
			}
		case opConvert:
			v, ok := convertInto(tval{t: prog.types[op.a], s: slots[op.a]}, op.t, &strs[op.str])
			if !ok {
				return planBinding{}, false
			}
			slots[op.dst] = v.s
		case opDecode:
			k := slots[op.a]
			if k == nil {
				break
			}
			raw, ok := decodeHex(*k)
			if !ok {
				return planBinding{}, false
			}
			strs[op.str] = byteaText(raw)
			slots[op.dst] = &strs[op.str]
		case opTimestamp:
			a := args[op.n].Value
			if a == nil {
				break
			}
			s, binary, ok := argText(a)
			if !ok || binary || op.conflict {
				return planBinding{}, false
			}
			v, ok := timestampOf(s)
			if !ok {
				return planBinding{}, false
			}
			slots[op.dst] = v.s
		case opFold:
			v, ok := fold(op.op, op.t, tval{t: prog.types[op.a], s: slots[op.a]}, tval{t: prog.types[op.b], s: slots[op.b]})
			if !ok {
				return planBinding{}, false
			}
			slots[op.dst] = v.s
		case opNormal:
			v := slots[op.a]
			if v != nil && !canonInt(*v) {
				n, _ := parseNumeric(*v)
				strs[op.str] = n.normal()
				v = &strs[op.str]
			}
			slots[op.dst] = v
		}
	}
	out := planBinding{limit: -1, order: prog.order}
	var ok bool
	if out.limit, ok = prog.limit.of(slots); !ok {
		return planBinding{}, false
	}
	if out.offset, ok = prog.offset.of(slots); !ok {
		return planBinding{}, false
	}
	if out.offset < 0 {
		out.offset = 0
	}
	// the values of the nodes of the condition and the row of an insert share one block
	nrow := 0
	if prog.cells != nil || prog.defs != nil {
		nrow = len(prog.meta.cols)
	}
	texts := framePart[*string](base, prog.at.texts, nrow+prog.nvals)
	if len(prog.conds) > 0 {
		out.cond = prog.bindConds(base, slots, texts[nrow:])
	}
	if nrow > 0 {
		row := stateRow(texts[:nrow:nrow])
		for _, c := range prog.cells {
			row[c.col] = slots[c.slot]
		}
		for _, i := range prog.defs {
			row[i] = prog.meta.cols[i].def
		}
		out.row = row
	}
	if len(prog.sets) > 0 {
		out.sets = prog.bindSets(base, slots)
	}
	return out, true
}

// of is the value of the count; -1 for none
func (t countTmpl) of(slots []*string) (int64, bool) {
	if t.slot < 0 {
		return t.n, true
	}
	s := slots[t.slot]
	if s == nil {
		return -1, true
	}
	n, err := strconv.ParseInt(*s, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// bindConds is the condition of the values of the slots; vals holds the values of the nodes of
// more than one value
func (prog *bindProg) bindConds(base unsafe.Pointer, slots []*string, vals []*string) *bcond {
	conds := framePart[bcond](base, prog.at.conds, len(prog.conds))
	kids := framePart[*bcond](base, prog.at.ckids, prog.nkids)
	for i := range prog.conds {
		t := &prog.conds[i]
		x := &conds[i]
		x.op, x.col, x.cmp, x.fam = t.op, t.col, t.cmp, t.fam
		if len(t.kids) > 0 {
			ks := kids[t.k0 : t.k0+len(t.kids) : t.k0+len(t.kids)]
			for j, k := range t.kids {
				ks[j] = &conds[k]
			}
			x.kids = ks
		}
		switch len(t.vals) {
		case 0:
		case 1:
			x.one[0] = slots[t.vals[0]]
			x.vals = x.one[:]
		default:
			vs := vals[t.v0 : t.v0+len(t.vals) : t.v0+len(t.vals)]
			for j, s := range t.vals {
				vs[j] = slots[s]
			}
			x.vals = vs
		}
	}
	return &conds[0]
}

// bindSets are the assignments of the values of the slots
func (prog *bindProg) bindSets(base unsafe.Pointer, slots []*string) []assign {
	xs := framePart[bexpr](base, prog.at.xs, len(prog.exprs))
	kids := framePart[*bexpr](base, prog.at.xkids, prog.nexprKids)
	for i := range prog.exprs {
		t := &prog.exprs[i]
		x := &xs[i]
		switch t.op {
		case eString:
			x.op, x.v, x.t = eString, tval{t: t.t, s: slots[t.slot]}, t.t
		case eCol:
			x.op, x.col, x.t = eCol, t.col, t.t
		default:
			ks := kids[t.k0 : t.k0+len(t.kids) : t.k0+len(t.kids)]
			for j, k := range t.kids {
				ks[j] = &xs[k]
			}
			x.op, x.t, x.kids = t.op, t.t, ks
		}
	}
	sets := framePart[assign](base, prog.at.sets, len(prog.sets))
	for j, s := range prog.sets {
		sets[j] = assign{col: s.col, expr: &xs[s.node]}
	}
	return sets
}

// diffBinding is how a binding b and whether it bound, ok, differ from the binding want and
// wantOK; "" when they do not
func diffBinding(want planBinding, wantOK bool, b planBinding, ok bool) string {
	if wantOK != ok {
		return fmt.Sprintf("bound %v, want %v", ok, wantOK)
	}
	if !ok {
		return ""
	}
	if d := diffCond(want.cond, b.cond); d != "" {
		return "cond: " + d
	}
	if !slices.Equal(want.order, b.order) {
		return fmt.Sprintf("order %v, want %v", b.order, want.order)
	}
	if want.limit != b.limit || want.offset != b.offset {
		return fmt.Sprintf("limit %d offset %d, want %d %d", b.limit, b.offset, want.limit, want.offset)
	}
	if (want.row == nil) != (b.row == nil) || len(want.row) != len(b.row) {
		return fmt.Sprintf("row %v, want %v", b.row, want.row)
	}
	for i := range want.row {
		if !sameText(want.row[i], b.row[i]) {
			return fmt.Sprintf("row column %d %s, want %s", i, textOf(b.row[i]), textOf(want.row[i]))
		}
	}
	if len(want.sets) != len(b.sets) {
		return fmt.Sprintf("%d sets, want %d", len(b.sets), len(want.sets))
	}
	for i := range want.sets {
		if want.sets[i].col != b.sets[i].col {
			return fmt.Sprintf("set %d of column %d, want %d", i, b.sets[i].col, want.sets[i].col)
		}
		if d := diffExpr(want.sets[i].expr, b.sets[i].expr); d != "" {
			return fmt.Sprintf("set %d: %s", i, d)
		}
	}
	return ""
}

func diffCond(want, c *bcond) string {
	if (want == nil) != (c == nil) {
		return fmt.Sprintf("%v, want %v", c, want)
	}
	if want == nil {
		return ""
	}
	if want.op != c.op || want.col != c.col || want.cmp != c.cmp || want.fam != c.fam {
		return fmt.Sprintf("node %d %d %q %d, want %d %d %q %d", c.op, c.col, c.cmp, c.fam, want.op, want.col, want.cmp, want.fam)
	}
	if len(want.kids) != len(c.kids) || len(want.vals) != len(c.vals) {
		return fmt.Sprintf("%d kids %d values, want %d %d", len(c.kids), len(c.vals), len(want.kids), len(want.vals))
	}
	for i := range want.vals {
		if !sameText(want.vals[i], c.vals[i]) {
			return fmt.Sprintf("value %d %s, want %s", i, textOf(c.vals[i]), textOf(want.vals[i]))
		}
	}
	for i := range want.kids {
		if d := diffCond(want.kids[i], c.kids[i]); d != "" {
			return d
		}
	}
	return ""
}

func diffExpr(want, x *bexpr) string {
	if want.op != x.op || want.col != x.col || want.t != x.t || want.v.t != x.v.t || !sameText(want.v.s, x.v.s) {
		return fmt.Sprintf("node %d %d %s %s %s, want %d %d %s %s %s", x.op, x.col, x.t.name, x.v.t.name, textOf(x.v.s),
			want.op, want.col, want.t.name, want.v.t.name, textOf(want.v.s))
	}
	if len(want.kids) != len(x.kids) {
		return fmt.Sprintf("%d kids, want %d", len(x.kids), len(want.kids))
	}
	for i := range want.kids {
		if d := diffExpr(want.kids[i], x.kids[i]); d != "" {
			return d
		}
	}
	return ""
}

func sameText(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func textOf(s *string) string {
	if s == nil {
		return "NULL"
	}
	return strconv.Quote(*s)
}
