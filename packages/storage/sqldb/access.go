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
	"math"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Row is a row of a table that a transaction reads or writes, named by the values of a unique key
// of the table. A Row with no key is the whole table.
type Row struct {
	Table string
	Key   string
}

// Write is a statement a transaction wrote with, as it ran, and the rows it wrote
type Write struct {
	SQL   string
	Vars  []any
	Table string
	Rows  []Row
	// ID is the id of the row an insert of a contract wrote; Generated tells it is the next id of
	// the table, which the insert took
	ID        int64
	Generated bool
	// KeepsMax tells the write leaves the highest id of the table as it was
	KeepsMax bool
}

type savepointMark struct {
	name string
	at   int
}

// Access records what a transaction reads and writes in the database as it runs. A block runs its
// transactions speculatively, each in a transaction of its own on a snapshot of the database at
// the start of the block, and keeps each run in the order of the block when nothing the run read
// was written by the transactions before it: the statements of the run then write in the
// transaction of the block what the transaction would have written there.
//
// A speculative record has the rows and tables the run read, the statements that wrote, with the
// rows each wrote, the highest ids of the tables the run saw, and the reason, if any, that the run
// cannot stand whatever the transactions before it did. A record that is not speculative has the
// writes only: those of a transaction the block runs in its own transaction.
//
// The statements of a transaction run one after the other, so a record needs no lock.
type Access struct {
	speculative bool
	catalog     *Catalog
	serial      string
	reads       map[Row]struct{}
	readAll     bool
	log         []Write
	marks       []savepointMark
	maxIDs      map[string]int64
	offsets     map[string]int64
	declared    *Write
	quiet       int
}

// NewAccess is a record of the statements of a transaction. offsets are, by table, the number of
// the next ids the transactions before it take, which the run adds to the highest ids it sees.
func NewAccess(speculative bool, catalog *Catalog, offsets map[string]int64) *Access {
	return &Access{
		speculative: speculative,
		catalog:     catalog,
		reads:       make(map[Row]struct{}),
		maxIDs:      make(map[string]int64),
		offsets:     offsets,
	}
}

// Serial is the reason the run cannot stand, if any: the transaction runs in the transaction of
// the block, and so do the transactions after it
func (a *Access) Serial() string { return a.serial }

// Writes are the statements that wrote and stand, in their order
func (a *Access) Writes() []Write { return a.log }

// MaxIDs are the highest ids of the tables the run saw first: each is the highest id of its table
// in the transaction of the block when the run stands
func (a *Access) MaxIDs() map[string]int64 { return a.maxIDs }

// Inserts is, by table, the number of the next ids the writes took
func (a *Access) Inserts() map[string]int64 {
	n := make(map[string]int64)
	for _, w := range a.log {
		if w.Generated {
			n[w.Table]++
		}
	}
	return n
}

func (a *Access) markSerial(reason string) {
	if a.serial == "" {
		a.serial = reason
	}
}

func (a *Access) read(r Row) {
	if a.speculative && a.quiet == 0 {
		a.reads[r] = struct{}{}
	}
}

// observeMaxID is the highest id of the table the run sees, of the highest id raw it has: the first
// is raw with the next ids the transactions before it take, and the writes of the run move it up
func (a *Access) observeMaxID(table string, raw int64) int64 {
	table = a.catalog.canon(table, false)
	if base, ok := a.maxIDs[table]; ok {
		return max(raw, base)
	}
	base := raw + a.offsets[table]
	a.maxIDs[table] = base
	return base
}

// Written is the set of the rows the transactions of a block that stand have written
type Written struct {
	rows   map[Row]struct{}
	tables map[string]struct{}
	whole  map[string]struct{}
}

func NewWritten() *Written {
	return &Written{rows: make(map[Row]struct{}), tables: make(map[string]struct{}), whole: make(map[string]struct{})}
}

// Add adds a row the block writes
func (w *Written) Add(r Row) {
	w.tables[r.Table] = struct{}{}
	if r.Key == "" {
		w.whole[r.Table] = struct{}{}
		return
	}
	w.rows[r] = struct{}{}
}

// AddWrites adds the rows of the writes of a transaction
func (w *Written) AddWrites(a *Access) {
	for _, wr := range a.log {
		for _, r := range wr.Rows {
			w.Add(r)
		}
	}
}

