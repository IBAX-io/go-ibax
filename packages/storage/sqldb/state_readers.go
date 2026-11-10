/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"slices"
	"sync"
)

// The readers of a block are the runs of its transactions not committed yet, by what they read.
// A commit wakes the readers of the rows it changed, which the block runs again ahead of the
// commit, on its workers, while the commit goes on with the transactions before them. The
// readers only tell when to run again: a run commits once Holds tells it holds, so a reader the
// index misses runs again at its commit, as without the index.

// Reader is a run of the transaction I of the block: Gen counts the runs of the transaction
type Reader struct {
	I   int
	Gen uint32
}

// readerKey is a point read of a table: of the key of the primary key (ix -1) or of the unique
// index ix
type readerKey struct {
	ts  *tableState
	ix  int
	key string
}

// pointReader is a reader of a point; writes tells the run also wrote a row of the point, whose
// commit changes the point again
type pointReader struct {
	Reader
	writes bool
}

// idReader is a reader of the highest id of a table, which it read as value, own the highest id
// of its own rows
type idReader struct {
	Reader
	own, value int64
}

// scanReader is a reader of the rows of a table that hold a condition, of every row for nil;
// wrote are the rows of the table the run wrote, before and after it wrote them
type scanReader struct {
	Reader
	cond  *bcond
	wrote []stateRow
}

// touch is a row a commit changed: its primary key, the row before and after
type touch struct {
	ts       *tableState
	pk       string
	old, new stateRow
}

// readerIndex are the readers of the block by what they read. The workers register their runs,
// the commit looks up the rows it changed; mu orders them.
type readerIndex struct {
	mu     sync.Mutex
	points map[readerKey][]pointReader
	scans  map[*tableState][]scanReader
	ids    map[*tableState][]idReader
	// st predicts the ids the transactions not committed take; nil predicts none
	st *BlockState
	// gens are the last runs registered, by transaction: a reader of an earlier run is stale
	gens map[int]uint32
	// committed is the number of the transactions the block committed or passed
	committed int
	woken     []Reader
}

func newReaderIndex() *readerIndex {
	return &readerIndex{points: make(map[readerKey][]pointReader), scans: make(map[*tableState][]scanReader),
		ids: make(map[*tableState][]idReader), gens: make(map[int]uint32)}
}

// WatchReaders makes the state keep the readers of the block, which the commits wake
func (st *BlockState) WatchReaders() {
	ri := newReaderIndex()
	ri.st = st
	st.readers.Store(ri)
}

// Register keeps the reads of the run gen of the transaction i in the view as readers, unless a
// later run of the transaction is. It tells whether it kept them, a commit since the view began
// may have changed what the view read before the run was a reader, and no reader before the
// run wrote what it read: the run has to check it holds, as no commit woke it, nor will one
// before the commit of the reader that wrote it.
func (st *BlockState) Register(i int, gen uint32, v *TxView) bool {
	ri := st.readers.Load()
	if ri == nil {
		return false
	}
	ri.mu.Lock()
	kept := ri.register(i, gen, v) && !ri.awaits(i, v)
	ri.mu.Unlock()
	// the commit publishes its seq before it looks up the readers, under the lock of the index:
	// a commit the run did not see either found the run, or published before the lock was
	// taken here
	return kept && st.pub.Load() > v.start
}

func (ri *readerIndex) register(i int, gen uint32, v *TxView) bool {
	if i < ri.committed {
		return false
	}
	if g, ok := ri.gens[i]; ok && gen < g {
		return false
	}
	ri.gens[i] = gen
	r := Reader{I: i, Gen: gen}
	for _, rd := range v.reads {
		switch {
		case rd.ts == nil:
			// a read of every table: the index wakes it on no commit
		case rd.point:
			k := readerKey{ts: rd.ts, ix: rd.ix, key: rd.key}
			ri.points[k] = append(ri.points[k], pointReader{Reader: r, writes: v.writes(rd.ts, rd.ix, rd.key)})
		default:
			ri.scans[rd.ts] = append(ri.scans[rd.ts], scanReader{Reader: r, cond: rd.cond, wrote: v.wrote(rd.ts)})
		}
	}
	for _, rd := range v.ids {
		ri.ids[rd.ts] = append(ri.ids[rd.ts], idReader{Reader: r, own: rd.own, value: rd.value})
	}
	return true
}

