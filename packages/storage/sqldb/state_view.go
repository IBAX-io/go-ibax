/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"math"
	"slices"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// TxView is the view of a transaction on the state of a block. A view run in parallel records
// what it reads, for its commit to tell the reads hold; a statement it cannot run fails with
// ErrNeedsOrder. A view run in the order of the block reads the state as the block committed it,
// and runs in PostgreSQL what the state cannot run in memory.
type TxView struct {
	st       *BlockState
	index    int
	ordered  bool
	contract string
	// conn is the connection a view run in parallel holds, which the worker pins for the block
	conn   *sql.Conn
	sysGen uint64
	own    map[string]*viewTable
	order  []string
	reads  []stateRead
	ids    []idRead
	utxo   []utxoRead
	// idBase is, by table, the highest id the first read of it gave
	idBase map[string]int64
	undo   []undoEntry
	marks  []viewMark
	// needsOrder tells the view met what it cannot run in parallel; restart that the view run
	// in order has to run again once the state put the tables of pending, or all of them, in
	// PostgreSQL
	needsOrder, restart, barrier bool
	pending                      []string
	pendingAll                   bool
	reason                       string
}

// viewTable is what a view wrote of a table
type viewTable struct {
	ts    *tableState
	rows  map[string]*vchange
	order []string
	uniq  []map[string]string
}

// vchange is a row the view wrote: the row it saw before, and the row now, nil for a deleted one
type vchange struct {
	prev, row stateRow
}

// stateRead is a read of a view: of the rows of a table that hold a condition, of every row of the
// table for a nil condition, of every table for a nil table; seq is the commit it saw. A point
// read is of the row of the key of the primary key (ix -1) or of the unique index ix the
// condition requires.
type stateRead struct {
	ts    *tableState
	seq   uint64
	cond  *bcond
	point bool
	ix    int
	key   string
}

// idRead is a read of the highest id of a table: own is the highest id the view wrote
type idRead struct {
	ts         *tableState
	own, value int64
}

type undoEntry struct {
	vt       *viewTable
	pk       string
	had      bool
	old      vchange
	orderLen int
	uniq     []uniqUndo
}

type uniqUndo struct {
	j      int
	k      string
	had    bool
	prevPK string
}

// viewMark is a savepoint of the view: at is its entry of undo, pg tells it is in PostgreSQL too
type viewMark struct {
	name string
	at   int
	pg   bool
}

// NewOrderedView is a view of the transaction i of the block, which runs the contract in order,
// on the connection of the transaction of the block
func (st *BlockState) NewOrderedView(i int, contract string) *TxView {
	return st.newView(i, contract, true, nil)
}

// NewParallelView is a view of the transaction i of the block, which runs the contract in
// parallel, on the connection conn of its worker
func (st *BlockState) NewParallelView(i int, contract string, conn *sql.Conn) *TxView {
	return st.newView(i, contract, false, conn)
}

func (st *BlockState) newView(i int, contract string, ordered bool, conn *sql.Conn) *TxView {
	return &TxView{st: st, index: i, ordered: ordered, contract: contract, conn: conn, sysGen: st.sysGen.Load(),
		own: make(map[string]*viewTable), idBase: make(map[string]int64)}
}

// PinConn takes a connection of the pool for the views a worker runs in parallel through a
// block, so their statements do not take and give back a connection each; closing it gives it
// back to the pool
func PinConn(ctx context.Context) (*sql.Conn, error) {
	db, err := DBConn.DB()
	if err != nil {
		return nil, err
	}
	return db.Conn(ctx)
}

// Context is the context whose statements run in the view
func (v *TxView) Context(ctx context.Context) context.Context { return withState(ctx, v) }

// NeedsOrder tells the view met a statement it cannot run in parallel, and why
func (v *TxView) NeedsOrder() (bool, string) { return v.needsOrder, v.reason }

// Restart tells the view run in order has to run again
func (v *TxView) Restart() bool { return v.restart }

// Prepare puts in PostgreSQL the tables a view run in order needs there before it runs again
func (v *TxView) Prepare() error {
	if !v.pendingAll && len(v.pending) == 0 {
		return nil
	}
	return v.st.toPG(slices.Clone(v.pending), v.pendingAll)
}

// IDCounts are, by table, the ids the view took past the highest id it read first
func (v *TxView) IDCounts() map[string]int {
	ids := make(map[string]int, len(v.idBase))
	for name, base := range v.idBase {
		vt := v.own[name]
		if vt == nil {
			continue
		}
		top := base
		for _, vc := range vt.rows {
			if id, ok := vt.ts.rowID(vc.row); ok && id > top {
				top = id
			}
		}
		if top > base {
			ids[name] = int(top - base)
		}
	}
	return ids
}

