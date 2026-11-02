/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql/driver"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
)

// The state of a block plans the statements it runs in memory: the selects of the rows of one
// table by a condition, the inserts of one row, and the updates and deletes of the rows of one
// table by a condition. It runs any other statement in PostgreSQL, and knows from its tokens the
// tables it reads and writes.

// stateCatalog is what the state knows of the relations of the schema
type stateCatalog struct {
	// rels are the relations of the schema by name; true for a table, false for a view
	rels map[string]bool
	// parents are the partitioned tables of the partitions
	parents map[string]string
	// funcs are the functions of the chain, which may read any table
	funcs map[string]bool
}

const catalogQuery = `SELECT c.relname, c.relkind::text, coalesce(p.relname, '')
	FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
	LEFT JOIN pg_inherits i ON i.inhrelid = c.oid LEFT JOIN pg_class p ON p.oid = i.inhparent
	WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f')
		AND (n.oid = current_schema()::regnamespace OR i.inhparent IS NOT NULL)`

const funcsQuery = `SELECT DISTINCT p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
	WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')`

// loadCatalog reads the catalog of the schema with queries on the connection
func loadCatalog(ctx context.Context, q driver.QueryerContext) (*stateCatalog, error) {
	c := &stateCatalog{rels: make(map[string]bool), parents: make(map[string]string), funcs: make(map[string]bool)}
	err := queryText(ctx, q, catalogQuery, func(v []string) {
		if v[2] != "" {
			c.parents[v[0]] = v[2]
			return
		}
		c.rels[v[0]] = v[1] == "r" || v[1] == "p" || v[1] == "f"
	})
	if err != nil {
		return nil, err
	}
	if err = queryText(ctx, q, funcsQuery, func(v []string) { c.funcs[v[0]] = true }); err != nil {
		return nil, err
	}
	return c, nil
}

// queryText runs a query with no arguments and gives each row to f as text
func queryText(ctx context.Context, q driver.QueryerContext, query string, f func([]string)) error {
	rows, err := q.QueryContext(ctx, query, nil)
	if err != nil {
		return err
	}
	defer rows.Close()
	dest := make([]driver.Value, len(rows.Columns()))
	text := make([]string, len(dest))
	for {
		if err = rows.Next(dest); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
		for i, v := range dest {
			switch x := v.(type) {
			case string:
				text[i] = x
			case []byte:
				text[i] = string(x)
			default:
				text[i] = ""
			}
		}
		f(text)
	}
}

// canon is the relation a name in a statement names: the partitioned table of a partition.
// quoted tells the name was quoted, and keeps its case.
func (c *stateCatalog) canon(name string, quoted bool) string {
	if !quoted {
		name = strings.ToLower(name)
	}
	if p, ok := c.parents[name]; ok {
		return p
	}
	return name
}

// known tells the name is a table of the schema
func (c *stateCatalog) known(table string) bool { return c.rels[table] }

// sideEffects are the functions of the database that change what the transaction of a block
// cannot roll back, or the session of the connection
var sideEffects = map[string]bool{
	"nextval": true, "setval": true, "pg_notify": true, "set_config": true, "txid_current": true,
	"pg_advisory_lock": true, "pg_advisory_xact_lock": true, "pg_try_advisory_lock": true,
	"pg_try_advisory_xact_lock": true, "pg_export_snapshot": true, "lo_import": true, "lo_export": true,
	"dblink": true, "dblink_exec": true,
}

type stmtKind int

const (
	// stOther is a statement the state runs in PostgreSQL, which may read and write any table
	stOther stmtKind = iota
	// stRead is a read the state runs in PostgreSQL: of the tables, or of any table for allReads
	stRead
	// stWrite is a write of the table the state runs in PostgreSQL, which reads the tables
	stWrite
	stSelect
	// stMaxID is the select of the highest id of a table: select id from "t" order by id desc limit 1
	stMaxID
	stInsert
	stUpdate
	stDelete
	stSavepoint
	stRollbackTo
	stRelease
)

// stmt is the plan of a statement
type stmt struct {
	kind     stmtKind
	table    string
	tables   []string
	allReads bool
	alias    string
	// the select
	star      bool
	proj      []projCol
	where     *condNode
	whereToks []token
	order     []orderItem
	orderToks []token
	limit     *exprNode
	offset    *exprNode
	// the update
	sets []setItem
	// the insert
	cols []string
	vals []*exprNode
	// the savepoint
	name string
	// head is the projection of the select on the table it last read
	head atomic.Pointer[projHead]
}

