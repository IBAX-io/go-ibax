/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"math"
	"testing"
)

// TestHeat checks the pairs of iswap-hot take swaps in proportion to 1/(r+1)^skew, the same each run
func TestHeat(t *testing.T) {
	const n, runs = 120, 1_200_000
	for _, skew := range []float64{0, 1, 1.2} {
		heat := heatOf(n, skew)
		got := make([]int, n)
		for seq := range runs {
			got[heat(seq)]++
		}
		total := 0.0
		for r := range n {
			total += 1 / math.Pow(float64(r+1), skew)
		}
		for _, r := range []int{0, 1, 9, 59, 119} {
			want := runs / math.Pow(float64(r+1), skew) / total
			if math.Abs(float64(got[r])-want) > 5*math.Sqrt(want)+1 {
				t.Errorf("skew %v: pair %d took %d swaps, want about %.0f", skew, r, got[r], want)
			}
		}
		again := heatOf(n, skew)
		for seq := range 1000 {
			if again(seq) != heat(seq) {
				t.Fatalf("skew %v: swap %d takes pair %d, then %d", skew, seq, heat(seq), again(seq))
			}
		}
	}
}
