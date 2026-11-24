/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"errors"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/types"
)

// A system transaction of the execution of an ordered block (a delayed contract) names the node
// that runs it: without one, or without a contract, it is refused before anything reads the
// database, as a delayed contract signed by a node is
func TestSystemSignedBy(t *testing.T) {
	header := &types.Header{EcosystemID: 1, KeyID: 7}
	for _, c := range []struct {
		name     string
		signedBy int64
		contract *Contract
	}{
		{"no node", 0, &Contract{Name: "@1CheckNodesBan"}},
		{"no contract", 5, nil},
	} {
		sc := &SmartContract{System: true, TxSmart: &types.SmartTransaction{Header: header, SignedBy: c.signedBy}, TxContract: c.contract}
		if _, err := sc.GetSignedBy(nil); !errors.Is(err, errDelayedContract) {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
}