// Conflict is a row the run read that the block wrote, if any
func (w *Written) Conflict(a *Access) (Row, bool) {
	if len(w.tables) == 0 {
		return Row{}, false
	}
	if a.readAll {
		return Row{}, true
	}
	for r := range a.reads {
		if r.Key == "" {
			if _, ok := w.tables[r.Table]; ok {
				return r, true
			}
			continue
		}
		if _, ok := w.whole[r.Table]; ok {
			return r, true
		}
		if _, ok := w.rows[r]; ok {
			return r, true
		}
	}
	return Row{}, false
}

// accessKey keys, in the context of a database transaction, the record of its statements
type accessKey struct{}

type accessSlot struct{ p atomic.Pointer[Access] }

func withAccess(ctx context.Context) (context.Context, *accessSlot) {
	s := &accessSlot{}
	return context.WithValue(ctx, accessKey{}, s), s
}

// SetAccess makes a record of the statements of the transaction from now on; nil records none
func (tr *DbTransaction) SetAccess(a *Access) {
	if tr != nil && tr.access != nil {
		tr.access.p.Store(a)
	}
}

// Access is the record of the statements of the transaction, if any
func (tr *DbTransaction) Access() *Access {
	if tr == nil || tr.access == nil {
		return nil
	}
	return tr.access.p.Load()
}

// Speculating tells the transaction runs speculatively, on a snapshot of the start of a block
func (tr *DbTransaction) Speculating() bool {
	a := tr.Access()
	return a != nil && a.speculative
}

// MarkSerial makes the speculative run of the transaction fail to stand, for the reason: it does
// what the snapshot cannot tell, or changes the state of the node
func (tr *DbTransaction) MarkSerial(reason string) {
	if a := tr.Access(); a != nil {
		a.markSerial(reason)
	}
}

// RecordRead records a read of a row the database does not hold: the outputs of an account the
// block keeps in memory
func (tr *DbTransaction) RecordRead(r Row) {
	if a := tr.Access(); a != nil {
		a.read(r)
	}
}

// UniqueColumns are the columns of the unique keys of the table the record names rows by; none for
// a transaction without a record
func (tr *DbTransaction) UniqueColumns(table string) []string {
	a := tr.Access()
	if a == nil {
		return nil
	}
	var cols []string
	for _, key := range a.catalog.keys(a.catalog.canon(table, false)) {
		for _, c := range key {
			if !slices.Contains(cols, c) {
				cols = append(cols, c)
			}
		}
	}
	return cols
}

// DeclareWrite names the rows that the next statement writing the table writes: rows gives the
// value of each column of each row. id is the id of a row the statement inserts, generated tells
// it is the next id of the table, and keepsMax tells the statement leaves the highest id of the
// table as it is.
func (tr *DbTransaction) DeclareWrite(table string, rows []func(col string) (string, bool), id int64, generated, keepsMax bool) {
	a := tr.Access()
	if a == nil {
		return
	}
	t := a.catalog.canon(table, false)
	w := &Write{Table: t, ID: id, Generated: generated, KeepsMax: keepsMax}
	keys := a.catalog.keys(t)
	for _, value := range rows {
		named := len(keys) > 0
		var rs []Row
		for _, key := range keys {
			vals := make([]string, len(key))
			for i, c := range key {
				v, ok := value(c)
				if !ok {
					named = false
					break
				}
				vals[i] = canonText(v)
			}
			if !named {
				break
			}
			rs = append(rs, Row{t, rowKey(key, vals)})
		}
		if !named {
			rs = []Row{{Table: t}}
		}
		w.Rows = append(w.Rows, rs...)
	}
	if len(rows) == 0 {
		w.Rows = []Row{{Table: t}}
	}
	a.declared = w
}

func rowKey(cols, vals []string) string {
	return strings.Join(cols, ",") + "=" + strings.Join(vals, "\x1f")
}

// canonText is the value of a key in a form that names it whatever its type: an integer in decimal,
// a string without its trailing blanks. Two values of a form name one row at most.
func canonText(s string) string {
	s = strings.TrimRight(s, " ")
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return strconv.FormatInt(n, 10)
	}
	return s
}