type projCol struct{ col, as string }

type orderItem struct {
	col        string
	desc       bool
	nullsFirst bool
}

type setItem struct {
	col  string
	expr *exprNode
}

type exprOp int

const (
	eParam exprOp = iota
	eString
	eNumber
	eNull
	eBool
	eCol
	eCast
	eDecode
	eToTimestamp
	eAdd
	eSub
	eConcat
)

// exprNode is an expression of a value: n is the index of a placeholder, text the text of a
// literal, the name of a column or the type of a cast, mods the modifiers of the type
type exprNode struct {
	op   exprOp
	n    int
	text string
	mods []int
	kids []*exprNode
}

func (e *exprNode) hasCol() bool {
	if e.op == eCol {
		return true
	}
	return slices.ContainsFunc(e.kids, (*exprNode).hasCol)
}

type condOp int

const (
	cAnd condOp = iota
	cOr
	cCmp
	cIn
	cNotIn
	cIsNull
	cNotNull
)

// condNode is a condition: a conjunction or disjunction of its kids, or a test of a column
type condNode struct {
	op   condOp
	kids []*condNode
	col  string
	cmp  string
	vals []*exprNode
}

// parseStmt is the plan of a statement
func parseStmt(query string, cat *stateCatalog) *stmt {
	toks := tokenize(query)
	for len(toks) > 0 && toks[len(toks)-1].is(";") {
		toks = toks[:len(toks)-1]
	}
	if len(toks) == 0 || slices.ContainsFunc(toks, func(t token) bool { return t.is(";") }) {
		return &stmt{kind: stOther}
	}
	switch toks[0].keyword() {
	case "savepoint":
		return &stmt{kind: stSavepoint, name: savepointName(toks[1:])}
	case "release":
		return &stmt{kind: stRelease, name: savepointName(toks[1:])}
	case "rollback":
		if len(toks) > 1 && toks[1].keyword() == "to" {
			return &stmt{kind: stRollbackTo, name: savepointName(toks[2:])}
		}
	case "select":
		p := &parser{toks: toks, cat: cat}
		if st, ok := p.selectStmt(); ok {
			return st
		}
		return readStmt(toks, cat)
	case "with", "values", "table":
		if slices.ContainsFunc(toks, func(t token) bool {
			k := t.keyword()
			return k == "insert" || k == "update" || k == "delete"
		}) {
			return &stmt{kind: stOther}
		}
		return readStmt(toks, cat)
	case "insert", "update", "delete":
		p := &parser{toks: toks, cat: cat}
		var (
			st *stmt
			ok bool
		)
		switch toks[0].keyword() {
		case "insert":
			st, ok = p.insertStmt()
		case "update":
			st, ok = p.updateStmt()
		default:
			st, ok = p.deleteStmt()
		}
		if ok {
			return st
		}
		table, end, ok := target(toks, cat)
		if !ok || !cat.known(table) {
			return &stmt{kind: stOther}
		}
		refs, all, effects := scanReads(toks, end, cat)
		if effects {
			return &stmt{kind: stOther}
		}
		return &stmt{kind: stWrite, table: table, tables: refs, allReads: all}
	}
	return &stmt{kind: stOther}
}

// readStmt is the plan of a read the state runs in PostgreSQL
func readStmt(toks []token, cat *stateCatalog) *stmt {
	if slices.ContainsFunc(toks, func(t token) bool {
		switch t.keyword() {
		case "for", "into":
			// a lock of rows, or a select that creates a table
			return true
		}
		return false
	}) {
		return &stmt{kind: stOther}
	}
	refs, all, effects := scanReads(toks, 0, cat)
	if effects {
		return &stmt{kind: stOther}
	}
	return &stmt{kind: stRead, tables: refs, allReads: all}
}

func savepointName(toks []token) string {
	if len(toks) > 0 && toks[0].keyword() == "savepoint" {
		toks = toks[1:]
	}
	if len(toks) == 0 {
		return ""
	}
	return toks[0].text
}

