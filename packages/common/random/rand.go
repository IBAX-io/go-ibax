/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package random

import (
	"math/rand"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

// TxRand is the random source of a transaction: seeded with its hash, so every node draws the same
// numbers for it, whatever runs before it in the block and wherever it runs
func TxRand(hash []byte) *rand.Rand {
	return rand.New(rand.NewSource(int64(crypto.CalcChecksum(hash))))
}

func RandInt(min, max int) int {
	if min >= max || min == 0 || max == 0 {
		return max
	}
	return rand.Intn(max-min) + min
}

const (
	KC_RAND_KIND_NUM   = 0 // number
	KC_RAND_KIND_LOWER = 1 //
	KC_RAND_KIND_UPPER = 2 //
	KC_RAND_KIND_ALL   = 3 //
)

func Krand(size int64, kind int) []byte {
	ikind, kinds, result := kind, [][]int{{10, 48}, {26, 97}, {26, 65}}, make([]byte, size)
	is_all := kind > 2 || kind < 0
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < int(size); i++ {
		if is_all { // random ikind
			ikind = rand.Intn(3)
		}
		scope, base := kinds[ikind][0], kinds[ikind][1]
		result[i] = uint8(base + rand.Intn(scope))
	}
	return result
}

func RandNumber(size int64) string {
	result := Krand(size, KC_RAND_KIND_ALL)
	return string(result[:])
}