// Ordered tells the view runs in the order of the block
func (v *TxView) Ordered() bool { return v.ordered }

// SetNeedsOrder fails the view run in parallel for a reason out of the database. A view run in
// order goes on, and is a barrier: the runs in parallel before it saw the node as it was.
func (v *TxView) SetNeedsOrder(reason string) error {
	if v.ordered {
		v.barrier = true
		return nil
	}
	return v.needs(reason)
}

// Barrier tells the view run in order changed the state of the node out of the database
func (v *TxView) Barrier() bool { return v.barrier }

// Transaction is the transaction whose statements run in the view: on the connection of the
// transaction of the block for a view run in order, on the connection of its worker for one run
// in parallel.
// Each has a cache of its own.
func (v *TxView) Transaction() *DbTransaction {
	ctx, statements := withStatementCount()
	ctx, cache := withTxCache(ctx)
	ctx = v.Context(ctx)
	var conn *gorm.DB
	if v.ordered {
		conn = v.st.block.conn.WithContext(ctx)
	} else {
		conn = DBConn.Session(&gorm.Session{SkipDefaultTransaction: true, Context: ctx})
		conn.Statement.ConnPool = v.conn
	}
	return &DbTransaction{conn: conn, statements: statements, cache: cache, view: v}
}

func (v *TxView) needs(reason string) error {
	if !v.needsOrder {
		v.needsOrder, v.reason = true, reason
		v.st.fallback("parallel: " + reason)
	}
	return ErrNeedsOrder
}

func (v *TxView) restartFor(names []string, all bool, reason string) error {
	v.restart, v.reason = true, reason
	v.pending = append(v.pending, names...)
	v.pendingAll = v.pendingAll || all
	v.st.fallback("order: " + reason)
	return errRestart
}

func (v *TxView) query(ctx context.Context, c *stateConn, query string, args []driver.NamedValue) (driver.Rows, error) {
	rows, _, err := v.run(ctx, c, query, args, true)
	return rows, err
}

func (v *TxView) exec(ctx context.Context, c *stateConn, query string, args []driver.NamedValue) (driver.Result, error) {
	_, res, err := v.run(ctx, c, query, args, false)
	return res, err
}