// target is the table a write statement writes and the index of the token after its name
func target(toks []token, cat *stateCatalog) (string, int, bool) {
	i := 1
	switch toks[0].keyword() {
	case "insert":
		if i >= len(toks) || toks[i].keyword() != "into" {
			return "", 0, false
		}
		i++
	case "delete":
		if i >= len(toks) || toks[i].keyword() != "from" {
			return "", 0, false
		}
		i++
	}
	if i < len(toks) && toks[i].keyword() == "only" {
		i++
	}
	return relName(toks, i, cat)
}

// relName is the relation a dotted name at i names, and the index of the token after the name
func relName(toks []token, i int, cat *stateCatalog) (string, int, bool) {
	if i >= len(toks) || !toks[i].isName() {
		return "", 0, false
	}
	last := toks[i]
	i++
	for i+1 < len(toks) && toks[i].is(".") && toks[i+1].isName() {
		last = toks[i+1]
		i += 2
	}
	return cat.canon(last.text, last.kind == tQuoted), i, true
}

// systemName tells a name at i is of the catalog of the database, which only the changes of the
// schema write
func systemName(toks []token, i int) bool {
	if toks[i].kind != tIdent {
		return false
	}
	return toks[i].text == "pg_catalog" || toks[i].text == "information_schema" || strings.HasPrefix(toks[i].text, "pg_")
}

// scanReads is the tables the tokens from start read: the relations of the FROM, JOIN and USING
// clauses. all tells the statement may read what the tables do not tell: a view, a relation the
// catalog does not have, a function of the chain, or a clause the scan does not follow. effects
// tells it calls a function with side effects.
func scanReads(toks []token, start int, cat *stateCatalog) (refs []string, all, effects bool) {
	// fromList tells, by depth of parentheses, that the tokens are in a list of relations
	fromList := []bool{false}
	expect := false
	for i := start; i < len(toks); i++ {
		t := toks[i]
		depth := len(fromList) - 1
		if t.kind == tIdent && i+1 < len(toks) && toks[i+1].is("(") {
			if sideEffects[t.text] {
				effects = true
			}
			if cat.funcs[t.text] {
				all = true
			}
		}
		switch {
		case t.is("("):
			inner := expect
			expect = false
			fromList = append(fromList, inner)
			if inner && i+1 < len(toks) {
				switch toks[i+1].keyword() {
				case "select", "with", "values":
					fromList[depth+1] = false
				default:
					expect = true
				}
			}
			continue
		case t.is(")"):
			if depth > 0 {
				fromList = fromList[:depth]
			}
			expect = false
			continue
		case t.is(","):
			if fromList[depth] {
				expect = true
			}
			continue
		}
		switch t.keyword() {
		case "from", "join":
			fromList[depth] = true
			expect = true
			continue
		case "using":
			if i+1 < len(toks) && toks[i+1].is("(") {
				continue
			}
			fromList[depth] = true
			expect = true
			continue
		case "where", "group", "order", "limit", "offset", "having", "window", "union", "intersect", "except",
			"returning", "set", "fetch", "values", "select", "into", "for", "on":
			fromList[depth] = false
			expect = false
			continue
		case "only", "lateral":
			continue
		}
		if !expect {
			continue
		}
		expect = false
		if !t.isName() {
			all = true
			continue
		}
		if systemName(toks, i) {
			_, next, _ := relName(toks, i, cat)
			i = next - 1
			continue
		}
		name, next, _ := relName(toks, i, cat)
		i = next - 1
		if next < len(toks) && toks[next].is("(") {
			// a function of the FROM clause reads its arguments; a function of the chain may read
			// any table
			if cat.funcs[name] {
				all = true
			}
			continue
		}
		if !cat.known(name) {
			all = true
			continue
		}
		if !slices.Contains(refs, name) {
			refs = append(refs, name)
		}
	}
	return refs, all, effects
}

// parser reads the plan of a statement from its tokens
type parser struct {
	toks  []token
	i     int
	cat   *stateCatalog
	table string
	alias string
}

func (p *parser) end() bool { return p.i >= len(p.toks) }

func (p *parser) peek() token {
	if p.end() {
		return token{kind: tPunct}
	}
	return p.toks[p.i]
}

// kw takes the keyword at the position
func (p *parser) kw(word string) bool {
	if p.peek().keyword() == word {
		p.i++
		return true
	}
	return false
}

// punct takes the punctuation at the position
func (p *parser) punct(s string) bool {
	if !p.end() && p.toks[p.i].is(s) {
		p.i++
		return true
	}
	return false
}

