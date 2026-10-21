/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import "testing"

func TestLargestTxSize(t *testing.T) {
	cases := []struct {
		txs  [][]byte
		want int
	}{
		{nil, 0},
		{[][]byte{make([]byte, 3)}, 3},
		{[][]byte{make([]byte, 2), make([]byte, 7), make([]byte, 5)}, 7},
	}
	for _, c := range cases {
		if got := LargestTxSize(c.txs); got != c.want {
			t.Errorf("LargestTxSize(%d txs) = %d, want %d", len(c.txs), got, c.want)
		}
	}
}
