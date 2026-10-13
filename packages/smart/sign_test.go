/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/types"
)

// A transaction that names a signer other than its own key is a node-signed delayed contract, which
// always has a contract. The transfer transactions carry none, so a header that names another
// signer must be refused here rather than read through a nil contract.
func TestGetSignedByRefusesSignerWithoutContract(t *testing.T) {
	sc := &SmartContract{
		TxSmart: &types.SmartTransaction{
			Header:   &types.Header{KeyID: 100, EcosystemID: 1},
			SignedBy: 200,
		},
	}
	if _, err := sc.GetSignedBy(nil); err == nil {
		t.Fatal("a transaction signed by another key and carrying no contract was accepted")
	}
}