// clause tells the keyword at the position ends a clause
func (p *parser) clause() bool {
	switch p.peek().keyword() {
	case "where", "order", "limit", "offset", "group", "having", "window", "for", "fetch", "union",
		"intersect", "except", "returning", "set", "values", "on", "join", "inner", "left", "right", "full",
		"cross", "natural", "using":
		return true
	}
	return false
}

// relation reads the table of a planned statement: a table of the schema that is no partition
func (p *parser) relation() bool {
	if p.end() || !p.peek().isName() {
		return false
	}
	t := p.toks[p.i]
	if p.i+1 < len(p.toks) && p.toks[p.i+1].is(".") {
		if t.kind != tIdent || t.text != "public" {
			return false
		}
		p.i += 2
		if p.end() || !p.peek().isName() {
			return false
		}
		t = p.toks[p.i]
	}
	p.i++
	name := t.text
	if t.kind == tIdent {
		name = strings.ToLower(name)
	}
	if !p.cat.known(name) || p.cat.parents[name] != "" {
		return false
	}
	p.table = name
	return true
}

// aliasName reads the alias of the table, if any
func (p *parser) aliasName() bool {
	as := p.kw("as")
	t := p.peek()
	if t.isName() && !(t.kind == tIdent && (t.isKeyword() || p.clause())) {
		p.alias = t.text
		p.i++
		return true
	}
	return !as
}

// column reads a reference to a column of the table: a name, qualified by the table or its alias
func (p *parser) column() (string, bool) {
	t := p.peek()
	if !t.isName() || t.kind == tIdent && t.isKeyword() {
		return "", false
	}
	p.i++
	if !p.punct(".") {
		return t.text, true
	}
	if t.text != p.table && t.text != p.alias {
		return "", false
	}
	c := p.peek()
	if !c.isName() {
		return "", false
	}
	p.i++
	return c.text, true
}

func (p *parser) selectStmt() (*stmt, bool) {
	st := &stmt{kind: stSelect}
	p.i = 1
	if p.peek().keyword() == "distinct" || p.peek().keyword() == "all" {
		return nil, false
	}
	type item struct {
		qual, col, as string
		star          bool
	}
	var items []item
	for {
		if p.punct("*") {
			items = append(items, item{star: true})
		} else {
			t := p.peek()
			if !t.isName() || t.kind == tIdent && t.isKeyword() {
				return nil, false
			}
			p.i++
			it := item{col: t.text}
			if p.punct(".") {
				it.qual = t.text
				if p.punct("*") {
					it.star = true
				} else {
					c := p.peek()
					if !c.isName() {
						return nil, false
					}
					p.i++
					it.col = c.text
				}
			}
			if !it.star {
				it.as = it.col
				as := p.kw("as")
				if a := p.peek(); a.isName() && !(a.kind == tIdent && (a.isKeyword() || a.keyword() == "from")) {
					it.as = a.text
					p.i++
				} else if as {
					return nil, false
				}
			}
			items = append(items, it)
		}
		if !p.punct(",") {
			break
		}
	}
	if !p.kw("from") || !p.relation() || !p.aliasName() {
		return nil, false
	}
	st.table, st.alias = p.table, p.alias
	for _, it := range items {
		if it.qual != "" && it.qual != p.table && it.qual != p.alias {
			return nil, false
		}
		if it.star {
			if len(items) == 1 {
				st.star = true
			} else {
				return nil, false
			}
			continue
		}
		st.proj = append(st.proj, projCol{col: it.col, as: it.as})
	}
	if !p.tail(st) {
		return nil, false
	}
	if len(st.proj) == 1 && st.proj[0].col == "id" && st.where == nil && len(st.order) == 1 &&
		st.order[0].col == "id" && st.order[0].desc && st.limit != nil &&
		st.limit.op == eNumber && st.limit.text == "1" && st.offset == nil {
		st.kind = stMaxID
	}
	st.tables = []string{st.table}
	return st, true
}

