/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"math"
	"math/rand"
	"strconv"
	"testing"
)

// BenchmarkWakeIDReaders commits, one after the other, the n transactions of a block that each
// insert a row and read the highest id of the table, as the swaps of iSwap do: each commit checks
// the readers of the highest id after it
func BenchmarkWakeIDReaders(b *testing.B) {
	for _, n := range []int{500, 1000, 2000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			m := &tableMeta{name: "t", byName: make(map[string]*colMeta), pk: []string{"id"}, uniques: [][]string{{"id"}}}
			cm := &colMeta{name: "id", typ: newColType(20, "int8", -1)}
			m.cols, m.byName["id"] = []*colMeta{cm}, cm
			for b.Loop() {
				b.StopTimer()
				ts := &tableState{meta: m, hasID: true, maxLoaded: true, maxKnown: true}
				ts.resetKeys()
				st := &BlockState{txs: make([]txSlot, n), contracts: map[string]map[string]int{}}
				ri := newReaderIndex()
				ri.st = st
				for i := range n {
					ids := map[string]int{"t": 1}
					st.txs[i].ids.Store(&ids)
					ri.gens[i] = 0
					ri.ids[ts] = append(ri.ids[ts], idReader{Reader: Reader{I: i}, own: math.MinInt64, value: int64(i)})
				}
				b.StartTimer()
				for i := range n {
					id := strconv.Itoa(i + 1)
					st.next = i + 1
					ts.max = int64(i + 1)
					ri.wake(i, []touch{{ts: ts, pk: id, new: stateRow{&id}}})
					if len(ri.woken) != 0 {
						b.Fatalf("commit %d woke %v", i, ri.woken)
					}
				}
			}
		})
	}
}

// BenchmarkWakeHotKey commits, one after the other, the n transactions of a block that each read
// and write the row of one key, as the swaps of a pair do: each commit wakes the next, which
// registers its run again
func BenchmarkWakeHotKey(b *testing.B) {
	for _, n := range []int{500, 1000, 2000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			m := &tableMeta{name: "t", byName: make(map[string]*colMeta), pk: []string{"id"}, uniques: [][]string{{"id"}}}
			cm := &colMeta{name: "id", typ: newColType(20, "int8", -1)}
			m.cols, m.byName["id"] = []*colMeta{cm}, cm
			id := "1"
			for b.Loop() {
				b.StopTimer()
				ts := &tableState{meta: m}
				ts.resetKeys()
				ri := newReaderIndex()
				k := readerKey{ts: ts, ix: -1, key: id}
				// the workers register the first runs out of the order of the block
				for _, i := range rand.New(rand.NewSource(1)).Perm(n) {
					ri.gens[i] = 0
					ri.points[k] = append(ri.points[k], pointReader{Reader: Reader{I: i}, writes: true})
				}
				b.StartTimer()
				for i := range n {
					ri.wake(i, []touch{{ts: ts, pk: id, new: stateRow{&id}}})
					for _, r := range ri.woken {
						ri.points[k] = append(ri.points[k], pointReader{Reader: Reader{I: r.I, Gen: r.Gen + 1}, writes: true})
					}
					ri.woken = nil
				}
			}
		})
	}
}