// canonValue is canonText of a value bound to a statement; false for a value that names no row
func canonValue(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", false
	case string:
		return canonText(x), true
	case []byte:
		return canonText(string(x)), true
	case int:
		return strconv.FormatInt(int64(x), 10), true
	case int8:
		return strconv.FormatInt(int64(x), 10), true
	case int16:
		return strconv.FormatInt(int64(x), 10), true
	case int32:
		return strconv.FormatInt(int64(x), 10), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case uint:
		return strconv.FormatUint(uint64(x), 10), true
	case uint8:
		return strconv.FormatUint(uint64(x), 10), true
	case uint16:
		return strconv.FormatUint(uint64(x), 10), true
	case uint32:
		return strconv.FormatUint(uint64(x), 10), true
	case uint64:
		return strconv.FormatUint(x, 10), true
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1<<53 {
			return strconv.FormatInt(int64(x), 10), true
		}
		return strconv.FormatFloat(x, 'g', -1, 64), true
	case decimal.Decimal:
		return canonText(x.String()), true
	case bool:
		return strconv.FormatBool(x), true
	case sql.NamedArg:
		return canonValue(x.Value)
	case driver.Valuer:
		dv, err := x.Value()
		if err != nil {
			return "", false
		}
		return canonValue(dv)
	}
	return canonText(fmt.Sprint(v)), true
}

// Catalog is what the record of the statements of a block knows of the relations of the database:
// the tables, the partitions of each partitioned table, the unique keys of each table that it names
// rows by, and the functions of the chain
type Catalog struct {
	tables  map[string]*relation
	parents map[string]string
	funcs   map[string]bool
}

type relation struct {
	view bool
	keys [][]string
}

// keyTypes are the types of the columns of the keys the record names rows by: the types whose
// values canonValue names
var keyTypes = []string{"int2", "int4", "int8", "text", "varchar", "bpchar"}

// LoadCatalog reads the catalog of the database as the transaction sees it
func LoadCatalog(tr *DbTransaction) (*Catalog, error) {
	c := newCatalog()
	var rels []struct {
		Name   string
		Kind   string
		Parent string
	}
	if err := GetDB(tr).Raw(`SELECT c.relname AS name, c.relkind::text AS kind, coalesce(p.relname, '') AS parent
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_inherits i ON i.inhrelid = c.oid LEFT JOIN pg_class p ON p.oid = i.inhparent
		WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f')
			AND (n.oid = current_schema()::regnamespace OR i.inhparent IS NOT NULL)`).Scan(&rels).Error; err != nil {
		return nil, err
	}
	for _, r := range rels {
		if r.Parent != "" {
			c.parents[r.Name] = r.Parent
			continue
		}
		c.tables[r.Name] = &relation{view: r.Kind == "v" || r.Kind == "m"}
	}
	var keys []struct {
		Name    string
		Columns string
		Types   string
	}
	if err := GetDB(tr).Raw(`SELECT c.relname AS name, string_agg(a.attname, ',' ORDER BY k.ord) AS columns,
			string_agg(t.typname, ',' ORDER BY k.ord) AS types
		FROM pg_index x JOIN pg_class c ON c.oid = x.indrelid
		CROSS JOIN LATERAL unnest(x.indkey) WITH ORDINALITY AS k(attnum, ord)
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum
		JOIN pg_type t ON t.oid = a.atttypid
		WHERE x.indisunique AND x.indpred IS NULL AND x.indexprs IS NULL
			AND c.relnamespace = current_schema()::regnamespace
		GROUP BY x.indexrelid, c.relname
		ORDER BY c.relname, x.indexrelid`).Scan(&keys).Error; err != nil {
		return nil, err
	}
	for _, k := range keys {
		rel := c.tables[k.Name]
		if rel == nil || !allIn(strings.Split(k.Types, ","), keyTypes) {
			continue
		}
		rel.keys = append(rel.keys, strings.Split(k.Columns, ","))
	}
	var funcs []string
	if err := GetDB(tr).Raw(`SELECT DISTINCT p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')`).Scan(&funcs).Error; err != nil {
		return nil, err
	}
	for _, f := range funcs {
		c.funcs[f] = true
	}
	return c, nil
}

func newCatalog() *Catalog {
	return &Catalog{tables: make(map[string]*relation), parents: make(map[string]string), funcs: make(map[string]bool)}
}

func allIn(xs, set []string) bool {
	for _, x := range xs {
		if !slices.Contains(set, x) {
			return false
		}
	}
	return true
}

