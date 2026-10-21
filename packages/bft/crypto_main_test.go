/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package bft

import (
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

// The tests run on a network of P-256 node keys and SHA-256, unless a test selects other algorithms
func TestMain(m *testing.M) {
	crypto.InitAsymAlgo(crypto.AsymAlgo_ECC_P256.String())
	crypto.InitHashAlgo(crypto.HashAlgo_SHA256.String())
	os.Exit(m.Run())
}
