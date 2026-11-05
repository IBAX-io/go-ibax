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

// pointReader is a reader of a point; writes tells the run also wrote the row of the key of the
// primary key it read
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

// scanReader is a reader of the rows of a table that hold a condition, of every row for nil
type scanReader struct {
	Reader
	cond *bcond
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
// later run of the transaction is. It tells whether it kept them and a commit since the view
// began may have changed what the view read before the run was a reader: the run has to check
// it holds, as no commit woke it.
func (st *BlockState) Register(i int, gen uint32, v *TxView) bool {
	ri := st.readers.Load()
	if ri == nil {
		return false
	}
	ri.mu.Lock()
	kept := ri.register(i, gen, v)
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
			ri.points[k] = append(ri.points[k], pointReader{Reader: r, writes: rd.ix < 0 && v.writes(rd.ts, rd.key)})
		default:
			ri.scans[rd.ts] = append(ri.scans[rd.ts], scanReader{Reader: r, cond: rd.cond})
		}
	}
	for _, rd := range v.ids {
		ri.ids[rd.ts] = append(ri.ids[rd.ts], idReader{Reader: r, own: rd.own, value: rd.value})
	}
	return true
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
// of the transactions committed and of the runs after which another ran. A reader is live as it
// was before the commit, so the readers it wakes do not depend on the order of the rows.
func (ri *readerIndex) wake(i int, touches []touch) {
	ri.committed = max(ri.committed, i+1)
	woken := map[int]uint32{}
	woke := func(r Reader) {
		if _, ok := woken[r.I]; !ok {
			woken[r.I] = r.Gen
			ri.woken = append(ri.woken, r)
		}
	}
	// pick wakes the readers of the key in the order of the block up to the first that wrote its
	// row: the commit of that one changes the row again, and wakes those after it
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
	var idTables []*tableState
	for _, t := range touches {
		ts := t.ts
		if _, ok := ri.ids[ts]; ok && idChange(ts, t) && !slices.Contains(idTables, ts) {
			idTables = append(idTables, ts)
		}
		pick(readerKey{ts: ts, ix: -1, key: t.pk})
		for j, ix := range ts.uniqIx {
			for _, row := range []stateRow{t.old, t.new} {
				if row == nil {
					continue
				}
				if k, ok := ts.meta.ukeyOf(ix, row); ok {
					pick(readerKey{ts: ts, ix: j, key: k})
				}
			}
		}
		if list := ri.scans[ts]; len(list) > 0 {
			kept := list[:0]
			for _, r := range list {
				if !ri.live(r.Reader) {
					continue
				}
				if t.old != nil && r.cond.matches(t.old) || t.new != nil && r.cond.matches(t.new) {
					woke(r.Reader)
					continue
				}
				kept = append(kept, r)
			}
			ri.scans[ts] = kept
		}
	}
	// a reader of the highest id holds while it is what the state now and the ids the
	// transactions before the reader take tell
	for _, ts := range idTables {
		list := ri.ids[ts]
		kept := list[:0]
		for _, r := range list {
			if !ri.live(r.Reader) {
				continue
			}
			if now, ok := ri.idNow(ts, r.I); !ok || max(now, r.own) != r.value {
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

// idNow is the highest id of the table the transaction i reads now, and whether the state knows
// it: the highest committed, and the ids the transactions before i the state has not committed
// take
func (ri *readerIndex) idNow(ts *tableState, i int) (int64, bool) {
	if ri.st == nil {
		return ts.max, ts.maxKnown && !ts.pg.Load()
	}
	ri.st.idMu.Lock()
	defer ri.st.idMu.Unlock()
	return ts.max + ri.st.predicted(i, ts.meta.name), ts.maxKnown && !ts.pg.Load()
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

// writes tells the view wrote the row of the key of the primary key of the table
func (v *TxView) writes(ts *tableState, pk string) bool {
	vt := v.own[ts.meta.name]
	return vt != nil && vt.ts == ts && vt.rows[pk] != nil
}