// canon is the table a name in a statement names: the partitioned table of a partition. quoted
// tells the name was quoted, and keeps its case.
func (c *Catalog) canon(name string, quoted bool) string {
	if !quoted {
		if strings.HasPrefix(name, `"`) && strings.HasSuffix(name, `"`) && len(name) > 1 {
			name = name[1 : len(name)-1]
		} else {
			name = strings.ToLower(name)
		}
	}
	if c == nil {
		return name
	}
	if p, ok := c.parents[name]; ok {
		return p
	}
	return name
}

func (c *Catalog) keys(table string) [][]string {
	if c == nil {
		return nil
	}
	if rel := c.tables[table]; rel != nil {
		return rel.keys
	}
	return nil
}

// known tells the name is a table of the catalog: not a view, nor a relation it does not have
func (c *Catalog) known(table string) bool {
	if c == nil {
		return false
	}
	rel := c.tables[table]
	return rel != nil && !rel.view
}

// sideEffects are the functions of the database that change what a snapshot cannot roll back
var sideEffects = map[string]bool{
	"nextval": true, "setval": true, "pg_notify": true, "set_config": true, "txid_current": true,
	"pg_advisory_lock": true, "pg_advisory_xact_lock": true, "pg_try_advisory_lock": true,
	"pg_try_advisory_xact_lock": true, "pg_export_snapshot": true, "lo_import": true, "lo_export": true,
	"dblink": true, "dblink_exec": true,
}

