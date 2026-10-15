/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package language

import "testing"

func TestUpdateLangLeavesUnloadedStateToLoad(t *testing.T) {
	const state = 1001
	if err := UpdateLang(state, "new_source", `{"en": "New source"}`); err != nil {
		t.Fatal(err)
	}
	if _, ok := lang[state]; ok {
		t.Fatal("UpdateLang made a cache of one source for a state whose sources are not loaded")
	}

	lang[state] = &cacheLang{map[string]*map[string]string{"old_source": {"en": "Old source"}}}
	defer delete(lang, state)
	if err := UpdateLang(state, "new_source", `{"en": "New source"}`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old_source", "new_source"} {
		if _, ok := lang[state].res[name]; !ok {
			t.Errorf("%s is not in the loaded cache", name)
		}
	}
}