// awaits tells a live reader before the transaction i wrote what the view read: the commit of
// that one wakes the readers after it of what it wrote
func (ri *readerIndex) awaits(i int, v *TxView) bool {
	for _, rd := range v.reads {
		switch {
		case rd.ts == nil:
		case rd.point:
			for _, r := range ri.points[readerKey{ts: rd.ts, ix: rd.ix, key: rd.key}] {
				if r.I < i && r.writes && ri.live(r.Reader) {
					return true
				}
			}
		default:
			for _, r := range ri.scans[rd.ts] {
				if r.I < i && ri.live(r.Reader) && slices.ContainsFunc(r.wrote, rd.cond.matches) {
					return true
				}
			}
		}
	}
	return false
}

// Woken are the readers the commits woke since the last call
func (st *BlockState) Woken() []Reader {
	ri := st.readers.Load()
	if ri == nil {
		return nil
	}
	ri.mu.Lock()
	defer ri.mu.Unlock()
	w := ri.woken
	ri.woken = nil
	return w
}

// live tells the reader is of the last run of a transaction not committed
func (ri *readerIndex) live(r Reader) bool {
	return r.I >= ri.committed && ri.gens[r.I] == r.Gen
}

// wake wakes the readers of the rows of the commit of the transaction i, and drops the readers
// of the transactions committed and of the runs after which another ran. Of the readers of a row
// it wakes, in the order of the block, those up to the first that wrote what they read: the
// commit of that one changes it again, and wakes those after it. A run that wrote a row of a
// point wrote what the readers of the point after it read; one that wrote a row of a scan, before
// or after it wrote it, what the readers after it of a condition the row holds read. A reader is
// live as it was before the commit, so the readers it wakes do not depend on the order of the
// rows.
func (ri *readerIndex) wake(i int, touches []touch) {
	ri.committed = max(ri.committed, i+1)
	woken := map[int]uint32{}
	woke := func(r Reader) {
		if _, ok := woken[r.I]; !ok {
			woken[r.I] = r.Gen
			ri.woken = append(ri.woken, r)
		}
	}
	// pick wakes the readers of the key in the order of the block up to the first that wrote it
	pick := func(k readerKey) {
		list, ok := ri.points[k]
		if !ok {
			return
		}
		list = slices.DeleteFunc(list, func(r pointReader) bool { return !ri.live(r.Reader) })
		slices.SortStableFunc(list, func(a, b pointReader) int { return a.I - b.I })
		n := 0
		for n < len(list) {
			r := list[n]
			n++
			woke(r.Reader)
			if r.writes {
				break
			}
		}
		if list = list[n:]; len(list) == 0 {
			delete(ri.points, k)
		} else {
			ri.points[k] = list
		}
	}
	// the points and tables of the commit, each once: a second pick of a point would wake those
	// after the first reader that wrote it
	var points []readerKey
	var tables, idTables []*tableState
	point := func(k readerKey) {
		if !slices.Contains(points, k) {
			points = append(points, k)
		}
	}
	for _, t := range touches {
		ts := t.ts
		if !slices.Contains(tables, ts) {
			tables = append(tables, ts)
		}
		if _, ok := ri.ids[ts]; ok && idChange(ts, t) && !slices.Contains(idTables, ts) {
			idTables = append(idTables, ts)
		}
		point(readerKey{ts: ts, ix: -1, key: t.pk})
		for j, ix := range ts.uniqIx {
			for _, row := range []stateRow{t.old, t.new} {
				if row == nil {
					continue
				}
				if k, ok := ts.meta.ukeyOf(ix, row); ok {
					point(readerKey{ts: ts, ix: j, key: k})
				}
			}
		}
	}
	for _, k := range points {
		pick(k)
	}
	for _, ts := range tables {
		list := ri.scans[ts]
		if len(list) == 0 {
			continue
		}
		list = slices.DeleteFunc(list, func(r scanReader) bool { return !ri.live(r.Reader) })
		slices.SortStableFunc(list, func(a, b scanReader) int { return a.I - b.I })
		hit := func(r scanReader) bool {
			return slices.ContainsFunc(touches, func(t touch) bool {
				return t.ts == ts && (t.old != nil && r.cond.matches(t.old) || t.new != nil && r.cond.matches(t.new))
			})
		}
		// wrote are the rows the readers woken wrote: a reader after them of a condition one of
		// the rows holds waits for their commits
		var wrote []stateRow
		kept := list[:0]
		for _, r := range list {
			if !hit(r) || slices.ContainsFunc(wrote, r.cond.matches) {
				kept = append(kept, r)
				continue
			}
			woke(r.Reader)
			wrote = append(wrote, r.wrote...)
		}
		ri.scans[ts] = kept
	}
	// a reader of the highest id holds while it is what the state now and the ids the
	// transactions before the reader take tell
	for _, ts := range idTables {
		list := slices.DeleteFunc(ri.ids[ts], func(r idReader) bool { return !ri.live(r.Reader) })
		slices.SortStableFunc(list, func(a, b idReader) int { return a.I - b.I })
		now, known := ri.idsNow(ts, list)
		kept := list[:0]
		for k, r := range list {
			if !known || max(now[k], r.own) != r.value {
				woke(r.Reader)
				continue
			}
			kept = append(kept, r)
		}
		ri.ids[ts] = kept
	}
	// the run of a reader woken runs again and registers its reads anew
	for i, gen := range woken {
		ri.gens[i] = gen + 1
	}
}

