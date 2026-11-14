//go:build b7check

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package queryBuilder

import "testing"

// TestAppendCheck binds the seeds without a reference of its own: the b7check build compares each
// statement with fmt %q and panics on a difference
func TestAppendCheck(t *testing.T) {
	for _, a := range appenderSeeds {
		for _, b := range appenderSeeds {
			AppendBoundStatement(nil, "q", []any{a, b})
		}
	}
}
