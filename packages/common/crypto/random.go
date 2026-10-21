/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package crypto

import (
	crand "crypto/rand"
	"math/big"
)

// RandSeq returns a random string of n letters and digits from crypto/rand
func RandSeq(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	max := big.NewInt(int64(len(letters)))
	b := make([]byte, n)
	for i := range b {
		k, err := crand.Int(crand.Reader, max)
		if err != nil {
			panic(err)
		}
		b[i] = letters[k.Int64()]
	}
	return string(b)
}

// RandInt63 returns a non-negative random int64 from crypto/rand
func RandInt63() int64 {
	n, err := crand.Int(crand.Reader, new(big.Int).Lsh(big.NewInt(1), 63))
	if err != nil {
		panic(err)
	}
	return n.Int64()
}