// idsNow are the highest ids of the table the readers, in the order of the block, read now, and
// whether the state knows them: the highest committed, and the ids the transactions before each
// reader the state has not committed take. It sums the ids of each transaction once: a sum for
// each reader would make the commits of a block of readers cubic in its size.
func (ri *readerIndex) idsNow(ts *tableState, list []idReader) ([]int64, bool) {
	now := make([]int64, len(list))
	if ri.st == nil {
		for k := range now {
			now[k] = ts.max
		}
		return now, ts.maxKnown && !ts.pg.Load()
	}
	st := ri.st
	st.idMu.Lock()
	defer st.idMu.Unlock()
	j, n := st.next, int64(0)
	for k, r := range list {
		for ; j < r.I && j < len(st.txs); j++ {
			n += st.takes(j, ts.meta.name)
		}
		now[k] = ts.max + n
	}
	return now, ts.maxKnown && !ts.pg.Load()
}

// idChange tells the change may change the highest id of the table: it inserted or deleted the
// row, or changed its id
func idChange(ts *tableState, t touch) bool {
	if t.old == nil || t.new == nil {
		return true
	}
	o, okOld := ts.rowID(t.old)
	n, okNew := ts.rowID(t.new)
	return okOld != okNew || o != n
}

// writes tells the view wrote a row of the point of the table: the row of the key of the primary
// key (ix -1), or a row of the key of the unique index ix before or after it wrote it
func (v *TxView) writes(ts *tableState, ix int, key string) bool {
	vt := v.own[ts.meta.name]
	if vt == nil || vt.ts != ts {
		return false
	}
	if ix < 0 {
		return vt.rows[key] != nil
	}
	for _, c := range vt.rows {
		for _, row := range []stateRow{c.prev, c.row} {
			if row == nil {
				continue
			}
			if k, ok := ts.meta.ukeyOf(ts.uniqIx[ix], row); ok && k == key {
				return true
			}
		}
	}
	return false
}

// wrote are the rows of the table the view wrote, before and after it wrote them
func (v *TxView) wrote(ts *tableState) []stateRow {
	vt := v.own[ts.meta.name]
	if vt == nil || vt.ts != ts {
		return nil
	}
	var rows []stateRow
	for _, c := range vt.rows {
		for _, row := range []stateRow{c.prev, c.row} {
			if row != nil {
				rows = append(rows, row)
			}
		}
	}
	return rows
}