// tail reads the WHERE, ORDER BY, LIMIT and OFFSET clauses of a select
func (p *parser) tail(st *stmt) bool {
	if p.kw("where") {
		from := p.i
		c, ok := p.condition()
		if !ok {
			return false
		}
		st.where, st.whereToks = c, p.toks[from:p.i]
	}
	if p.kw("order") {
		if !p.kw("by") {
			return false
		}
		from := p.i
		for {
			col, ok := p.column()
			if !ok {
				return false
			}
			it := orderItem{col: col}
			if p.kw("desc") {
				it.desc = true
			} else {
				p.kw("asc")
			}
			it.nullsFirst = it.desc
			if p.kw("nulls") {
				switch {
				case p.kw("first"):
					it.nullsFirst = true
				case p.kw("last"):
					it.nullsFirst = false
				default:
					return false
				}
			}
			st.order = append(st.order, it)
			if !p.punct(",") {
				break
			}
		}
		st.orderToks = p.toks[from:p.i]
	}
	for !p.end() {
		switch {
		case p.kw("limit"):
			if st.limit != nil {
				return false
			}
			if p.kw("all") {
				continue
			}
			e, ok := p.count()
			if !ok {
				return false
			}
			st.limit = e
		case p.kw("offset"):
			if st.offset != nil {
				return false
			}
			e, ok := p.count()
			if !ok {
				return false
			}
			st.offset = e
			if !p.kw("rows") {
				p.kw("row")
			}
		default:
			return false
		}
	}
	return true
}

// count reads the count of a LIMIT or OFFSET clause: a number or a placeholder
func (p *parser) count() (*exprNode, bool) {
	t := p.peek()
	switch t.kind {
	case tNumber:
		if _, err := strconv.ParseInt(t.text, 10, 64); err != nil {
			return nil, false
		}
		p.i++
		return &exprNode{op: eNumber, text: t.text}, true
	case tParam:
		n, ok := paramIndex(t)
		if !ok {
			return nil, false
		}
		p.i++
		return &exprNode{op: eParam, n: n}, true
	}
	return nil, false
}

func paramIndex(t token) (int, bool) {
	n, err := strconv.Atoi(t.text[1:])
	if err != nil || n < 1 {
		return 0, false
	}
	return n - 1, true
}

func (p *parser) updateStmt() (*stmt, bool) {
	p.i = 1
	p.kw("only")
	if !p.relation() {
		return nil, false
	}
	if p.peek().keyword() != "set" && !p.aliasName() {
		return nil, false
	}
	if !p.kw("set") {
		return nil, false
	}
	st := &stmt{kind: stUpdate, table: p.table, alias: p.alias, tables: []string{p.table}}
	for {
		t := p.peek()
		if !t.isName() {
			return nil, false
		}
		p.i++
		if !p.punct("=") {
			return nil, false
		}
		e, ok := p.expr()
		if !ok {
			return nil, false
		}
		st.sets = append(st.sets, setItem{col: t.text, expr: e})
		if !p.punct(",") {
			break
		}
	}
	if p.kw("where") {
		from := p.i
		c, ok := p.condition()
		if !ok {
			return nil, false
		}
		st.where, st.whereToks = c, p.toks[from:p.i]
	}
	return st, p.end()
}

func (p *parser) insertStmt() (*stmt, bool) {
	p.i = 1
	if !p.kw("into") || !p.relation() || !p.punct("(") {
		return nil, false
	}
	st := &stmt{kind: stInsert, table: p.table, tables: []string{p.table}}
	for {
		t := p.peek()
		if !t.isName() {
			return nil, false
		}
		p.i++
		st.cols = append(st.cols, t.text)
		if !p.punct(",") {
			break
		}
	}
	if !p.punct(")") || !p.kw("values") || !p.punct("(") {
		return nil, false
	}
	for {
		e, ok := p.expr()
		if !ok || e.hasCol() {
			return nil, false
		}
		st.vals = append(st.vals, e)
		if !p.punct(",") {
			break
		}
	}
	if !p.punct(")") || len(st.vals) != len(st.cols) {
		return nil, false
	}
	return st, p.end()
}

func (p *parser) deleteStmt() (*stmt, bool) {
	p.i = 1
	if !p.kw("from") {
		return nil, false
	}
	p.kw("only")
	if !p.relation() || !p.aliasName() {
		return nil, false
	}
	st := &stmt{kind: stDelete, table: p.table, alias: p.alias, tables: []string{p.table}}
	if p.kw("where") {
		from := p.i
		c, ok := p.condition()
		if !ok {
			return nil, false
		}
		st.where, st.whereToks = c, p.toks[from:p.i]
	}
	return st, p.end()
}

