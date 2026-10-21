/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package queryBuilder

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/types"
)

// A string value of $in and $nin is quoted as the values of every other operator are
func TestGetWhereInQuotesStringValue(t *testing.T) {
	for _, c := range []struct {
		op   string
		want string
	}{
		{`$in`, `"name" in ('a''b')`},
		{`$nin`, `"name" not in ('a''b')`},
	} {
		inner := types.NewMap()
		inner.Set(c.op, `a'b`)
		where := types.NewMap()
		where.Set(`name`, inner)
		got, err := GetWhere(where)
		if err != nil {
			t.Fatalf("%s: %v", c.op, err)
		}
		if got != `(`+c.want+`)` {
			t.Errorf("%s: %s, want (%s)", c.op, got, c.want)
		}
	}
}