// registerAccess records each statement of a transaction in the record of its statements, if any
func registerAccess(db *gorm.DB) error {
	record := func(db *gorm.DB) {
		if db.Statement.Context == nil {
			return
		}
		s, _ := db.Statement.Context.Value(accessKey{}).(*accessSlot)
		if s == nil {
			return
		}
		if a := s.p.Load(); a != nil {
			a.record(db.Statement.SQL.String(), db.Statement.Vars, db.Error != nil)
		}
	}
	cb := db.Callback()
	for _, err := range []error{
		cb.Create().After("gorm:create").Register("ibax:access_create", record),
		cb.Update().After("gorm:update").Register("ibax:access_update", record),
		cb.Delete().After("gorm:delete").Register("ibax:access_delete", record),
		cb.Query().After("gorm:query").Register("ibax:access_query", record),
		cb.Row().After("gorm:row").Register("ibax:access_row", record),
		cb.Raw().After("gorm:raw").Register("ibax:access_raw", record),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

// record records a statement that ran, with the values bound to it; failed tells it failed
func (a *Access) record(query string, vars []any, failed bool) {
	toks := tokenize(query)
	for len(toks) > 0 && toks[len(toks)-1].is(";") {
		toks = toks[:len(toks)-1]
	}
	if len(toks) == 0 {
		return
	}
	for _, t := range toks {
		if t.is(";") {
			a.markSerial("several statements in one")
			return
		}
	}
	switch toks[0].keyword() {
	case "savepoint":
		a.savepoint(savepointName(toks[1:]))
	case "rollback":
		if len(toks) > 1 && toks[1].keyword() == "to" {
			a.rollbackTo(savepointName(toks[2:]))
			return
		}
		a.markSerial("rollback")
	case "release":
		a.release(savepointName(toks[1:]))
	case "set", "show", "begin", "start", "commit", "end", "reset":
	case "select", "values", "table":
		if a.speculative && a.quiet == 0 {
			a.recordReads(toks, vars)
		}
	case "explain":
		if slices.ContainsFunc(toks, func(t token) bool { return t.keyword() == "analyze" }) {
			a.markSerial("explain analyze")
		}
	case "with":
		if slices.ContainsFunc(toks, func(t token) bool {
			k := t.keyword()
			return k == "insert" || k == "update" || k == "delete"
		}) {
			a.markSerial("a write in a common table expression")
			return
		}
		if a.speculative && a.quiet == 0 {
			a.recordReads(toks, vars)
		}
	case "insert", "update", "delete":
		a.recordWrite(toks, query, vars, failed)
	default:
		a.markSerial("statement " + toks[0].text)
	}
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

func (a *Access) savepoint(name string) {
	a.marks = append(a.marks, savepointMark{name: name, at: len(a.log)})
}

func (a *Access) lastMark(name string) int {
	for i := len(a.marks) - 1; i >= 0; i-- {
		if a.marks[i].name == name {
			return i
		}
	}
	return -1
}

func (a *Access) rollbackTo(name string) {
	a.declared = nil
	i := a.lastMark(name)
	if i < 0 {
		a.log = a.log[:0]
		a.marks = a.marks[:0]
		return
	}
	a.log = a.log[:a.marks[i].at]
	a.marks = a.marks[:i+1]
}

func (a *Access) release(name string) {
	if i := a.lastMark(name); i >= 0 {
		a.marks = a.marks[:i]
	}
}

// recordReads records the tables a statement reads, and the row of a read of one table by a
// unique key
func (a *Access) recordReads(toks []token, vars []any) {
	refs, ok := a.scanReads(toks, 0)
	if !ok {
		a.readAll = true
		return
	}
	if len(refs) == 1 && simpleSelect(toks) {
		if r, ok := a.rowOf(refs[0], toks, vars); ok {
			a.read(r)
			return
		}
	}
	for _, t := range refs {
		a.read(Row{Table: t})
	}
}

// recordWrite records a statement that writes: the rows it writes, the rows it reads to write them
// and the other tables it reads
func (a *Access) recordWrite(toks []token, query string, vars []any, failed bool) {
	declared := a.declared
	a.declared = nil
	table, end, ok := a.target(toks)
	if !ok || !a.catalog.known(table) {
		a.markSerial("a write to " + table)
		return
	}
	if a.speculative {
		refs, ok := a.scanReads(toks, end)
		if !ok {
			a.readAll = true
		}
		for _, t := range refs {
			a.read(Row{Table: t})
		}
	}
	w := Write{Table: table, Rows: []Row{{Table: table}}}
	if declared != nil && declared.Table == table {
		w = *declared
	}
	for _, r := range w.Rows {
		a.read(r)
	}
	if failed {
		return
	}
	if a.speculative {
		w.SQL, w.Vars = query, slices.Clone(vars)
	}
	a.log = append(a.log, w)
}

// target is the table a write statement writes and the index of the token after its name
func (a *Access) target(toks []token) (string, int, bool) {
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
	name, next, ok := a.name(toks, i)
	return name, next, ok
}

// name is the relation a dotted name at i names, and the index of the token after the name
func (a *Access) name(toks []token, i int) (string, int, bool) {
	if i >= len(toks) || !toks[i].isName() {
		return "", 0, false
	}
	last := toks[i]
	i++
	for i+1 < len(toks) && toks[i].is(".") && toks[i+1].isName() {
		last = toks[i+1]
		i += 2
	}
	return a.catalog.canon(last.text, last.kind == tQuoted), i, true
}

// systemName tells a name at i is of the catalog of the database, which only the changes of the
// schema write: a change of the schema does not run speculatively
func systemName(toks []token, i int) bool {
	if toks[i].kind != tIdent {
		return false
	}
	return toks[i].text == "pg_catalog" || toks[i].text == "information_schema" || strings.HasPrefix(toks[i].text, "pg_")
}

// scanReads is the tables the tokens from start read: the relations of the FROM, JOIN and USING
// clauses. false tells the statement may read what the tables do not tell: a view, a relation
// the catalog does not have, a function of the chain, or a clause the scan does not follow.
func (a *Access) scanReads(toks []token, start int) ([]string, bool) {
	var refs []string
	// fromList tells, by depth of parentheses, that the tokens are in a list of relations
	fromList := []bool{false}
	expect := false
	ok := true
	for i := start; i < len(toks); i++ {
		t := toks[i]
		depth := len(fromList) - 1
		if t.kind == tIdent && i+1 < len(toks) && toks[i+1].is("(") {
			if sideEffects[t.text] {
				a.markSerial("function " + t.text)
			}
			if a.catalog != nil && a.catalog.funcs[t.text] {
				a.markSerial("function " + t.text + " of the chain")
			}
		}
		switch {
		case t.is("("):
			inner := expect
			expect = false
			fromList = append(fromList, inner)
			if inner {
				if i+1 < len(toks) {
					switch toks[i+1].keyword() {
					case "select", "with", "values":
						fromList[depth+1] = false
					default:
						expect = true
					}
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
		case "from":
			fromList[depth] = true
			expect = true
			continue
		case "join":
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
			"returning", "set", "fetch", "values", "select", "into":
			fromList[depth] = false
			expect = false
			continue
		case "for":
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
			ok = false
			continue
		}
		if systemName(toks, i) {
			_, next, _ := a.name(toks, i)
			i = next - 1
			continue
		}
		name, next, _ := a.name(toks, i)
		if next < len(toks) && toks[next].is("(") {
			// a function of the FROM clause reads its arguments; a function of the chain may read
			// any table
			if a.catalog != nil && a.catalog.funcs[name] {
				ok = false
			}
			i = next - 1
			continue
		}
		i = next - 1
		if !a.catalog.known(name) {
			ok = false
			continue
		}
		if !slices.Contains(refs, name) {
			refs = append(refs, name)
		}
	}
	return refs, ok
}

// simpleSelect tells a statement selects from one relation, with no join, subquery nor set
// operation
func simpleSelect(toks []token) bool {
	if toks[0].keyword() != "select" {
		return false
	}
	depth := 0
	inFrom := false
	for _, t := range toks[1:] {
		switch t.keyword() {
		case "select", "union", "intersect", "except", "with", "join", "lateral":
			return false
		}
		switch {
		case t.is("("):
			depth++
		case t.is(")"):
			depth--
		case depth > 0:
		case t.keyword() == "from":
			inFrom = true
		case t.is(","):
			if inFrom {
				return false
			}
		case t.keyword() == "where", t.keyword() == "group", t.keyword() == "order", t.keyword() == "limit",
			t.keyword() == "for", t.keyword() == "offset", t.keyword() == "fetch", t.keyword() == "having",
			t.keyword() == "window":
			inFrom = false
		}
	}
	return true
}

// rowOf is the row of the table a statement reads, when its WHERE clause is a conjunction with
// a term col = value for each column of a unique key of the table
func (a *Access) rowOf(table string, toks []token, vars []any) (Row, bool) {
	keys := a.catalog.keys(table)
	if len(keys) == 0 {
		return Row{}, false
	}
	where := -1
	depth := 0
	for i, t := range toks {
		switch {
		case t.is("("):
			depth++
		case t.is(")"):
			depth--
		case depth == 0 && t.keyword() == "where":
			where = i
		}
		if where >= 0 {
			break
		}
	}
	if where < 0 {
		return Row{}, false
	}
	end := len(toks)
	depth = 0
	for i := where + 1; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.is("("):
			depth++
		case t.is(")"):
			depth--
		case depth == 0:
			switch t.keyword() {
			case "order", "limit", "group", "for", "offset", "fetch", "having", "window", "returning":
				end = i
			}
		}
		if end != len(toks) {
			break
		}
	}
	eqs, ok := conjunction(toks[where+1:end], vars)
	if !ok {
		return Row{}, false
	}
	for _, key := range keys {
		vals := make([]string, len(key))
		covered := true
		for i, c := range key {
			v, ok := eqs[c]
			if !ok {
				covered = false
				break
			}
			vals[i] = v
		}
		if covered {
			return Row{table, rowKey(key, vals)}, true
		}
	}
	return Row{}, false
}

// conjunction is, for a condition that is a conjunction, the columns its terms col = value hold to
// a value. false tells the condition is no conjunction.
func conjunction(toks []token, vars []any) (map[string]string, bool) {
	eqs := make(map[string]string)
	var terms [][]token
	depth, from := 0, 0
	between := false
	for i, t := range toks {
		switch {
		case t.is("("):
			depth++
		case t.is(")"):
			depth--
		case depth == 0:
			switch t.keyword() {
			case "or":
				return nil, false
			case "between":
				between = true
			case "and":
				if between {
					between = false
					continue
				}
				terms = append(terms, toks[from:i])
				from = i + 1
			}
		}
	}
	terms = append(terms, toks[from:])
	for _, term := range terms {
		if len(term) >= 2 && term[0].is("(") && closes(term) {
			if inner, ok := conjunction(term[1:len(term)-1], vars); ok {
				for c, v := range inner {
					eqs[c] = v
				}
			}
			continue
		}
		if c, v, ok := equality(term, vars); ok {
			if _, seen := eqs[c]; !seen {
				eqs[c] = v
			}
		}
	}
	return eqs, true
}

// closes tells the parenthesis that opens the tokens closes at their end
func closes(toks []token) bool {
	depth := 0
	for i, t := range toks {
		switch {
		case t.is("("):
			depth++
		case t.is(")"):
			depth--
			if depth == 0 {
				return i == len(toks)-1
			}
		}
	}
	return false
}

// equality is the column and the value of a term col = value or value = col
func equality(term []token, vars []any) (string, string, bool) {
	eq := slices.IndexFunc(term, func(t token) bool { return t.is("=") })
	if eq < 0 {
		return "", "", false
	}
	left, right := term[:eq], term[eq+1:]
	if c, ok := columnName(left); ok {
		if v, ok := value(right, vars); ok {
			return c, v, true
		}
	}
	if c, ok := columnName(right); ok {
		if v, ok := value(left, vars); ok {
			return c, v, true
		}
	}
	return "", "", false
}

// columnName is the column a dotted name names, without its qualifier
func columnName(toks []token) (string, bool) {
	if len(toks) == 0 || len(toks)%2 == 0 {
		return "", false
	}
	for i, t := range toks {
		if i%2 == 1 {
			if !t.is(".") {
				return "", false
			}
		} else if !t.isName() || t.isKeyword() {
			return "", false
		}
	}
	return toks[len(toks)-1].text, true
}

// value is the value of a placeholder or a literal
func value(toks []token, vars []any) (string, bool) {
	if len(toks) != 1 {
		return "", false
	}
	t := toks[0]
	switch t.kind {
	case tParam:
		n, err := strconv.Atoi(t.text[1:])
		if err != nil || n < 1 || n > len(vars) {
			return "", false
		}
		return canonValue(vars[n-1])
	case tNumber:
		return canonText(t.text), true
	case tString:
		return canonText(t.text), true
	}
	return "", false
}

const (
	tIdent = iota
	tQuoted
	tString
	tNumber
	tParam
	tPunct
)

type token struct {
	kind int
	text string
}

func (t token) is(p string) bool { return t.kind == tPunct && t.text == p }

func (t token) isName() bool { return t.kind == tIdent || t.kind == tQuoted }

// keyword is the word of an unquoted identifier
func (t token) keyword() string {
	if t.kind != tIdent {
		return ""
	}
	return t.text
}

// isKeyword tells an unquoted identifier is a reserved word of a condition
func (t token) isKeyword() bool {
	switch t.keyword() {
	case "and", "or", "not", "null", "true", "false", "in", "is", "like", "ilike", "between", "any", "all",
		"some", "exists", "case", "when", "then", "else", "end", "select", "from", "where":
		return true
	}
	return false
}

// tokenize splits a statement into its tokens: unquoted identifiers in lower case, quoted ones,
// strings, numbers, placeholders and punctuation; comments are left out
func tokenize(s string) []token {
	var toks []token
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			i++
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				i = len(s)
			} else {
				i += end + 4
			}
		case c == '"':
			var b strings.Builder
			j := i + 1
			for j < len(s) {
				if s[j] == '"' {
					if j+1 < len(s) && s[j+1] == '"' {
						b.WriteByte('"')
						j += 2
						continue
					}
					break
				}
				b.WriteByte(s[j])
				j++
			}
			toks = append(toks, token{tQuoted, b.String()})
			i = j + 1
		case c == '\'':
			str, next := quotedString(s, i, false)
			toks = append(toks, token{tString, str})
			i = next
		case (c == 'e' || c == 'E') && i+1 < len(s) && s[i+1] == '\'':
			str, next := quotedString(s, i+1, true)
			toks = append(toks, token{tString, str})
			i = next
		case c == '$' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			j := i + 1
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			toks = append(toks, token{tParam, s[i:j]})
			i = j
		case c == '$':
			// a dollar-quoted string: $tag$ ... $tag$
			j := i + 1
			for j < len(s) && s[j] != '$' && isIdentChar(s[j]) {
				j++
			}
			if j < len(s) && s[j] == '$' {
				tag := s[i : j+1]
				end := strings.Index(s[j+1:], tag)
				if end < 0 {
					toks = append(toks, token{tString, s[j+1:]})
					i = len(s)
				} else {
					toks = append(toks, token{tString, s[j+1 : j+1+end]})
					i = j + 1 + end + len(tag)
				}
				continue
			}
			toks = append(toks, token{tPunct, "$"})
			i++
		case c >= '0' && c <= '9' || c == '.' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			j := i
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.' || s[j] == 'e' || s[j] == 'E' ||
				(s[j] == '+' || s[j] == '-') && j > i && (s[j-1] == 'e' || s[j-1] == 'E')) {
				j++
			}
			toks = append(toks, token{tNumber, s[i:j]})
			i = j
		case isIdentStart(c):
			j := i
			for j < len(s) && isIdentChar(s[j]) {
				j++
			}
			toks = append(toks, token{tIdent, strings.ToLower(s[i:j])})
			i = j
		case strings.IndexByte("(),;.[]", c) >= 0:
			toks = append(toks, token{tPunct, string(c)})
			i++
		case c == ':' && i+1 < len(s) && s[i+1] == ':':
			toks = append(toks, token{tPunct, "::"})
			i += 2
		case strings.IndexByte("+-*/<>=~!@#%^&|`?:", c) >= 0:
			j := i
			for j < len(s) && strings.IndexByte("+-*/<>=~!@#%^&|`?", s[j]) >= 0 {
				if j > i && s[j] == '-' && j+1 < len(s) && s[j+1] == '-' {
					break
				}
				j++
			}
			if j == i {
				j++
			}
			toks = append(toks, token{tPunct, s[i:j]})
			i = j
		default:
			toks = append(toks, token{tPunct, string(c)})
			i++
		}
	}
	return toks
}