// run runs a statement in the view
func (v *TxView) run(ctx context.Context, c *stateConn, query string, args []driver.NamedValue, isQuery bool) (driver.Rows, driver.Result, error) {
	p, err := schema.plan(ctx, c.pg, query)
	if err != nil {
		return nil, nil, err
	}
	if p.kind == stSavepoint || p.kind == stRollbackTo || p.kind == stRelease {
		// a run that has to run again, or in order, still takes back what it wrote, in
		// PostgreSQL too
		return v.savepoint(ctx, c, p, query, args, isQuery)
	}
	if v.needsOrder {
		return nil, nil, ErrNeedsOrder
	}
	if v.restart {
		return nil, nil, errRestart
	}
	switch p.kind {
	case stSelect, stMaxID, stInsert, stUpdate, stDelete:
		rows, n, err := v.planned(ctx, c, p, args)
		if err == errUnsupported {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if isQuery {
			if rows == nil {
				rows = emptyRows()
			}
			return rows, nil, nil
		}
		if rows != nil {
			n = int64(len(rows.rows))
		}
		return nil, driver.RowsAffected(n), nil
	}
	return v.inPG(ctx, c, p, query, args, isQuery)
}

// tableOf is the state of a table; nil for a relation the state does not keep
func (v *TxView) tableOf(ctx context.Context, c *stateConn, name string) (*tableState, error) {
	m, err := schema.meta(ctx, c.pg, name)
	if err != nil || m == nil {
		return nil, err
	}
	return v.st.table(ctx, c, m)
}

// ownChanged tells the view wrote rows of the table
func (v *TxView) ownChanged(name string) bool {
	vt := v.own[name]
	return vt != nil && len(vt.rows) > 0
}

// inPG runs a statement in PostgreSQL: in parallel, a read of tables the block has not changed;
// in order, any statement once the tables it reads and writes are in PostgreSQL
func (v *TxView) inPG(ctx context.Context, c *stateConn, p *stmt, query string, args []driver.NamedValue, isQuery bool) (driver.Rows, driver.Result, error) {
	reads, all, write, other := p.tables, p.allReads, "", false
	switch p.kind {
	case stOther:
		other = true
	case stWrite, stInsert, stUpdate, stDelete:
		write = p.table
	case stSelect, stMaxID:
		reads = []string{p.table}
	case stRollbackTo, stRelease:
		// a savepoint the view has not: PostgreSQL gives the error
		if !v.ordered {
			return nil, nil, v.needs("unknown savepoint")
		}
	}
	st := v.st
	if !v.ordered {
		switch {
		case other:
			return nil, nil, v.needs("statement")
		case write != "":
			return nil, nil, v.needs("write of " + write)
		}
		var states []*tableState
		for _, name := range reads {
			ts, err := v.tableOf(ctx, c, name)
			if err != nil {
				return nil, nil, err
			}
			if ts == nil {
				return nil, nil, v.needs("read of " + name)
			}
			states = append(states, ts)
		}
		st.mu.RLock()
		seq, allPG, lastChange := st.seq, st.allPG.Load(), st.lastChange
		clean := !allPG && (!all || lastChange == 0)
		for _, ts := range states {
			if ts.pg.Load() || ts.changed() || v.ownChanged(ts.meta.name) {
				clean = false
			}
		}
		st.mu.RUnlock()
		if all && len(v.order) > 0 {
			clean = false
		}
		if !clean {
			return nil, nil, v.needs("read of changed tables in PostgreSQL")
		}
		for _, ts := range states {
			v.reads = append(v.reads, stateRead{ts: ts, seq: seq})
		}
		if all {
			v.reads = append(v.reads, stateRead{seq: seq})
		}
		return v.cleanRun(ctx, c, query, args, isQuery)
	}
	if other {
		st.mu.Lock()
		allPG := st.allPG.Load()
		st.schemaChanged = true
		st.mu.Unlock()
		if !allPG {
			return nil, nil, v.restartFor(nil, true, "statement")
		}
		return v.pgRun(ctx, c, query, args, isQuery)
	}
	var pending []string
	st.mu.RLock()
	if all {
		for name, ts := range st.tables {
			if !ts.pg.Load() && (ts.changed() || v.ownChanged(name)) {
				pending = append(pending, name)
			}
		}
	}
	st.mu.RUnlock()
	need := func(name string) error {
		ts, err := v.tableOf(ctx, c, name)
		if err != nil || ts == nil {
			return err
		}
		dirty := !ts.pg.Load() && (ts.changed() || v.ownChanged(name))
		if dirty && !slices.Contains(pending, name) {
			pending = append(pending, name)
		}
		return nil
	}
	for _, name := range reads {
		if err := need(name); err != nil {
			return nil, nil, err
		}
	}
	var target *tableState
	if write != "" {
		if err := need(write); err != nil {
			return nil, nil, err
		}
		ts, err := v.tableOf(ctx, c, write)
		if err != nil {
			return nil, nil, err
		}
		if ts == nil {
			// a relation the state does not keep: every table goes to PostgreSQL
			if !st.allPG.Load() {
				return nil, nil, v.restartFor(nil, true, "write of "+write)
			}
		}
		target = ts
	}
	if len(pending) > 0 {
		return nil, nil, v.restartFor(pending, false, "statement in PostgreSQL on "+strings.Join(pending, ","))
	}
	if target != nil {
		if !target.pg.Load() {
			if len(v.marks) > 0 {
				// the savepoints of the view are not in PostgreSQL
				return nil, nil, v.restartFor([]string{write}, false, "write in PostgreSQL of "+write)
			}
			st.markPG(target)
		}
	}
	st.fallback("order: statement in PostgreSQL")
	return v.pgRun(ctx, c, query, args, isQuery)
}

// cleanRun runs a select of tables the block has not changed: the block keeps the answer of
// PostgreSQL for its other transactions
func (v *TxView) cleanRun(ctx context.Context, c *stateConn, query string, args []driver.NamedValue, isQuery bool) (driver.Rows, driver.Result, error) {
	if !isQuery {
		return v.pgRun(ctx, c, query, args, isQuery)
	}
	key := readKey(query, args)
	if r, ok := v.st.cleanRead(key); ok {
		return &cleanRows{r: r}, nil, nil
	}
	rows, err := c.pg.QueryContext(ctx, query, args)
	if err != nil {
		return nil, nil, err
	}
	r, err := captureRows(rows)
	if err != nil {
		return nil, nil, err
	}
	v.st.keepCleanRead(key, r)
	return &cleanRows{r: r}, nil, nil
}

// pgRun runs a statement in PostgreSQL on the connection
func (v *TxView) pgRun(ctx context.Context, c *stateConn, query string, args []driver.NamedValue, isQuery bool) (driver.Rows, driver.Result, error) {
	if isQuery {
		rows, err := c.pg.QueryContext(ctx, query, args)
		return rows, nil, err
	}
	res, err := c.pg.ExecContext(ctx, query, args)
	return nil, res, err
}

// anyPG tells a table is in PostgreSQL: a view run in order then has its savepoints there too
func (st *BlockState) anyPG() bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if st.allPG.Load() {
		return true
	}
	for _, ts := range st.tables {
		if ts.pg.Load() {
			return true
		}
	}
	return false
}

