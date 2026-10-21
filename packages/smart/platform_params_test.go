/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/types"
)

func TestDBWriteDeniesPlatformParameters(t *testing.T) {
	values := types.LoadMap(map[string]any{"value": "1"})
	where := types.LoadMap(map[string]any{"id": 1})
	for _, eco := range []int64{1, 2} {
		sc := &SmartContract{TxSmart: &types.SmartTransaction{Header: &types.Header{EcosystemID: eco}}}
		for _, name := range []string{"platform_parameters", "@1platform_parameters", "@1Platform_Parameters"} {
			if eco != 1 && name == "platform_parameters" {
				continue
			}
			if _, _, err := DBInsert(sc, name, values); err == nil {
				t.Errorf("ecosystem %d: DBInsert(%q) was allowed", eco, name)
			}
			if _, err := DBUpdateExt(sc, name, where, values); err == nil {
				t.Errorf("ecosystem %d: DBUpdateExt(%q) was allowed", eco, name)
			}
		}
	}
}