// quotedString is the string quoted at i, and the index after it; escapes tells backslashes escape
func quotedString(s string, i int, escapes bool) (string, int) {
	var b strings.Builder
	j := i + 1
	for j < len(s) {
		switch {
		case escapes && s[j] == '\\' && j+1 < len(s):
			b.WriteByte(s[j+1])
			j += 2
			continue
		case s[j] == '\'':
			if j+1 < len(s) && s[j+1] == '\'' {
				b.WriteByte('\'')
				j += 2
				continue
			}
			return b.String(), j + 1
		}
		b.WriteByte(s[j])
		j++
	}
	return b.String(), j
}

func isIdentStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 0x80
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9' || c == '$'
}

// ExportSnapshot exports the snapshot of the transaction, for the speculative runs of the
// transactions of a block. false tells the transaction has written, and its snapshot would not
// show what it wrote.
func (tr *DbTransaction) ExportSnapshot() (string, bool, error) {
	var r struct {
		ID    string
		Clean bool
	}
	err := tr.conn.Raw(`SELECT pg_export_snapshot() AS id, txid_current_if_assigned() IS NULL AS clean`).Scan(&r).Error
	return r.ID, r.Clean, err
}

// StartSnapshotTransaction begins a transaction that sees the database as the snapshot an
// exporting transaction exported shows it
func StartSnapshotTransaction(snapshot string) (*DbTransaction, error) {
	if snapshot == "" || strings.Trim(snapshot, "0123456789ABCDEFabcdef-") != "" {
		return nil, fmt.Errorf("snapshot %q", snapshot)
	}
	ctx, statements := withStatementCount()
	ctx, cache := withTxCache(ctx)
	ctx, access := withAccess(ctx)
	conn := DBConn.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if conn.Error != nil {
		return nil, conn.Error
	}
	if err := conn.Exec(`SET TRANSACTION SNAPSHOT '` + snapshot + `'`).Error; err != nil {
		conn.Rollback()
		return nil, err
	}
	if err := setupConnOptions(conn); err != nil {
		conn.Rollback()
		return nil, err
	}
	return &DbTransaction{conn: conn, statements: statements, cache: cache, access: access}, nil
}

// ReplayError is a write of a speculative run that failed in the transaction of the block
type ReplayError struct{ Err error }

func (e *ReplayError) Error() string { return "replaying a speculative run: " + e.Err.Error() }

func (e *ReplayError) Unwrap() error { return e.Err }

// Replay runs in the transaction the writes of a speculative run that stands, in their order
func (tr *DbTransaction) Replay(a *Access) error {
	pool, ctx := tr.conn.Statement.ConnPool, tr.conn.Statement.Context
	for _, w := range a.log {
		prev, had := tr.CachedMaxID(w.Table)
		if _, err := pool.ExecContext(ctx, w.SQL, w.Vars...); err != nil {
			return &ReplayError{err}
		}
		if tr.statements != nil {
			tr.statements.Add(1)
		}
		if tr.cache != nil {
			tr.cache.forget(w.Table)
			if had && (w.ID > 0 || w.KeepsMax) {
				tr.KeepMaxID(w.Table, max(prev, w.ID))
			}
		}
	}
	return nil
}