// savepoint runs a statement on savepoints
func (v *TxView) savepoint(ctx context.Context, c *stateConn, p *stmt, query string, args []driver.NamedValue, isQuery bool) (driver.Rows, driver.Result, error) {
	last := -1
	for i := len(v.marks) - 1; i >= 0; i-- {
		if v.marks[i].name == p.name {
			last = i
			break
		}
	}
	if p.kind != stSavepoint && last < 0 {
		switch {
		case v.needsOrder:
			return nil, nil, ErrNeedsOrder
		case v.restart:
			return nil, nil, errRestart
		}
		return v.inPG(ctx, c, p, query, args, isQuery)
	}
	// a savepoint is in PostgreSQL when a view run in order makes it while a table is there,
	// and the statements on it then run there too
	pg := v.ordered && v.st.anyPG()
	if p.kind != stSavepoint {
		pg = v.marks[last].pg
	}
	if pg {
		if _, _, err := v.pgRun(ctx, c, query, args, false); err != nil {
			return nil, nil, err
		}
	}
	switch p.kind {
	case stSavepoint:
		v.marks = append(v.marks, viewMark{name: p.name, at: len(v.undo), pg: pg})
	case stRollbackTo:
		v.rollbackUndo(v.marks[last].at)
		v.marks = v.marks[:last+1]
	case stRelease:
		v.marks = v.marks[:last]
	}
	if isQuery {
		return emptyRows(), nil, nil
	}
	return nil, driver.RowsAffected(0), nil
}

// rollbackUndo takes back the writes of the view since the undo entry at
func (v *TxView) rollbackUndo(at int) {
	for i := len(v.undo) - 1; i >= at; i-- {
		e := v.undo[i]
		vt := e.vt
		for j := len(e.uniq) - 1; j >= 0; j-- {
			u := e.uniq[j]
			if u.had {
				vt.uniq[u.j][u.k] = u.prevPK
			} else {
				delete(vt.uniq[u.j], u.k)
			}
		}
		if e.had {
			*vt.rows[e.pk] = e.old
		} else {
			delete(vt.rows, e.pk)
		}
		vt.order = vt.order[:e.orderLen]
	}
	v.undo = v.undo[:at]
}

// viewTable is what the view wrote of a table, which it creates
func (v *TxView) viewTable(ts *tableState) *viewTable {
	name := ts.meta.name
	vt := v.own[name]
	if vt == nil {
		vt = &viewTable{ts: ts, rows: make(map[string]*vchange), uniq: make([]map[string]string, len(ts.uniq))}
		for j := range vt.uniq {
			vt.uniq[j] = make(map[string]string)
		}
		v.own[name] = vt
		v.order = append(v.order, name)
	}
	return vt
}

// setRow writes a row of the primary key in the view: prev is the row the view saw, row the new
// one, nil to delete it
func (v *TxView) setRow(vt *viewTable, pk string, prev, row stateRow) {
	m := vt.ts.meta
	old, had := vt.rows[pk]
	e := undoEntry{vt: vt, pk: pk, had: had, orderLen: len(vt.order)}
	oldRow := prev
	if had {
		e.old = *old
		oldRow = old.row
	}
	for j, ix := range vt.ts.uniqIx {
		if had && oldRow != nil {
			if k, ok := m.ukeyOf(ix, oldRow); ok && vt.uniq[j][k] == pk {
				e.uniq = append(e.uniq, uniqUndo{j: j, k: k, had: true, prevPK: pk})
				delete(vt.uniq[j], k)
			}
		}
		if row != nil {
			if k, ok := m.ukeyOf(ix, row); ok {
				prevPK, h := vt.uniq[j][k]
				e.uniq = append(e.uniq, uniqUndo{j: j, k: k, had: h, prevPK: prevPK})
				vt.uniq[j][k] = pk
			}
		}
	}
	if had {
		old.row = row
	} else {
		vt.rows[pk] = &vchange{prev: prev, row: row}
		vt.order = append(vt.order, pk)
	}
	v.undo = append(v.undo, e)
}

