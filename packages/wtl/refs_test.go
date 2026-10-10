/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package wtl

import (
	"testing"
)

func TestRead(t *testing.T) {
	for _, c := range []struct {
		name, src string
		ecosystem int64
		want      string
	}{
		{"legacy template", `Div(){Include(Name: header)}`, 1, `{}`},
		{"no element", "@wtl 1\n", 1, `{}`},
		{"page", "@wtl 1\n<Page title=\"x\">\n  <Include name=\"pager_header\" props={{ description: t('a}b') }} />\n  <Include name=\"welcome\" />\n  <Include name=\"welcome\" />\n</Page>\n", 1,
			`{"blocks":["pager_header","welcome"]}`},
		{"own and other ecosystem", "@wtl 1\n<Page><Include name=\"@5own\" /><Include name=\"@1shared\" ecosystem={1} /><Include name=\"@55other\" ecosystem={55} /></Page>\n", 5,
			`{"blocks":["@1shared","@55other","own"]}`},
		{"menu", "@wtl 1\n<Menu>\n<MenuItem title={t('home')} icon=\"house\" page=\"default_page\" />\n<MenuGroup title=\"g\"><MenuItem title=\"n\" page=\"notifications\" /><MenuItem title=\"w\" href=\"https://x\" /></MenuGroup>\n</Menu>\n", 1,
			`{"pages":["default_page","notifications"]}`},
		{"names in text, comments and expressions are not elements", "@wtl 1\n<Page>{/* <Include name=\"a\" /> */}{'<Include name=\"b\" />'}<Text>{\"}\"}</Text><Include name=\"c\" /></Page>\n", 1,
			`{"blocks":["c"]}`},
		{"a name given by an expression is not a reference", "@wtl 1\n<Page><Include name={x} /><Link page=\"p\" /></Page>\n", 1, `{}`},
		{"crlf and bom", "\uFEFF@wtl 1\r\n<Page>\r\n<Include name=\"a\" />\r\n</Page>\r\n", 1, `{"blocks":["a"]}`},
		{"the text after a syntax error is not read", "@wtl 1\n<Page><Include name=\"a\" /><Include name=\"b /></Page>\n", 1, `{"blocks":["a"]}`},
		{"unclosed expression", "@wtl 1\n<Page><Include name=\"a\" props={{ x: 1 } /></Page>\n", 1, `{}`},
	} {
		if got := Read(c.src, c.ecosystem).JSON(); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestIsSource(t *testing.T) {
	for src, want := range map[string]bool{
		"@wtl 1\n<Page />\n": true,
		"@wtl":               true,
		"@wtlx 1":            false,
		"Div(){}":            false,
		"":                   false,
	} {
		if got := IsSource(src); got != want {
			t.Errorf("IsSource(%q) = %v, want %v", src, got, want)
		}
	}
}