// condition reads a condition: a disjunction of conjunctions of tests of columns
func (p *parser) condition() (*condNode, bool) {
	left, ok := p.conjunction()
	if !ok {
		return nil, false
	}
	if p.peek().keyword() != "or" {
		return left, true
	}
	c := &condNode{op: cOr, kids: []*condNode{left}}
	for p.kw("or") {
		k, ok := p.conjunction()
		if !ok {
			return nil, false
		}
		c.kids = append(c.kids, k)
	}
	return c, true
}

func (p *parser) conjunction() (*condNode, bool) {
	left, ok := p.test()
	if !ok {
		return nil, false
	}
	if p.peek().keyword() != "and" {
		return left, true
	}
	c := &condNode{op: cAnd, kids: []*condNode{left}}
	for p.kw("and") {
		k, ok := p.test()
		if !ok {
			return nil, false
		}
		c.kids = append(c.kids, k)
	}
	return c, true
}

var flipped = map[string]string{"=": "=", "<>": "<>", "!=": "<>", "<": ">", "<=": ">=", ">": "<", ">=": "<="}

func cmpOp(t token) (string, bool) {
	if t.kind != tPunct {
		return "", false
	}
	switch t.text {
	case "=", "<>", "<", "<=", ">", ">=":
		return t.text, true
	case "!=":
		return "<>", true
	}
	return "", false
}

// test reads a test of a column, or a condition in parentheses
func (p *parser) test() (*condNode, bool) {
	if p.peek().is("(") {
		save := p.i
		p.i++
		if c, ok := p.condition(); ok && p.punct(")") {
			return c, true
		}
		p.i = save
	}
	save := p.i
	if col, ok := p.column(); ok {
		switch {
		case p.kw("is"):
			op := cIsNull
			if p.kw("not") {
				op = cNotNull
			}
			if !p.kw("null") {
				return nil, false
			}
			return &condNode{op: op, col: col}, true
		case p.peek().keyword() == "in" || p.peek().keyword() == "not":
			op := cIn
			if p.kw("not") {
				op = cNotIn
			}
			if !p.kw("in") || !p.punct("(") {
				return nil, false
			}
			c := &condNode{op: op, col: col}
			for {
				e, ok := p.expr()
				if !ok || e.hasCol() {
					return nil, false
				}
				c.vals = append(c.vals, e)
				if !p.punct(",") {
					break
				}
			}
			return c, p.punct(")")
		}
		if op, ok := cmpOp(p.peek()); ok {
			p.i++
			e, ok := p.expr()
			if !ok || e.hasCol() {
				return nil, false
			}
			return &condNode{op: cCmp, col: col, cmp: op, vals: []*exprNode{e}}, true
		}
		return nil, false
	}
	p.i = save
	e, ok := p.expr()
	if !ok || e.hasCol() {
		return nil, false
	}
	op, ok := cmpOp(p.peek())
	if !ok {
		return nil, false
	}
	p.i++
	col, ok := p.column()
	if !ok {
		return nil, false
	}
	return &condNode{op: cCmp, col: col, cmp: flipped[op], vals: []*exprNode{e}}, true
}

// expr reads an expression: terms joined by || over sums
func (p *parser) expr() (*exprNode, bool) {
	left, ok := p.sum()
	if !ok {
		return nil, false
	}
	for p.punct("||") {
		right, ok := p.sum()
		if !ok {
			return nil, false
		}
		left = &exprNode{op: eConcat, kids: []*exprNode{left, right}}
	}
	return left, true
}

func (p *parser) sum() (*exprNode, bool) {
	left, ok := p.term()
	if !ok {
		return nil, false
	}
	for {
		op := eAdd
		switch {
		case p.punct("+"):
		case p.punct("-"):
			op = eSub
		default:
			return left, true
		}
		right, ok := p.term()
		if !ok {
			return nil, false
		}
		left = &exprNode{op: op, kids: []*exprNode{left, right}}
	}
}

// term reads a value with its casts
func (p *parser) term() (*exprNode, bool) {
	e, ok := p.primary()
	if !ok {
		return nil, false
	}
	for p.punct("::") {
		typ, mods, ok := p.typeName()
		if !ok {
			return nil, false
		}
		e = &exprNode{op: eCast, text: typ, mods: mods, kids: []*exprNode{e}}
	}
	return e, true
}

