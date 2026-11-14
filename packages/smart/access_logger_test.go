/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/types"
)

// TestAccessMakesNoLogger: a check of access that passes makes no logger of the contract; one is
// made only for what is logged
func TestAccessMakesNoLogger(t *testing.T) {
	newSC := func(full bool) *SmartContract {
		return &SmartContract{FullAccess: full, TxSmart: &types.SmartTransaction{Header: &types.Header{EcosystemID: 1}}}
	}
	checks := []struct {
		name  string
		full  bool
		check func(sc *SmartContract) error
	}{
		{"columns, full access", true, func(sc *SmartContract) error {
			return sc.AccessColumns("1_keys", &[]string{"amount"}, true)
		}},
		{"columns of the parameters, read", false, func(sc *SmartContract) error {
			return sc.AccessColumns("parameters", &[]string{"value"}, false)
		}},
		{"table of the parameters, read", false, func(sc *SmartContract) error {
			_, err := sc.AccessTablePerm("app_params", "read")
			return err
		}},
	}
	for _, c := range checks {
		sc := newSC(c.full)
		if err := c.check(sc); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if sc.logger != nil {
			t.Fatalf("%s: a logger is made", c.name)
		}
	}
}
