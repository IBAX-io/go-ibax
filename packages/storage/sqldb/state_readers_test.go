/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"math"
	"math/rand"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReadersWake: a commit wakes readers whose reads it changed, as the validation of their reads
// after it tells: those of the rows it changed, by a unique index, by a condition and of the
// highest id, and of the key of a primary key in the order of the block up to the first that
// also wrote its row; through readers that run again and register anew
func TestReadersWake(t *testing.T) {
	m := &tableMeta{name: "t", byName: make(map[string]*colMeta), pk: []string{"id"},
		uniques: [][]string{{"id"}, {"eco", "name"}}}
	for _, c := range []struct {
		name string
		typ  colType
	}{{"id", newColType(20, "int8", -1)}, {"eco", newColType(20, "int8", -1)},
		{"name", newColType(25, "text", -1)}, {"amount", newColType(20, "int8", -1)}} {
		cm := &colMeta{name: c.name, typ: c.typ}
		m.cols = append(m.cols, cm)
		m.byName[c.name] = cm
	}
	ts := &tableState{meta: m, uniq: []*smap[string]{new(smap[string])}, uniqIx: []int{1}, hasID: true}
	ts.resetKeys()
	ts.maxLoaded, ts.maxKnown = true, true

	rnd := rand.New(rand.NewSource(1))
	str := func(n int) *string { s := strconv.Itoa(n); return &s }
	row := func(id int) stateRow {
		r := stateRow{str(id), str(rnd.Intn(2)), str(rnd.Intn(4)), str(rnd.Intn(3))}
		if rnd.Intn(8) == 0 {
			r[2] = nil
		}
		return r
	}
	eq := func(col int, v *string) *bcond {
		fam, _ := family(m.cols[col].typ, m.cols[col].typ)
		return &bcond{op: cCmp, col: col, cmp: "=", fam: fam, vals: []*string{v}}
	}
	var seq uint64
	// a view of reads at the seq now: points by the primary key, which it may write, or the
	// unique index, scans by a condition or of every row, and reads of the highest id
	view := func() *TxView {
		v := &TxView{start: seq, own: map[string]*viewTable{}}
		for range 1 + rnd.Intn(3) {
			switch rnd.Intn(5) {
			case 0, 1:
				var r stateRead
				if rnd.Intn(2) == 0 {
					r = stateRead{ts: ts, seq: seq, cond: eq(0, str(rnd.Intn(12))), ix: -1}
				} else {
					r = stateRead{ts: ts, seq: seq, cond: &bcond{op: cAnd, kids: []*bcond{eq(1, str(rnd.Intn(2))), eq(2, str(rnd.Intn(4)))}}, ix: 0}
				}
				j, key, ok := pointKey(m, ts, r.cond)
				require.True(t, ok)
				require.Equal(t, r.ix, j)
				r.point, r.key = true, key
				v.reads = append(v.reads, r)
				if r.ix < 0 && rnd.Intn(2) == 0 {
					vt := v.own["t"]
					if vt == nil {
						vt = &viewTable{ts: ts, rows: map[string]*vchange{}}
						v.own["t"] = vt
					}
					vt.rows[key] = &vchange{}
				}
			case 2:
				v.reads = append(v.reads, stateRead{ts: ts, seq: seq, cond: eq(3, str(rnd.Intn(3)))})
			case 3:
				if rnd.Intn(4) == 0 {
					v.reads = append(v.reads, stateRead{ts: ts, seq: seq})
				}
			case 4:
				v.ids = append(v.ids, idRead{ts: ts, own: math.MinInt64, value: ts.max})
			}
		}
		return v
	}
	ri := newReaderIndex()
	// runs are the views of the last runs of the transactions not committed, gens their runs
	runs := map[int]*TxView{}
	gens := map[int]uint32{}
	// idStale tells a read of the highest id of the view does not hold, as the validation tells
	idStale := func(v *TxView) bool {
		for _, r := range v.ids {
			if max(ts.max, r.own) != r.value {
				return true
			}
		}
		return false
	}
	changed := func(v *TxView) bool {
		for i := range v.reads {
			if ts.changedSince(&v.reads[i]) {
				return true
			}
		}
		return idStale(v)
	}
	// expect are the readers the commit of the touches wakes: of a key of the primary key in
	// their order up to the first that wrote its row, every other reader of what changed
	expect := func(touches []touch) map[int]bool {
		want := map[int]bool{}
		for _, t := range touches {
			if idChange(ts, t) {
				for i, v := range runs {
					if idStale(v) {
						want[i] = true
					}
				}
			}
			var keyed []int
			for i, v := range runs {
				hit := false
				for _, rd := range v.reads {
					switch {
					case rd.point && rd.ix < 0:
						if rd.key == t.pk {
							keyed = append(keyed, i)
						}
					case rd.point:
						for _, row := range []stateRow{t.old, t.new} {
							if row == nil {
								continue
							}
							if k, ok := m.ukeyOf(ts.uniqIx[rd.ix], row); ok && k == rd.key {
								hit = true
							}
						}
					default:
						hit = hit || t.old != nil && rd.cond.matches(t.old) || t.new != nil && rd.cond.matches(t.new)
					}
				}
				if hit {
					want[i] = true
				}
			}
			slices.Sort(keyed)
			for _, i := range slices.Compact(keyed) {
				want[i] = true
				if runs[i].writes(ts, t.pk) {
					break
				}
			}
		}
		return want
	}
	cur := make(map[int]stateRow)
	next, woken, checked, stale := 0, 0, 0, 0
	for k := range 3000 {
		// the workers run ahead of the commit, at most 64 transactions
		for ; next < k+64; next++ {
			v := view()
			runs[next], gens[next] = v, 0
			require.True(t, ri.register(next, 0, v))
		}
		delete(runs, k)
		seq++
		var touches []touch
		ids := map[int]bool{}
		for range 1 + rnd.Intn(3) {
			id := rnd.Intn(12)
			if ids[id] {
				continue
			}
			ids[id] = true
			old := cur[id]
			var nw stateRow
			if old == nil || rnd.Intn(4) > 0 {
				nw = row(id)
			}
			if old == nil && nw == nil {
				continue
			}
			cur[id] = nw
			pk := m.pkOf(stateRow{str(id), nil, nil, nil})
			ts.addEvent(stateEvent{seq: seq, old: old, new: nw}, pk)
			touches = append(touches, touch{ts: ts, pk: pk, old: old, new: nw})
		}
		ts.max = 0
		for id, r := range cur {
			if r != nil {
				ts.max = max(ts.max, int64(id))
			}
		}
		want := expect(touches)
		ri.wake(k, touches)
		got := map[int]bool{}
		for _, r := range ri.woken {
			require.False(t, got[r.I], "woken twice: %d", r.I)
			require.Equal(t, gens[r.I], r.Gen)
			got[r.I] = true
		}
		ri.woken = nil
		for i, v := range runs {
			require.Equal(t, want[i], got[i], "transaction %d after the commit %d", i, k)
			if got[i] {
				// a reader woken read what changed
				require.True(t, changed(v), "transaction %d after the commit %d", i, k)
				checked++
			} else if changed(v) {
				stale++
			}
		}
		// the woken run again, now or after later commits
		for i := range got {
			woken++
			gens[i]++
			v := view()
			runs[i] = v
			require.True(t, ri.register(i, gens[i], v))
			// a late run of an earlier generation is not a reader
			require.False(t, ri.register(i, gens[i]-1, view()))
		}
	}
	require.Greater(t, checked, 2000)
	require.Equal(t, checked, woken)
	// readers behind one that wrote the row wait for its commit
	require.Greater(t, stale, 1000)
}