// typeName reads the name of a type of a cast, as the catalog names it
func (p *parser) typeName() (string, []int, bool) {
	t := p.peek()
	if t.kind != tIdent {
		return "", nil, false
	}
	p.i++
	var name string
	switch t.text {
	case "bigint", "int8":
		name = "int8"
	case "integer", "int", "int4":
		name = "int4"
	case "smallint", "int2":
		name = "int2"
	case "numeric", "decimal":
		name = "numeric"
	case "text":
		name = "text"
	case "varchar":
		name = "varchar"
	case "character":
		if !p.kw("varying") {
			return "", nil, false
		}
		name = "varchar"
	case "jsonb":
		name = "jsonb"
	case "bool", "boolean":
		name = "bool"
	case "bytea":
		name = "bytea"
	case "timestamptz":
		name = "timestamptz"
	case "uuid":
		name = "uuid"
	case "timestamp":
		name = "timestamp"
		if p.kw("without") {
			if !p.kw("time") || !p.kw("zone") {
				return "", nil, false
			}
		} else if p.kw("with") {
			if !p.kw("time") || !p.kw("zone") {
				return "", nil, false
			}
			name = "timestamptz"
		}
	default:
		return "", nil, false
	}
	var mods []int
	if p.punct("(") {
		for {
			n := p.peek()
			if n.kind != tNumber {
				return "", nil, false
			}
			v, err := strconv.Atoi(n.text)
			if err != nil {
				return "", nil, false
			}
			p.i++
			mods = append(mods, v)
			if !p.punct(",") {
				break
			}
		}
		if !p.punct(")") {
			return "", nil, false
		}
	}
	return name, mods, true
}

func (p *parser) primary() (*exprNode, bool) {
	t := p.peek()
	switch t.kind {
	case tParam:
		n, ok := paramIndex(t)
		if !ok {
			return nil, false
		}
		p.i++
		return &exprNode{op: eParam, n: n}, true
	case tString:
		p.i++
		return &exprNode{op: eString, text: t.text}, true
	case tNumber:
		p.i++
		return &exprNode{op: eNumber, text: t.text}, true
	case tPunct:
		switch {
		case t.text == "-" && p.i+1 < len(p.toks) && p.toks[p.i+1].kind == tNumber:
			p.i += 2
			return &exprNode{op: eNumber, text: "-" + p.toks[p.i-1].text}, true
		case t.text == "(":
			p.i++
			e, ok := p.expr()
			if !ok || !p.punct(")") {
				return nil, false
			}
			return e, true
		}
		return nil, false
	}
	if t.kind == tIdent {
		switch t.text {
		case "null":
			p.i++
			return &exprNode{op: eNull}, true
		case "true", "false":
			p.i++
			return &exprNode{op: eBool, text: t.text}, true
		}
		if p.i+1 < len(p.toks) && p.toks[p.i+1].is("(") {
			p.i += 2
			switch t.text {
			case "decode":
				e, ok := p.expr()
				if !ok || !p.punct(",") {
					return nil, false
				}
				f := p.peek()
				if f.kind != tString || !strings.EqualFold(f.text, "hex") {
					return nil, false
				}
				p.i++
				if !p.punct(")") {
					return nil, false
				}
				return &exprNode{op: eDecode, kids: []*exprNode{e}}, true
			case "to_timestamp":
				e, ok := p.expr()
				if !ok || !p.punct(")") {
					return nil, false
				}
				return &exprNode{op: eToTimestamp, kids: []*exprNode{e}}, true
			}
			return nil, false
		}
	}
	col, ok := p.column()
	if !ok {
		return nil, false
	}
	return &exprNode{op: eCol, text: col}, true
}

// renderTokens is the text of tokens, with the placeholders numbered anew by params: params maps
// the index of a placeholder of the tokens to its index in the text, and grows with the
// placeholders it has not seen; order is the indexes of the text in their order
func renderTokens(b *strings.Builder, toks []token, params map[int]int, order *[]int) {
	for i, t := range toks {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch t.kind {
		case tIdent, tNumber, tPunct:
			b.WriteString(t.text)
		case tQuoted:
			b.WriteString(quoteIdent(t.text))
		case tString:
			b.WriteByte('\'')
			b.WriteString(strings.ReplaceAll(t.text, "'", "''"))
			b.WriteByte('\'')
		case tParam:
			n, _ := paramIndex(t)
			k, ok := params[n]
			if !ok {
				k = len(params)
				params[n] = k
				*order = append(*order, n)
			}
			b.WriteString("$" + strconv.Itoa(k+1))
		}
	}
}

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
