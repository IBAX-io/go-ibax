/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"errors"
	"testing"
)

func TestNodeKeyUsable(t *testing.T) {
	cases := []struct {
		name  string
		found bool
		key   Key
		want  error
	}{
		{"missing", false, Key{}, ErrNodeKeyNotRegistered},
		{"deleted", true, Key{Deleted: 1}, ErrNodeKeyNotRegistered},
		{"blocked", true, Key{Blocked: 1}, ErrNodeKeyNotRegistered},
		{"enabled", true, Key{}, nil},
	}
	for _, c := range cases {
		if got := nodeKeyUsable(c.found, &c.key); !errors.Is(got, c.want) {
			t.Errorf("%s: nodeKeyUsable = %v, want %v", c.name, got, c.want)
		}
	}
}
