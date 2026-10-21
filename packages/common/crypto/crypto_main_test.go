/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package crypto

import (
	"os"
	"testing"
)

// The tests run on a network of P-256 node keys and SHA-256, unless a test selects other algorithms
func TestMain(m *testing.M) {
	setAsymAlgo(AsymAlgo_ECC_P256)
	setHashAlgo(HashAlgo_SHA256)
	os.Exit(m.Run())
}

// Signing or hashing before the algorithms are selected panics instead of using enum zero
func TestUninitializedAlgorithmsPanic(t *testing.T) {
	defer asymAlgo.Store(asymAlgo.Load())
	defer hashAlgo.Store(hashAlgo.Load())
	asymAlgo.Store(nil)
	hashAlgo.Store(nil)
	for name, f := range map[string]func(){
		"Hash":     func() { Hash([]byte("x")) },
		"NodeAlgo": func() { NodeAlgo() },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s ran without the algorithms selected", name)
				}
			}()
			f()
		}()
	}
}
