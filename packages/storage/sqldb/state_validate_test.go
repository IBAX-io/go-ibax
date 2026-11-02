/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestChangedSincePoint: a point read tells a commit changed what it read exactly when a scan
// of every event of the table since the read tells it
func TestChangedSincePoint(t *testing.T) {
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
	ts := &tableState{meta: m, uniq: []*smap[string]{new(smap[string])}, uniqIx: []int{1}}
	ts.resetKeys()

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
	cur := make(map[int]stateRow)
	for seq := uint64(1); seq <= 400; seq++ {
		for range 1 + rnd.Intn(3) {
			id := rnd.Intn(12)
			old := cur[id]
			var nw stateRow
			if old == nil || rnd.Intn(4) > 0 {
				nw = row(id)
			}
			if old == nil && nw == nil {
				continue
			}
			cur[id] = nw
			ts.addEvent(stateEvent{seq: seq, old: old, new: nw}, m.pkOf(stateRow{str(id), nil, nil, nil}))
		}
	}
	checked := 0
	for range 20000 {
		r := stateRead{ts: ts, seq: uint64(rnd.Intn(401))}
		kids := []*bcond{}
		if rnd.Intn(2) == 0 {
			id := str(rnd.Intn(12))
			kids = append(kids, eq(0, id))
			r.ix = -1
		} else {
			e, n := str(rnd.Intn(2)), str(rnd.Intn(4))
			kids = append(kids, eq(1, e), eq(2, n))
			r.ix = 0
		}
		if rnd.Intn(2) == 0 {
			kids = append(kids, eq(3, str(rnd.Intn(3))))
		}
		r.cond = &bcond{op: cAnd, kids: kids}
		j, key, ok := pointKey(m, ts, r.cond)
		require.True(t, ok)
		require.Equal(t, r.ix, j)
		r.key = key
		scan := r
		r.point = true
		got, want := ts.changedSince(&r), ts.changedSince(&scan)
		require.Equal(t, want, got, "read %+v", r)
		if want {
			checked++
		}
	}
	require.Greater(t, checked, 1000)
}
