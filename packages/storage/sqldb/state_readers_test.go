/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"maps"
	"math"
	"math/rand"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReadersWake: a commit wakes readers whose reads it changed, as the validation of their reads
// after it tells: of the highest id, and of a point, by the primary key or a unique index, and by
// a condition in the order of the block up to the first that also wrote what it read; through
// readers that run again and register anew
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
	// a view of reads at the seq now: points by the primary key or the unique index, scans by a
	// condition or of every row, each of which it may write, and reads of the highest id
	view := func() *TxView {
		v := &TxView{start: seq, own: map[string]*viewTable{}}
		write := func(key string, c *vchange) {
			vt := v.own["t"]
			if vt == nil {
				vt = &viewTable{ts: ts, rows: map[string]*vchange{}}
				v.own["t"] = vt
			}
			vt.rows[key] = c
		}
		// a row of the values, by column, the read requires, the others random
		rowOf := func(vals map[int]*string) stateRow {
			r := row(rnd.Intn(12))
			for col, v := range vals {
				r[col] = v
			}
			return r
		}
		for range 1 + rnd.Intn(3) {
			switch rnd.Intn(5) {
			case 0, 1:
				var r stateRead
				var vals map[int]*string
				if rnd.Intn(2) == 0 {
					vals = map[int]*string{0: str(rnd.Intn(12))}
					r = stateRead{ts: ts, seq: seq, cond: eq(0, vals[0]), ix: -1}
				} else {
					vals = map[int]*string{1: str(rnd.Intn(2)), 2: str(rnd.Intn(4))}
					r = stateRead{ts: ts, seq: seq, cond: &bcond{op: cAnd, kids: []*bcond{eq(1, vals[1]), eq(2, vals[2])}}, ix: 0}
				}
				j, key, ok := pointKey(m, ts, r.cond)
				require.True(t, ok)
				require.Equal(t, r.ix, j)
				r.point, r.key = true, key
				v.reads = append(v.reads, r)
				if rnd.Intn(2) == 0 {
					if w := rowOf(vals); r.ix < 0 {
						write(key, &vchange{row: w})
					} else {
						write(m.pkOf(w), &vchange{row: w})
					}
				}
			case 2:
				vals := map[int]*string{3: str(rnd.Intn(3))}
				v.reads = append(v.reads, stateRead{ts: ts, seq: seq, cond: eq(3, vals[3])})
				if rnd.Intn(2) == 0 {
					w := rowOf(vals)
					write(m.pkOf(w), &vchange{row: w})
				}
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
	// expect are the readers the commit of the touches wakes: of the highest id, and of each point
	// and condition the touches changed in their order up to the first that wrote what it read
	expect := func(touches []touch) map[int]bool {
		want := map[int]bool{}
		order := slices.Sorted(maps.Keys(runs))
		if slices.ContainsFunc(touches, func(t touch) bool { return idChange(ts, t) }) {
			for i, v := range runs {
				if idStale(v) {
					want[i] = true
				}
			}
		}
		type point struct {
			ix  int
			key string
		}
		var points []point
		add := func(p point) {
			if !slices.Contains(points, p) {
				points = append(points, p)
			}
		}
		for _, t := range touches {
			add(point{-1, t.pk})
			for _, row := range []stateRow{t.old, t.new} {
				if row == nil {
					continue
				}
				if k, ok := m.ukeyOf(ts.uniqIx[0], row); ok {
					add(point{0, k})
				}
			}
		}
		for _, p := range points {
			for _, i := range order {
				if !slices.ContainsFunc(runs[i].reads, func(rd stateRead) bool { return rd.point && rd.ix == p.ix && rd.key == p.key }) {
					continue
				}
				want[i] = true
				if runs[i].writes(ts, p.ix, p.key) {
					break
				}
			}
		}
		var wrote []stateRow
		for _, i := range order {
			for _, rd := range runs[i].reads {
				if rd.point {
					continue
				}
				hit := slices.ContainsFunc(touches, func(t touch) bool {
					return t.old != nil && rd.cond.matches(t.old) || t.new != nil && rd.cond.matches(t.new)
				})
				if hit && !slices.ContainsFunc(wrote, rd.cond.matches) {
					want[i] = true
					wrote = append(wrote, runs[i].wrote(ts)...)
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

// TestReadersWakeHotKey: transactions that read a row by its unique index, or the rows of a
// condition, and each update the row, as the swaps of a pair do: the commit of each wakes the
// next alone, and a run of one with a reader before it that wrote what it read waits for a wake
func TestReadersWakeHotKey(t *testing.T) {
	m := &tableMeta{name: "t", byName: make(map[string]*colMeta), pk: []string{"id"},
		uniques: [][]string{{"id"}, {"eco", "name"}}}
	for _, name := range []string{"id", "eco", "name", "amount"} {
		cm := &colMeta{name: name, typ: newColType(20, "int8", -1)}
		m.cols = append(m.cols, cm)
		m.byName[name] = cm
	}
	ts := &tableState{meta: m, uniq: []*smap[string]{new(smap[string])}, uniqIx: []int{1}, hasID: true}
	ts.resetKeys()
	str := func(s string) *string { return &s }
	eq := func(col int, v string) *bcond {
		fam, _ := family(m.cols[col].typ, m.cols[col].typ)
		return &bcond{op: cCmp, col: col, cmp: "=", fam: fam, vals: []*string{str(v)}}
	}
	pair := func(amount string) stateRow { return stateRow{str("1"), str("1"), str("2"), str(amount)} }
	pk := m.pkOf(pair("0"))
	for _, c := range []struct {
		name string
		read func() stateRead
	}{
		{"unique index", func() stateRead {
			r := stateRead{ts: ts, cond: &bcond{op: cAnd, kids: []*bcond{eq(1, "1"), eq(2, "2")}}}
			j, key, ok := pointKey(m, ts, r.cond)
			require.True(t, ok)
			r.point, r.ix, r.key = true, j, key
			return r
		}},
		{"condition", func() stateRead { return stateRead{ts: ts, cond: eq(1, "1")} }},
	} {
		t.Run(c.name, func(t *testing.T) {
			const n = 50
			ri := newReaderIndex()
			// each run read the row as it was before the block, and wrote it
			view := func() *TxView {
				return &TxView{reads: []stateRead{c.read()}, own: map[string]*viewTable{"t": {ts: ts,
					rows: map[string]*vchange{pk: {prev: pair("0"), row: pair("9")}}}}}
			}
			for i := range n {
				require.True(t, ri.register(i, 0, view()))
			}
			gens := map[int]uint32{}
			for i := range n - 1 {
				ri.wake(i, []touch{{ts: ts, pk: pk, old: pair(strconv.Itoa(i)), new: pair(strconv.Itoa(i + 1))}})
				require.Equal(t, []Reader{{I: i + 1, Gen: gens[i+1]}}, ri.woken, "the commit %d", i)
				ri.woken = nil
				gens[i+1]++
				require.True(t, ri.register(i+1, gens[i+1], view()))
				if i+2 < n {
					// the run of the next has a reader before it that wrote what it read
					require.True(t, ri.awaits(i+2, view()))
				}
				require.False(t, ri.awaits(i+1, view()))
			}
		})
	}
}