// planned runs a planned statement in memory; errUnsupported for one the state does not run so
func (v *TxView) planned(ctx context.Context, c *stateConn, p *stmt, args []driver.NamedValue) (*memRows, int64, error) {
	m, err := schema.meta(ctx, c.pg, p.table)
	if err != nil {
		return nil, 0, err
	}
	if m == nil || !m.readable {
		return nil, 0, errUnsupported
	}
	ts, err := v.st.table(ctx, c, m)
	if err != nil {
		return nil, 0, err
	}
	if ts.pg.Load() {
		return nil, 0, errUnsupported
	}
	b := &binder{args: args, row: m}
	var cond *bcond
	if p.where != nil {
		var ok bool
		if cond, ok = b.bindCond(p.where); !ok {
			return nil, 0, errUnsupported
		}
	}
	switch p.kind {
	case stMaxID:
		return v.maxID(ts, p)
	case stSelect:
		order, ok := bindOrder(m, p.order)
		if !ok {
			return nil, 0, errUnsupported
		}
		limit, ok := b.count(p.limit)
		if !ok {
			return nil, 0, errUnsupported
		}
		offset, ok := b.count(p.offset)
		if !ok {
			return nil, 0, errUnsupported
		}
		if offset < 0 {
			offset = 0
		}
		rows, err := v.selectRows(ctx, c, ts, p, args, cond, order, limit, offset)
		if err != nil {
			return nil, 0, err
		}
		for _, pc := range p.proj {
			if m.colIndex(pc.col) < 0 {
				return nil, 0, errUnsupported
			}
		}
		return project(m, p, rows), 0, nil
	}
	if !m.writable {
		return nil, 0, errUnsupported
	}
	switch p.kind {
	case stInsert:
		return v.insert(ctx, c, ts, p, b)
	case stUpdate:
		return v.update(ctx, c, ts, p, b, args, cond)
	}
	return v.delete(ctx, c, ts, p, args, cond)
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

// read records a read of the view
func (v *TxView) read(ts *tableState, seq uint64, cond *bcond) {
	if !v.ordered {
		v.reads = append(v.reads, stateRead{ts: ts, seq: seq, cond: cond})
	}
}

// readPoint records a read of the view of the row of a key of the primary key (j -1) or of the
// unique index j, by a condition that requires the key
func (v *TxView) readPoint(ts *tableState, seq uint64, cond *bcond, j int, key string) {
	if !v.ordered {
		v.reads = append(v.reads, stateRead{ts: ts, seq: seq, cond: cond, point: true, ix: j, key: key})
	}
}

// pointKey is the index and key a condition requires of a row by equality: -1 for the primary key
func pointKey(m *tableMeta, ts *tableState, cond *bcond) (int, string, bool) {
	eq := cond.equalities()
	if len(eq) == 0 {
		return 0, "", false
	}
	val := func(c string) *string { return eq[m.colIndex(c)] }
	if k, ok := m.keyOf(m.pk, val); ok && allIn(m, m.pk, eq) {
		return -1, k, true
	}
	for j, ix := range ts.uniqIx {
		cols := m.uniques[ix]
		if !allIn(m, cols, eq) {
			continue
		}
		if k, ok := m.keyOf(cols, val); ok {
			return j, k, true
		}
	}
	return 0, "", false
}

func allIn(m *tableMeta, cols []string, eq map[int]*string) bool {
	for _, c := range cols {
		if _, ok := eq[m.colIndex(c)]; !ok {
			return false
		}
	}
	return true
}

// selectRows are the rows of the table that hold the condition, in the order, past offset and up
// to limit (-1 for none)
func (v *TxView) selectRows(ctx context.Context, c *stateConn, ts *tableState, p *stmt, args []driver.NamedValue,
	cond *bcond, order []border, limit, offset int64) ([]stateRow, error) {
	m := ts.meta
	var rows []stateRow
	var err error
	if j, key, ok := pointKey(m, ts, cond); ok {
		rows, err = v.point(ctx, c, ts, cond, j, key)
	} else {
		rows, err = v.merge(ctx, c, ts, p, args, cond, order, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	if len(rows) > 1 {
		pkIdx, pkKinds := pkOrder(m)
		slices.SortStableFunc(rows, func(a, b stateRow) int { return compareRows(order, pkIdx, pkKinds, a, b) })
	}
	if offset > 0 {
		if offset >= int64(len(rows)) {
			return nil, nil
		}
		rows = rows[offset:]
	}
	if limit >= 0 && limit < int64(len(rows)) {
		rows = rows[:limit]
	}
	return rows, nil
}

// pkOrder are the columns of the primary key and the kinds that order them
func pkOrder(m *tableMeta) ([]int, []colKind) {
	idx := make([]int, len(m.pk))
	kinds := make([]colKind, len(m.pk))
	for j, c := range m.pk {
		idx[j] = m.colIndex(c)
		k := m.cols[idx[j]].typ.kind
		if k == kInt {
			k = kNumeric
		}
		kinds[j] = k
	}
	return idx, kinds
}

// point is the row of a key of the primary key (j -1) or of the unique index j, if it holds the
// condition
func (v *TxView) point(ctx context.Context, c *stateConn, ts *tableState, cond *bcond, j int, key string) ([]stateRow, error) {
	m := ts.meta
	vt := v.own[m.name]
	result := func(r stateRow) []stateRow {
		if r != nil && cond.matches(r) {
			return []stateRow{r}
		}
		return nil
	}
	if vt != nil {
		if j < 0 {
			if vc, ok := vt.rows[key]; ok {
				return result(vc.row), nil
			}
		} else if pk, ok := vt.uniq[j][key]; ok {
			return result(vt.rows[pk].row), nil
		}
	}
	st := v.st
	seq := st.pub.Load()
	pk, found := key, true
	if j >= 0 {
		pk, found = ts.uniq[j].load(key)
	}
	var r stateRow
	if found {
		r, found = ts.visible(pk)
	}
	v.readPoint(ts, seq, cond, j, key)
	if found {
		if vt != nil {
			if vc, ok := vt.rows[pk]; ok {
				r = vc.row
			}
		}
		return result(r), nil
	}
	cols := m.pk
	if j >= 0 {
		cols = m.uniques[ts.uniqIx[j]]
	}
	eq := cond.equalities()
	params := make([]any, len(cols))
	for i, col := range cols {
		params[i] = eq[m.colIndex(col)]
	}
	r, ok := ts.base.get(j, key)
	if !ok {
		base, err := fetchRows(ctx, c, selectAll(m, cols), params)
		if err != nil {
			return nil, err
		}
		r = nil
		if len(base) > 0 {
			r = base[0]
		}
		ts.base.put(j, key, r)
	}
	if r == nil {
		return nil, nil
	}
	pk = m.pkOf(r)
	if vt != nil {
		if vc, ok := vt.rows[pk]; ok {
			return result(vc.row), nil
		}
	}
	if cr, ok := ts.visible(pk); ok {
		r = cr
	}
	return result(r), nil
}

// merge are the rows of the table that hold the condition: the rows the state and the view
// changed, and the rows of PostgreSQL they did not
func (v *TxView) merge(ctx context.Context, c *stateConn, ts *tableState, p *stmt, args []driver.NamedValue,
	cond *bcond, order []border, limit, offset int64) ([]stateRow, error) {
	m := ts.meta
	bounded := limit >= 0 || offset > 0
	if bounded {
		_, kinds := pkOrder(m)
		for _, k := range kinds {
			if !ordered(k) {
				return nil, errUnsupported
			}
		}
	}
	vt := v.own[m.name]
	var rows []stateRow
	extra := int64(0)
	seq := v.st.pub.Load()
	ts.rows.each(func(pk string, rc *rowChange) bool {
		if vt != nil {
			if _, mine := vt.rows[pk]; mine {
				return true
			}
		}
		if rc.base != nil && cond.matches(rc.base) {
			extra++
		}
		if rc.row != nil && cond.matches(rc.row) {
			rows = append(rows, rc.row)
		}
		return true
	})
	if vt != nil {
		for pk, vc := range vt.rows {
			base := vc.prev
			if rc, ok := ts.rows.load(pk); ok {
				base = rc.base
			}
			if base != nil && cond.matches(base) {
				extra++
			}
			if vc.row != nil && cond.matches(vc.row) {
				rows = append(rows, vc.row)
			}
		}
	}
	v.read(ts, seq, cond)

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
	values := make([]any, len(porder))
	for i, n := range porder {
		if n >= len(args) {
			return nil, errUnsupported
		}
		values[i] = args[n].Value
	}
	query := b.String()
	key, keep := stmtKey(query, values)
	base, read := []stateRow(nil), false
	if keep {
		base, read = ts.base.scan(key)
	}
	if !read {
		var err error
		if base, err = fetchRows(ctx, c, query, values); err != nil {
			return nil, err
		}
		if keep {
			ts.base.putScan(key, base)
		}
	}
	for _, r := range base {
		pk := m.pkOf(r)
		if vt != nil {
			if _, mine := vt.rows[pk]; mine {
				continue
			}
		}
		if _, changed := ts.rows.load(pk); changed {
			continue
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// eqCond is the condition of equality of the columns with their values in a row
func eqCond(m *tableMeta, cols []string, r stateRow) *bcond {
	c := &bcond{op: cAnd}
	for _, col := range cols {
		i := m.colIndex(col)
		fam, _ := family(m.cols[i].typ, m.cols[i].typ)
		c.kids = append(c.kids, &bcond{op: cCmp, col: i, cmp: "=", fam: fam, vals: []*string{r[i]}})
	}
	return c
}

// taken tells a row of a primary key other than self has the key of the row in the unique index
// ix of the table, which the write of the row would violate
func (v *TxView) taken(ctx context.Context, c *stateConn, ts *tableState, ix int, row stateRow, self string) (bool, error) {
	m := ts.meta
	cols := m.uniques[ix]
	key, ok := m.keyOf(cols, func(col string) *string { return row[m.colIndex(col)] })
	if !ok {
		return false, nil
	}
	isPK := slices.Equal(cols, m.pk)
	j := slices.Index(ts.uniqIx, ix)
	vt := v.own[m.name]
	if vt != nil {
		if isPK {
			if vc, ok := vt.rows[key]; ok {
				return vc.row != nil && key != self, nil
			}
		} else if pk, ok := vt.uniq[j][key]; ok && pk != self {
			return true, nil
		}
	}
	mine := func(pk string) bool {
		if vt == nil {
			return false
		}
		_, ok := vt.rows[pk]
		return ok
	}
	seq := v.st.pub.Load()
	var found, decided bool
	if isPK {
		if r, ok := ts.visible(key); ok {
			found, decided = r != nil && key != self, true
		}
	} else if pk, ok := ts.uniq[j].load(key); ok && pk != self && !mine(pk) {
		found, decided = true, true
	}
	skipBase := false
	if !decided && ts.hasID && ts.maxLoaded && slices.Contains(cols, "id") {
		if id, ok := ts.rowID(row); ok && id > ts.baseMax {
			// no row of PostgreSQL has an id past the highest of the block's start
			skipBase = true
		}
	}
	pj := j
	if isPK {
		pj = -1
	}
	v.readPoint(ts, seq, eqCond(m, cols, row), pj, key)
	if decided || skipBase {
		return found, nil
	}
	bj := j
	if isPK {
		bj = -1
	}
	r, ok := ts.base.get(bj, key)
	if !ok {
		params := make([]any, len(cols))
		for i, col := range cols {
			params[i] = row[m.colIndex(col)]
		}
		base, err := fetchRows(ctx, c, selectAll(m, cols), params)
		if err != nil {
			return false, err
		}
		if len(base) > 0 {
			r = base[0]
		}
		ts.base.put(bj, key, r)
	}
	if r == nil {
		return false, nil
	}
	pk := m.pkOf(r)
	if pk == self || mine(pk) {
		return false, nil
	}
	_, changed := ts.rows.load(pk)
	return !changed, nil
}

// checkUnique tells the row violates no unique index the change from prev touches
func (v *TxView) checkUnique(ctx context.Context, c *stateConn, ts *tableState, prev, row stateRow, self string) error {
	m := ts.meta
	for ix := range m.uniques {
		if prev != nil {
			k1, ok1 := m.ukeyOf(ix, prev)
			k2, ok2 := m.ukeyOf(ix, row)
			if ok1 == ok2 && k1 == k2 {
				continue
			}
		}
		taken, err := v.taken(ctx, c, ts, ix, row, self)
		if err != nil {
			return err
		}
		if taken {
			// PostgreSQL gives the error of the violation
			return errUnsupported
		}
	}
	return nil
}

func (v *TxView) insert(ctx context.Context, c *stateConn, ts *tableState, p *stmt, b *binder) (*memRows, int64, error) {
	m := ts.meta
	row := make(stateRow, len(m.cols))
	set := make([]bool, len(m.cols))
	for k, name := range p.cols {
		i := m.colIndex(name)
		if i < 0 || set[i] {
			return nil, 0, errUnsupported
		}
		set[i] = true
		col := m.cols[i]
		e, ok := b.bindExpr(p.vals[k], col.typ)
		if !ok {
			return nil, 0, errUnsupported
		}
		val, ok := e.eval(nil)
		if !ok {
			return nil, 0, errUnsupported
		}
		if val, ok = convert(val, col.typ); !ok {
			return nil, 0, errUnsupported
		}
		row[i] = val.s
	}
	for i, col := range m.cols {
		if set[i] || !col.hasDef {
			continue
		}
		if !col.defOK {
			return nil, 0, errUnsupported
		}
		row[i] = col.def
	}
	if !m.checkRow(row) {
		return nil, 0, errUnsupported
	}
	pk := m.pkOf(row)
	at := len(v.undo)
	if err := v.checkUnique(ctx, c, ts, nil, row, ""); err != nil {
		return nil, 0, err
	}
	vt := v.viewTable(ts)
	if len(v.undo) != at {
		v.rollbackUndo(at)
	}
	v.setRow(vt, pk, nil, row)
	return nil, 1, nil
}

func (v *TxView) update(ctx context.Context, c *stateConn, ts *tableState, p *stmt, b *binder, args []driver.NamedValue, cond *bcond) (*memRows, int64, error) {
	m := ts.meta
	type assign struct {
		col  int
		expr *bexpr
	}
	var sets []assign
	for _, s := range p.sets {
		i := m.colIndex(s.col)
		if i < 0 || slices.Contains(m.pk, s.col) || slices.ContainsFunc(sets, func(a assign) bool { return a.col == i }) {
			return nil, 0, errUnsupported
		}
		e, ok := b.bindExpr(s.expr, m.cols[i].typ)
		if !ok {
			return nil, 0, errUnsupported
		}
		sets = append(sets, assign{col: i, expr: e})
	}
	targets, err := v.selectRows(ctx, c, ts, p, args, cond, nil, -1, 0)
	if err != nil {
		return nil, 0, err
	}
	news := make([]stateRow, len(targets))
	for k, r := range targets {
		nr := r.clone()
		for _, a := range sets {
			val, ok := a.expr.eval(r)
			if !ok {
				return nil, 0, errUnsupported
			}
			if val, ok = convert(val, m.cols[a.col].typ); !ok {
				return nil, 0, errUnsupported
			}
			nr[a.col] = val.s
		}
		if !m.checkRow(nr) {
			return nil, 0, errUnsupported
		}
		news[k] = nr
	}
	vt := v.viewTable(ts)
	at := len(v.undo)
	for k, r := range targets {
		pk := m.pkOf(r)
		if err := v.checkUnique(ctx, c, ts, r, news[k], pk); err != nil {
			v.rollbackUndo(at)
			return nil, 0, err
		}
		v.setRow(vt, pk, r, news[k])
	}
	return nil, int64(len(targets)), nil
}

func (v *TxView) delete(ctx context.Context, c *stateConn, ts *tableState, p *stmt, args []driver.NamedValue, cond *bcond) (*memRows, int64, error) {
	m := ts.meta
	targets, err := v.selectRows(ctx, c, ts, p, args, cond, nil, -1, 0)
	if err != nil {
		return nil, 0, err
	}
	vt := v.viewTable(ts)
	for _, r := range targets {
		v.setRow(vt, m.pkOf(r), r, nil)
	}
	return nil, int64(len(targets)), nil
}

// maxID answers the select of the highest id of the table: in parallel, with the ids the
// transactions before the view that the state has not committed take
func (v *TxView) maxID(ts *tableState, p *stmt) (*memRows, int64, error) {
	if !ts.hasID {
		return nil, 0, errUnsupported
	}
	st := v.st
	st.idMu.Lock()
	known, committed := ts.maxKnown && ts.maxLoaded, ts.max
	var pred int64
	if !v.ordered {
		pred = st.predicted(v.index, ts.meta.name)
	}
	st.idMu.Unlock()
	if !known {
		return nil, 0, errUnsupported
	}
	own := int64(math.MinInt64)
	if vt := v.own[ts.meta.name]; vt != nil {
		for _, vc := range vt.rows {
			if vc.row != nil && vc.row[ts.idCol] == nil {
				// a NULL id comes first in the order of the select
				return nil, 0, errUnsupported
			}
			if id, ok := ts.rowID(vc.row); ok && id > own {
				own = id
			}
			if id, ok := ts.rowID(vc.prev); ok && id >= committed {
				if nid, ok := ts.rowID(vc.row); !ok || nid < id {
					// the view took out the row of the highest id
					return nil, 0, errUnsupported
				}
			}
		}
	}
	value := max(committed+pred, own)
	if !v.ordered {
		v.ids = append(v.ids, idRead{ts: ts, own: own, value: value})
	}
	if _, ok := v.idBase[ts.meta.name]; !ok {
		v.idBase[ts.meta.name] = value
	}
	out := &memRows{names: []string{p.proj[0].as}, types: []colType{ts.meta.cols[ts.idCol].typ},
		mods: []int32{ts.meta.cols[ts.idCol].typmod}}
	if value > 0 {
		s := strconv.FormatInt(value, 10)
		out.rows = [][]*string{{&s}}
	}
	return out, 0, nil
}

// BaseMaxID is the highest id of the table when the block began; false when the state does not
// know it
func (v *TxView) BaseMaxID(ctx context.Context, c *stateConn, table string) (int64, bool) {
	ts, err := v.tableOf(ctx, c, table)
	if err != nil || ts == nil || !ts.maxLoaded {
		return 0, false
	}
	return ts.baseMax, true
}
