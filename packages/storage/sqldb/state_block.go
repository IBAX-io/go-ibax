/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql/driver"
	"errors"
	"slices"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/metrics"
)

// The state of a block keeps in memory the rows the transactions of the block write. A row it
// has not written stays in PostgreSQL as it was when the block began: the base of the row. The
// transactions run in views of the state (TxView), in parallel: a view sees the rows the state
// has committed and its own writes, and records what it reads. The block commits the views in the
// order of its transactions; a view whose reads a later commit changed runs again, in order. The
// state writes its rows to PostgreSQL in the transaction of the block when the block ends.
//
// A table the state cannot keep goes to PostgreSQL: the state writes there what it has of the
// table, and the statements on the table then run in PostgreSQL, in the order of the block.

// ErrNeedsOrder is the error of a statement a transaction run in parallel cannot run: the
// transaction runs again in the order of the block
var ErrNeedsOrder error = &rerunError{"the transaction runs in the order of the block"}

// errRestart is the error of a statement after which a transaction run in order runs again: the
// state has to write tables to PostgreSQL first
var errRestart error = &rerunError{"the transaction runs again after the state writes tables to PostgreSQL"}

// rerunError is the error of a statement after which the transaction runs again. It is the flow
// of the block, not a fault: the packages that log errors tell it by its method Rerun, and those
// that wrap it keep it
type rerunError struct{ text string }

func (e *rerunError) Error() string { return e.text }

// Rerun tells the transaction runs again
func (e *rerunError) Rerun() bool { return true }

// IsRerun tells err is, or wraps, the error of a statement after which the transaction runs again
func IsRerun(err error) bool {
	var r interface{ Rerun() bool }
	return errors.As(err, &r) && r.Rerun()
}

// errUnsupported tells the state does not run a statement in memory
var errUnsupported = errors.New("not supported by the state of the block")

// stateSchema is what the state knows of the schema: the catalog, the tables and the plans of the
// statements. A change of the schema clears it.
type stateSchema struct {
	mu    sync.RWMutex
	gen   uint64
	cat   *stateCatalog
	metas map[string]*tableMeta
	plans map[string]*stmt
}

var schema stateSchema

// maxPlans bounds the plans the state keeps
const maxPlans = 4096

// bumpSchema clears what the state knows of the schema
func bumpSchema() {
	schema.mu.Lock()
	defer schema.mu.Unlock()
	schema.gen++
	schema.cat, schema.metas, schema.plans = nil, nil, nil
}

func (s *stateSchema) catalogLocked(ctx context.Context, q driver.QueryerContext) (*stateCatalog, error) {
	if s.cat == nil {
		cat, err := loadCatalog(ctx, q)
		if err != nil {
			return nil, err
		}
		s.cat = cat
	}
	return s.cat, nil
}

