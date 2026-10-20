/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package language

import "testing"

func source(state int, name string) (string, bool) {
	c, ok := lang[state]
	if !ok {
		return "", false
	}
	res, ok := c.res[name]
	if !ok {
		return "", false
	}
	return (*res)["en"], true
}

// Restore puts back the sources a block changed, added to an ecosystem and added in a new one
func TestSnapshot(t *testing.T) {
	lang = make(map[int]*cacheLang)
	if err := UpdateLang(1, "kept", `{"en":"kept"}`); err != nil {
		t.Fatal(err)
	}
	if err := UpdateLang(1, "edited", `{"en":"before"}`); err != nil {
		t.Fatal(err)
	}
	snap := TakeSnapshot()
	for _, u := range [][3]any{{1, "edited", `{"en":"after"}`}, {1, "added", `{"en":"added"}`}, {2, "other", `{"en":"other"}`}} {
		if err := UpdateLang(u[0].(int), u[1].(string), u[2].(string)); err != nil {
			t.Fatal(err)
		}
	}
	if v, _ := source(1, "edited"); v != "after" {
		t.Fatalf("edited = %s", v)
	}
	Restore(snap)
	if v, _ := source(1, "kept"); v != "kept" {
		t.Fatalf("kept = %s", v)
	}
	if v, _ := source(1, "edited"); v != "before" {
		t.Fatalf("edited after Restore = %s", v)
	}
	if _, ok := source(1, "added"); ok {
		t.Fatal("the added source is still there")
	}
	if _, ok := lang[2]; ok {
		t.Fatal("the new ecosystem is still there")
	}
	// the snapshot is a copy: what the next block changes does not reach it
	if err := UpdateLang(1, "added", `{"en":"added"}`); err != nil {
		t.Fatal(err)
	}
	Restore(snap)
	if _, ok := source(1, "added"); ok {
		t.Fatal("the second block reached the snapshot")
	}
}
