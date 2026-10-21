/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package utils

import (
	"net/http"
	"testing"
)

func TestLogHeadersHidesCredentials(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer secret")
	h.Add("Cookie", "a=1")
	h.Add("Cookie", "b=2")
	h.Set("Content-Type", "application/json")
	out := LogHeaders(h)
	for _, name := range []string{"Authorization", "Cookie"} {
		if v := out.Values(name); len(v) != 1 || v[0] != "[hidden]" {
			t.Errorf("%s logged as %q", name, v)
		}
	}
	if out.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type logged as %q", out.Get("Content-Type"))
	}
	if h.Get("Authorization") != "Bearer secret" {
		t.Error("the request headers were changed")
	}
	if out := LogHeaders(http.Header{"Accept": {"*/*"}}); len(out) != 1 {
		t.Errorf("headers the request did not have were added: %v", out)
	}
}
