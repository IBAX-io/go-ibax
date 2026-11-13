/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package converter

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// nameRE is the regular expression ParseName used, the reference of splitName
var nameRE = regexp.MustCompile(`(?is)^@(\d+)(\w[_\w\d]*)$`)

// nameParts expands the bytes of a fuzz input to parts of names, so that the inputs are names often
var nameParts = []string{"@", "1", "0", "9", "12", "a", "Z", "_", "x1", "ſ", "K", "S", "K",
	"é", "\xff", "\xc5", "\xe2\x84", "-", " ", "\n", "$", "@1", "İ", "�"}

func expandName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		b.WriteString(nameParts[int(s[i])%len(nameParts)])
	}
	return b.String()
}

func checkSplitName(t *testing.T, in string) {
	t.Helper()
	digits, name, ok := splitName(in)
	m := nameRE.FindStringSubmatch(in)
	if ok != (m != nil) || ok && (digits != m[1] || name != m[2]) {
		t.Fatalf("splitName(%q) = %q, %q, %v; the regular expression gives %q", in, digits, name, ok, m)
	}
	var id int64
	var want string
	if m != nil {
		id, want = StrToInt64(m[1]), m[2]
	}
	if gotID, got := ParseName(in); gotID != id || got != want {
		t.Fatalf("ParseName(%q) = %d, %q, want %d, %q", in, gotID, got, id, want)
	}
}

func TestSplitName(t *testing.T) {
	for _, in := range []string{
		"", "@", "@1", "@12", "@123", "@1a", "@1_", "@1keys", "@1Keys", "@12345contract_1",
		"@1a2b3", "@01x", "@99999999999999999999x", "1keys", "keys", "@keys", "@a1", "@1-x", "@1 x",
		"@1x ", " @1x", "@1x\n", "@1ſ", "@1K", "@1ſkK", "@1é", "@1x\xff",
		"@1\xc5", "@1\xe2\x84", "@1İ", "@1�", "@ſ1", "@1@2x", "@1\x00",
	} {
		checkSplitName(t, in)
	}
}

// TestFoldedWordLen checks every character against \w under (?i)
func TestFoldedWordLen(t *testing.T) {
	re := regexp.MustCompile(`(?i)^\w$`)
	for r := rune(0); r <= utf8.MaxRune; r++ {
		if !utf8.ValidRune(r) {
			continue
		}
		s := string(r)
		want := 0
		if re.MatchString(s) {
			want = len(s)
		}
		if got := foldedWordLen(s, 0); got != want {
			t.Fatalf("foldedWordLen(%q) = %d, want %d", s, got, want)
		}
	}
	for c := 0x80; c < 0x100; c++ {
		if got := foldedWordLen(string([]byte{byte(c)}), 0); got != 0 {
			t.Fatalf("foldedWordLen(%#x) = %d, want 0", c, got)
		}
	}
}

func FuzzSplitName(f *testing.F) {
	for _, in := range []string{"@1keys", "@12", "@1ſ", "@1K_x", "@1x\xff", "", "\x00\x01\x09"} {
		f.Add(in)
	}
	f.Fuzz(func(t *testing.T, in string) {
		checkSplitName(t, in)
		checkSplitName(t, expandName(in))
	})
}

var benchNames = []string{"@1keys", "@1contract_name", "@12345SomeContract", "keys", "@1"}

func BenchmarkParseName(b *testing.B) {
	for b.Loop() {
		for _, in := range benchNames {
			ParseName(in)
		}
	}
}

func BenchmarkParseNameRegexp(b *testing.B) {
	for b.Loop() {
		for _, in := range benchNames {
			if ret := nameRE.FindStringSubmatch(in); len(ret) == 3 {
				_ = StrToInt64(ret[1])
			}
		}
	}
}