// plan is the plan of a statement
func (s *stateSchema) plan(ctx context.Context, q driver.QueryerContext, query string) (*stmt, error) {
	s.mu.RLock()
	p, ok := s.plans[query]
	s.mu.RUnlock()
	if ok {
		return p, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.plans[query]; ok {
		return p, nil
	}
	cat, err := s.catalogLocked(ctx, q)
	if err != nil {
		return nil, err
	}
	p = parseStmt(query, cat)
	if s.plans == nil || len(s.plans) >= maxPlans {
		s.plans = make(map[string]*stmt)
	}
	s.plans[query] = p
	return p, nil
}

// meta is the table of the name; nil for a relation that is no table of the schema
func (s *stateSchema) meta(ctx context.Context, q driver.QueryerContext, table string) (*tableMeta, error) {
	s.mu.RLock()
	m, ok := s.metas[table]
	s.mu.RUnlock()
	if ok {
		return m, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.metas[table]; ok {
		return m, nil
	}
	m, err := loadMeta(ctx, q, table)
	if err != nil {
		return nil, err
	}
	if s.metas == nil {
		s.metas = make(map[string]*tableMeta)
	}
	s.metas[table] = m
	return m, nil
}

// BlockState is the state of a block
type BlockState struct {
	// mu orders the changes of the state and what reads it whole: the views read the rows of
	// the tables, the seq they saw at pub, and whether a table is in PostgreSQL without it
	mu     sync.RWMutex
	tables map[string]*tableState
	// known has the tables of tables, for the views to find them without the lock
	known sync.Map
	// seq counts the commits of views that changed the state; pub is seq once the changes of
	// the commit are in the tables
	seq uint64
	pub atomic.Uint64
	// lastChange is the seq of the last change of any table, a table that went to PostgreSQL
	// included
	lastChange uint64
	// sysGen counts the changes of what the node keeps in memory out of the database: the
	// system parameters, the contracts of the virtual machine
	sysGen atomic.Uint64
	// allPG tells every table is in PostgreSQL
	allPG atomic.Bool
	// schemaChanged tells a statement may have changed the schema
	schemaChanged bool
	// outputs are the outputs of the accounts of the block, under utxoMu; utxoSeq the commit
	// that last changed those of an account
	utxoMu  sync.RWMutex
	outputs map[KeyUTXO][]SpentInfo
	utxoSeq map[KeyUTXO]uint64
	// dirty are the tables the transaction of the block wrote before the state began
	dirty    map[string]bool
	dirtyAll bool
	// block is the transaction of the block
	block *DbTransaction
	// the transactions of the block, for the ids the earlier ones take; idMu guards next,
	// contracts and the highest ids of the tables, which the views read for the next ids
	idMu      sync.Mutex
	txs       []txSlot
	next      int
	contracts map[string]map[string]int
	// fallbacks count the statements the state ran in PostgreSQL, by reason
	fallbacks sync.Map
	// cleanReads are the answers of PostgreSQL to selects of tables the block has not changed,
	// by readKey
	cleanReads sync.Map
	cleanCount atomic.Int64
	// shared are the values the transactions of the block decoded of the rows of tables the
	// block has not changed, by table and key of the cache of a transaction
	shared      sync.Map
	sharedCount atomic.Int64
}

// txSlot is a transaction of the block: the contract it runs, and the ids it took by table in its
// last run
type txSlot struct {
	contract string
	ids      atomic.Pointer[map[string]int]
}

// tableState is what the state has of a table
type tableState struct {
	meta *tableMeta
	// rows are the rows the state changed, by primary key
	rows smap[*rowChange]
	// uniq maps the keys of each unique index but the primary key of the rows the state changed
	// to their primary key
	uniq   []*smap[string]
	uniqIx []int
	events []stateEvent
	// byKey are the indexes in events of the changes of the rows of each key: of the primary key
	// at 0, of the unique index j at j+1, by the keys of the row before and after
	byKey []map[string][]int32
	// pg tells the table is in PostgreSQL since the commit pgSeq
	pg    atomic.Bool
	pgSeq uint64
	// the highest id: of the table when the block began, and with the changes of the state;
	// maxKnown is false when a change took out the row of the highest id
	hasID, maxLoaded, maxKnown bool
	baseMax, max               int64
	idCol                      int
	// base are the rows of PostgreSQL the block read by a key
	base baseRows
}

// rowChange is a row the state changed: the row when the block began, nil for none, and the
// row now, nil for a deleted one. A commit replaces it whole.
type rowChange struct {
	base, row stateRow
}

// stateEvent is a change of a row in a commit
type stateEvent struct {
	seq      uint64
	old, new stateRow
}

// NewBlockState is the state of a block that runs in the transaction of the block. The tables
// the transaction wrote before go to PostgreSQL.
func NewBlockState(block *DbTransaction, contracts []string) (*BlockState, error) {
	st := &BlockState{tables: make(map[string]*tableState), block: block, contracts: make(map[string]map[string]int)}
	st.Extend(contracts)
	err := st.onBlockConn(func(ctx context.Context, c *stateConn) error {
		st.dirty = make(map[string]bool, len(c.dirty))
		for t := range c.dirty {
			st.dirty[t] = true
		}
		st.dirtyAll = c.dirtyAll
		return nil
	})
	if err != nil {
		return nil, err
	}
	st.allPG.Store(st.dirtyAll)
	return st, nil
}

// blockQueryer runs a function on the connection of the transaction of the block
type blockQueryer struct {
	f func(ctx context.Context, c *stateConn) error
}

func (q blockQueryer) query(ctx context.Context, c *stateConn, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	return emptyRows(), q.f(ctx, c)
}

func (q blockQueryer) exec(ctx context.Context, c *stateConn, _ string, _ []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), q.f(ctx, c)
}

// onBlockConn runs f on the connection of the transaction of the block
func (st *BlockState) onBlockConn(f func(ctx context.Context, c *stateConn) error) error {
	ctx := st.block.conn.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return st.block.conn.WithContext(withState(ctx, blockQueryer{f})).Exec("SELECT 'state of the block'").Error
}

// fallback counts a statement the state ran in PostgreSQL
func (st *BlockState) fallback(reason string) {
	v, _ := st.fallbacks.LoadOrStore(reason, new(atomic.Int64))
	v.(*atomic.Int64).Add(1)
	metrics.StateFallbacks.WithLabelValues(reason).Inc()
}

// Fallbacks are the counts of the statements the state ran in PostgreSQL, by reason
func (st *BlockState) Fallbacks() map[string]int64 {
	out := make(map[string]int64)
	st.fallbacks.Range(func(k, v any) bool {
		out[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	return out
}

// SysGen is the count of the changes of what the node keeps in memory out of the database
func (st *BlockState) SysGen() uint64 { return st.sysGen.Load() }

// SysUpdate tells the state the node changed what it keeps in memory out of the database: the
// views that began before do not commit
func (st *BlockState) SysUpdate() { st.sysGen.Add(1) }

// Extend appends the transactions of the contracts to those of the block
func (st *BlockState) Extend(contracts []string) {
	st.idMu.Lock()
	defer st.idMu.Unlock()
	for _, c := range contracts {
		st.txs = append(st.txs, txSlot{contract: c})
	}
}

// SetRunIDs keeps the ids by table the last run of the transaction i took
func (st *BlockState) SetRunIDs(i int, ids map[string]int) {
	st.idMu.Lock()
	st.txs[i].ids.Store(&ids)
	st.idMu.Unlock()
}

// predicted is the count of the ids of the table the transactions before i the state has not
// committed take: as in their last run, or as the last transaction of their contract did
func (st *BlockState) predicted(i int, table string) int64 {
	var n int64
	for j := st.next; j < i && j < len(st.txs); j++ {
		if ids := st.txs[j].ids.Load(); ids != nil {
			n += int64((*ids)[table])
		} else {
			n += int64(st.contracts[st.txs[j].contract][table])
		}
	}
	return n
}

// table is the state of a table, which it creates for the meta with what it reads of the table
// on the connection
func (st *BlockState) table(ctx context.Context, c *stateConn, m *tableMeta) (*tableState, error) {
	if ts, ok := st.known.Load(m.name); ok {
		return ts.(*tableState), nil
	}
	st.mu.RLock()
	pg := st.allPG.Load() || st.dirty[m.name] || !m.writable
	st.mu.RUnlock()
	ts := &tableState{meta: m, idCol: m.colIndex("id")}
	ts.pg.Store(pg)
	ts.hasID = ts.idCol >= 0 && m.cols[ts.idCol].typ.kind == kInt
	for i, u := range m.uniques {
		if slices.Equal(u, m.pk) {
			continue
		}
		ts.uniq = append(ts.uniq, new(smap[string]))
		ts.uniqIx = append(ts.uniqIx, i)
	}
	ts.resetKeys()
	if ts.hasID && !pg {
		if err := ts.loadMax(ctx, c); err != nil {
			return nil, err
		}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if prev := st.tables[m.name]; prev != nil {
		return prev, nil
	}
	if st.allPG.Load() {
		ts.pg.Store(true)
	}
	st.tables[m.name] = ts
	st.known.Store(m.name, ts)
	return ts, nil
}

// resetKeys empties the index of the events by key
func (ts *tableState) resetKeys() {
	ts.byKey = make([]map[string][]int32, len(ts.uniqIx)+1)
	for i := range ts.byKey {
		ts.byKey[i] = make(map[string][]int32)
	}
}

// addEvent appends a change of the row of the primary key pk to the events
func (ts *tableState) addEvent(e stateEvent, pk string) {
	n := int32(len(ts.events))
	ts.events = append(ts.events, e)
	ts.byKey[0][pk] = append(ts.byKey[0][pk], n)
	for j, ix := range ts.uniqIx {
		ko, okOld := "", false
		if e.old != nil {
			ko, okOld = ts.meta.ukeyOf(ix, e.old)
			if okOld {
				ts.byKey[j+1][ko] = append(ts.byKey[j+1][ko], n)
			}
		}
		if e.new != nil {
			if kn, ok := ts.meta.ukeyOf(ix, e.new); ok && (!okOld || kn != ko) {
				ts.byKey[j+1][kn] = append(ts.byKey[j+1][kn], n)
			}
		}
	}
}

// changedSince tells a commit after seq changed a row the read r of the table reads
func (ts *tableState) changedSince(r *stateRead) bool {
	ev := ts.events
	if r.cond == nil {
		return len(ev) > 0 && ev[len(ev)-1].seq > r.seq
	}
	if r.point {
		idx := ts.byKey[r.ix+1][r.key]
		from := sort.Search(len(idx), func(i int) bool { return ev[idx[i]].seq > r.seq })
		for _, i := range idx[from:] {
			if ev[i].changes(r.cond) {
				return true
			}
		}
		return false
	}
	from := sort.Search(len(ev), func(i int) bool { return ev[i].seq > r.seq })
	for _, e := range ev[from:] {
		if e.changes(r.cond) {
			return true
		}
	}
	return false
}

// changes tells the row before or after the event holds the condition
func (e *stateEvent) changes(cond *bcond) bool {
	return e.old != nil && cond.matches(e.old) || e.new != nil && cond.matches(e.new)
}

// changed tells the state changed rows of the table
func (ts *tableState) changed() bool { return ts.rows.len() > 0 }

// visible is the row of the primary key the state has, and whether it has it
func (ts *tableState) visible(pk string) (stateRow, bool) {
	rc, ok := ts.rows.load(pk)
	if !ok {
		return nil, false
	}
	return rc.row, true
}

// pkOf is the primary key of a row
func (m *tableMeta) pkOf(r stateRow) string {
	k, _ := m.keyOf(m.pk, func(c string) *string { return r[m.colIndex(c)] })
	return k
}

// ukeyOf is the key of a row in a unique index; false when a column is NULL
func (m *tableMeta) ukeyOf(ix int, r stateRow) (string, bool) {
	return m.keyOf(m.uniques[ix], func(c string) *string { return r[m.colIndex(c)] })
}

// rowID is the id of a row; false for a row without
func (ts *tableState) rowID(r stateRow) (int64, bool) {
	if r == nil || !ts.hasID || r[ts.idCol] == nil {
		return 0, false
	}
	id, err := strconv.ParseInt(*r[ts.idCol], 10, 64)
	return id, err == nil
}

// loadMax reads the highest id of the table when the block began
func (ts *tableState) loadMax(ctx context.Context, c *stateConn) error {
	rows, err := fetchRows(ctx, c, `SELECT "id" FROM `+quoteIdent(ts.meta.name)+` ORDER BY "id" DESC LIMIT 1`, nil)
	if err != nil {
		return err
	}
	var id int64
	if len(rows) == 1 && rows[0][0] == nil {
		// a NULL id: the state leaves the ids of the table to PostgreSQL
		ts.hasID = false
		return nil
	}
	if len(rows) == 1 {
		if id, err = strconv.ParseInt(*rows[0][0], 10, 64); err != nil {
			return err
		}
	}
	return ts.setBaseMax(id)
}

// setBaseMax keeps the highest id of the table when the block began
func (ts *tableState) setBaseMax(id int64) error {
	ts.maxLoaded, ts.maxKnown, ts.baseMax, ts.max = true, true, id, id
	return nil
}

// validate tells the reads of a view hold in the state as it is now: no commit since a read
// changed what it read. The caller holds the lock of the state.
func (st *BlockState) validate(v *TxView) bool {
	if v.sysGen != st.sysGen.Load() || st.allPG.Load() {
		return false
	}
	for _, r := range v.reads {
		if r.ts == nil {
			if st.lastChange > r.seq {
				return false
			}
			continue
		}
		if r.ts.pg.Load() {
			return false
		}
		if r.ts.changedSince(&r) {
			return false
		}
	}
	for _, r := range v.ids {
		if r.ts.pg.Load() || !r.ts.maxKnown || max(r.ts.max, r.own) != r.value {
			return false
		}
	}
	return st.utxoHolds(v)
}

// Holds tells the reads of a view of a transaction hold in the state as it is now; a view run
// in order holds. A view run in parallel that does not hold has to run again in order. Only
// the thread that commits changes the state, so what Holds tells holds until it commits.
func (st *BlockState) Holds(v *TxView) bool {
	if v.ordered {
		return true
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.validate(v)
}

// Commit commits a view of the transaction i, which Holds, in the order of the block
func (st *BlockState) Commit(i int, v *TxView) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.apply(v)
	st.idMu.Lock()
	if i >= st.next {
		st.next = i + 1
	}
	if v.contract != "" {
		st.contracts[v.contract] = v.IDCounts()
	}
	st.idMu.Unlock()
}

// Skip passes the transaction i, which commits nothing
func (st *BlockState) Skip(i int) {
	st.idMu.Lock()
	if i >= st.next {
		st.next = i + 1
	}
	st.idMu.Unlock()
}

// apply writes the changes of a view into the state. The caller holds the lock of the state.
func (st *BlockState) apply(v *TxView) {
	changed := false
	for _, name := range v.order {
		vt := v.own[name]
		if len(vt.order) == 0 {
			continue
		}
		ts := vt.ts
		m := ts.meta
		if !changed {
			st.seq++
			changed = true
		}
		type change struct {
			pk       string
			old, new stateRow
		}
		var changes []change
		for _, pk := range vt.order {
			vc, ok := vt.rows[pk]
			if !ok {
				continue
			}
			rc, had := ts.rows.load(pk)
			var old stateRow
			if had {
				old = rc.row
			} else {
				old = vc.prev
			}
			if old == nil && vc.row == nil {
				continue
			}
			changes = append(changes, change{pk: pk, old: old, new: vc.row})
		}
		// out the old keys of the unique indexes first: two rows may swap them
		for _, c := range changes {
			if c.old == nil {
				continue
			}
			for j, ix := range ts.uniqIx {
				if k, ok := m.ukeyOf(ix, c.old); ok {
					if pk, _ := ts.uniq[j].load(k); pk == c.pk {
						ts.uniq[j].delete(k)
					}
				}
			}
		}
		for _, c := range changes {
			base := c.old
			if rc, had := ts.rows.load(c.pk); had {
				base = rc.base
			}
			ts.rows.store(c.pk, &rowChange{base: base, row: c.new})
			if c.new != nil {
				for j, ix := range ts.uniqIx {
					if k, ok := m.ukeyOf(ix, c.new); ok {
						ts.uniq[j].store(k, c.pk)
					}
				}
			}
			ts.addEvent(stateEvent{seq: st.seq, old: c.old, new: c.new}, c.pk)
			if ts.maxLoaded {
				st.idMu.Lock()
				oldID, hadOld := ts.rowID(c.old)
				newID, hasNew := ts.rowID(c.new)
				if hadOld && oldID >= ts.max && (!hasNew || newID < oldID) {
					ts.maxKnown = false
				}
				if hasNew && newID > ts.max {
					ts.max = newID
				}
				if c.new != nil && ts.hasID && c.new[ts.idCol] == nil {
					// a NULL id comes first in the order of the select of the highest id
					ts.maxKnown = false
				}
				st.idMu.Unlock()
			}
		}
	}
	if changed {
		st.lastChange = st.seq
		st.pub.Store(st.seq)
	}
}

// toPG writes what the state has of the tables to PostgreSQL, where the tables then are. The
// caller holds no lock; it runs in the order of the block.
func (st *BlockState) toPG(names []string, all bool) error {
	return st.onBlockConn(func(ctx context.Context, c *stateConn) error {
		st.mu.Lock()
		defer st.mu.Unlock()
		if all {
			st.allPG.Store(true)
			names = names[:0]
			for name := range st.tables {
				names = append(names, name)
			}
		}
		slices.Sort(names)
		for _, name := range names {
			ts := st.tables[name]
			if ts == nil || ts.pg.Load() {
				continue
			}
			if err := flushTable(ctx, c, ts); err != nil {
				return err
			}
			st.seq++
			ts.pg.Store(true)
			ts.pgSeq = st.seq
			ts.rows.clear()
			ts.events = nil
			ts.resetKeys()
			for _, u := range ts.uniq {
				u.clear()
			}
			st.lastChange = st.seq
			st.pub.Store(st.seq)
		}
		return nil
	})
}

// markPG puts in PostgreSQL a table the state has not changed: statements in the order of the
// block write it there
func (st *BlockState) markPG(ts *tableState) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if ts.pg.Load() {
		return
	}
	st.seq++
	ts.pg.Store(true)
	ts.pgSeq = st.seq
	st.lastChange = st.seq
	st.pub.Store(st.seq)
}

// Flush writes the rows the state changed to PostgreSQL in the transaction of the block
func (st *BlockState) Flush() error {
	start := time.Now()
	defer func() { metrics.StateFlush.Observe(metrics.Since(start)) }()
	return st.onBlockConn(func(ctx context.Context, c *stateConn) error {
		st.mu.Lock()
		defer st.mu.Unlock()
		names := make([]string, 0, len(st.tables))
		for name := range st.tables {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			ts := st.tables[name]
			if ts.pg.Load() {
				continue
			}
			if err := flushTable(ctx, c, ts); err != nil {
				return err
			}
		}
		return nil
	})
}

// SchemaChanged tells a statement of the block may have changed the schema
func (st *BlockState) SchemaChanged() bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.schemaChanged
}

// End ends the state after the block: a change of the schema clears what the state knows of it
func (st *BlockState) End() {
	if st.SchemaChanged() {
		bumpSchema()
	}
}
