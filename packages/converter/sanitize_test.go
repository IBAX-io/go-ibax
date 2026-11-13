/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package converter

import (
	"fmt"
	"strings"
	"testing"
)

// oldSanitize is Sanitize without the names it keeps as they are
func oldSanitize(name string, available string) string {
	out := make([]rune, 0, len(name))
	for _, ch := range name {
		if ch > 127 || (ch >= '0' && ch <= '9') || ch == '_' || (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') || strings.IndexRune(available, ch) >= 0 {
			out = append(out, ch)
		}
	}
	return string(out)
}

// oldParseTable is ParseTable with fmt.Sprintf and oldSanitize
func oldParseTable(tblname string, defaultEcosystem int64) string {
	ecosystem, name := ParseName(tblname)
	if ecosystem == 0 {
		if FirstEcosystemTables[tblname] {
			ecosystem = 1
		} else {
			ecosystem = defaultEcosystem
		}
		name = tblname
	}
	return strings.ToLower(fmt.Sprintf(`%d_%s`, ecosystem, oldSanitize(name, ``)))
}

var sanitizeAvailable = []string{``, ` ->+`, `->$`, `- `, `+.- `, `é`, `\x00`, `ſ@`}

func checkSanitize(t *testing.T, s string, eco int64) {
	for _, available := range sanitizeAvailable {
		if got, want := Sanitize(s, available), oldSanitize(s, available); got != want {
			t.Fatalf("Sanitize(%q, %q) = %q, want %q", s, available, got, want)
		}
	}
	if got, want := ParseTable(s, eco), oldParseTable(s, eco); got != want {
		t.Fatalf("ParseTable(%q, %d) = %q, want %q", s, eco, got, want)
	}
}

func TestSanitize(t *testing.T) {
	for _, s := range []string{"", "keys", "@1keys", "@12Keys", "Keys", "1_keys", "a b", "info->a", "+amount",
		"-amount", "x\xff", "ſ", "K", "İ", "@0x", "@1", "keys;drop", "名字", "$and", "a\x00b"} {
		for _, eco := range []int64{0, 1, 2, -1, 1 << 40} {
			checkSanitize(t, s, eco)
		}
	}
}

func FuzzSanitize(f *testing.F) {
	for _, s := range []string{"keys", "@1keys", "a->b", "x\xff", "ſK"} {
		f.Add(s, int64(1))
	}
	f.Fuzz(func(t *testing.T, s string, eco int64) {
		checkSanitize(t, s, eco)
		checkSanitize(t, expandName(s), eco)
	})
}
