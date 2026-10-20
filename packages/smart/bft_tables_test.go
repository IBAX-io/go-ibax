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

func TestBftTablesDenied(t *testing.T) {
	for _, tc := range []struct {
		ecosystem int64
		name      string
		denied    bool
	}{
		{1, "bft_evidence", true},
		{1, "BFT_Reviews", true},
		{1, "@1bft_evidence", true},
		{5, "@1bft_reviews", true},
		{5, "bft_evidence", false}, // 5_bft_evidence, a table of ecosystem 5
		{1, "bad_blocks", false},
	} {
		sc := &SmartContract{TxSmart: &types.SmartTransaction{Header: &types.Header{EcosystemID: tc.ecosystem}}}
		if err := sc.denyBftTable(tc.name); errors.Is(err, errBftTable) != tc.denied {
			t.Errorf("ecosystem %d, table %s: %v", tc.ecosystem, tc.name, err)
		}
	}
	// a write is refused before any permission is read, also with the full access
	sc := &SmartContract{FullAccess: true, TxSmart: &types.SmartTransaction{Header: &types.Header{EcosystemID: 1}}}
	for _, table := range []string{"1_bft_evidence", "1_bft_reviews"} {
		for _, action := range []string{"insert", "update", "delete", "new_column"} {
			if err := sc.AccessTable(table, action); !errors.Is(err, errBftTable) {
				t.Errorf("AccessTable %s %s: %v", table, action, err)
			}
			if _, err := sc.AccessTablePerm(table, action); !errors.Is(err, errBftTable) {
				t.Errorf("AccessTablePerm %s %s: %v", table, action, err)
			}
		}
	}
}
