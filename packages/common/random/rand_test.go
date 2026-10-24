/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package random

import (
	"math/rand"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"

	"github.com/stretchr/testify/assert"
)

func TestRand(t *testing.T) {
	cases := [][]int64{
		{3434102771992637744, 1523931518789473682},
		{3434102771992637744, 1523931518789473682},
	}

	for _, values := range cases {
		r := TxRand([]byte("reset"))
		for _, v := range values {
			assert.Equal(t, v, r.Int63())
		}
	}
}

// TestTxRandSequence: the numbers of TxRand are those of a source rand.NewSource makes of the seed,
// for each way rand.Rand draws them
func TestTxRandSequence(t *testing.T) {
	for _, hash := range [][]byte{[]byte("reset"), {}, {1, 2, 3}} {
		got := TxRand(hash)
		want := rand.New(rand.NewSource(int64(crypto.CalcChecksum(hash))))
		for i := 0; i < 100; i++ {
			assert.Equal(t, want.Int63(), got.Int63())
			assert.Equal(t, want.Uint64(), got.Uint64())
			assert.Equal(t, want.Float64(), got.Float64())
			assert.Equal(t, want.Int63n(1000), got.Int63n(1000))
			assert.Equal(t, want.Intn(77), got.Intn(77))
		}
		got.Seed(9)
		want.Seed(9)
		assert.Equal(t, want.Int63(), got.Int63())
	}
}
