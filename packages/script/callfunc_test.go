/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package script

import "testing"

// An extended function without parameters runs like any other
func TestCallFuncWithoutParams(t *testing.T) {
	vm := NewVM()
	vm.Extern = true
	vm.Extend(&ExtendData{map[string]any{"Answer": func() string { return "ok" }}, nil, nil})
	if err := vm.Compile([]rune(`func f() string { return Answer() }`),
		&OwnerInfo{StateID: 1, Active: true, TableID: 1}); err != nil {
		t.Fatal(err)
	}
	out, err := vm.Call(`f`, nil, map[string]any{`rt_state`: uint32(1), `data`: make([]any, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != "ok" {
		t.Errorf("result %v, want ok", out[0])
	}
}
