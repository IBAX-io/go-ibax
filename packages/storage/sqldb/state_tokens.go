/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"strings"
)

// The tokens of a statement: the state of a block reads its statements from them

const (
	tIdent = iota
	tQuoted
	tString
	tNumber
	tParam
	tPunct
)

type token struct {
	kind int
	text string
}

func (t token) is(p string) bool { return t.kind == tPunct && t.text == p }

func (t token) isName() bool { return t.kind == tIdent || t.kind == tQuoted }

// keyword is the word of an unquoted identifier
func (t token) keyword() string {
	if t.kind != tIdent {
		return ""
	}
	return t.text
}

// isKeyword tells an unquoted identifier is a reserved word of a condition
func (t token) isKeyword() bool {
	switch t.keyword() {
	case "and", "or", "not", "null", "true", "false", "in", "is", "like", "ilike", "between", "any", "all",
		"some", "exists", "case", "when", "then", "else", "end", "select", "from", "where":
		return true
	}
	return false
}

// tokenize splits a statement into its tokens: unquoted identifiers in lower case, quoted ones,
// strings, numbers, placeholders and punctuation; comments are left out
func tokenize(s string) []token {
	var toks []token
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			i++
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				i = len(s)
			} else {
				i += end + 4
			}
		case c == '"':
			var b strings.Builder
			j := i + 1
			for j < len(s) {
				if s[j] == '"' {
					if j+1 < len(s) && s[j+1] == '"' {
						b.WriteByte('"')
						j += 2
						continue
					}
					break
				}
				b.WriteByte(s[j])
				j++
			}
			toks = append(toks, token{tQuoted, b.String()})
			i = j + 1
		case c == '\'':
			str, next := quotedString(s, i, false)
			toks = append(toks, token{tString, str})
			i = next
		case (c == 'e' || c == 'E') && i+1 < len(s) && s[i+1] == '\'':
			str, next := quotedString(s, i+1, true)
			if strings.IndexByte(s[i:next], '\\') >= 0 {
				// the escapes of the string are not kept: no plan takes it
				toks = append(toks, token{tPunct, "E''"})
			} else {
				toks = append(toks, token{tString, str})
			}
			i = next
		case c == '$' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			j := i + 1
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			toks = append(toks, token{tParam, s[i:j]})
			i = j
		case c == '$':
			// a dollar-quoted string: $tag$ ... $tag$
			j := i + 1
			for j < len(s) && s[j] != '$' && isIdentChar(s[j]) {
				j++
			}
			if j < len(s) && s[j] == '$' {
				tag := s[i : j+1]
				end := strings.Index(s[j+1:], tag)
				if end < 0 {
					toks = append(toks, token{tString, s[j+1:]})
					i = len(s)
				} else {
					toks = append(toks, token{tString, s[j+1 : j+1+end]})
					i = j + 1 + end + len(tag)
				}
				continue
			}
			toks = append(toks, token{tPunct, "$"})
			i++
		case c >= '0' && c <= '9' || c == '.' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			j := i
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.' || s[j] == 'e' || s[j] == 'E' ||
				(s[j] == '+' || s[j] == '-') && j > i && (s[j-1] == 'e' || s[j-1] == 'E')) {
				j++
			}
			toks = append(toks, token{tNumber, s[i:j]})
			i = j
		case isIdentStart(c):
			j := i
			for j < len(s) && isIdentChar(s[j]) {
				j++
			}
			toks = append(toks, token{tIdent, strings.ToLower(s[i:j])})
			i = j
		case strings.IndexByte("(),;.[]", c) >= 0:
			toks = append(toks, token{tPunct, string(c)})
			i++
		case c == ':' && i+1 < len(s) && s[i+1] == ':':
			toks = append(toks, token{tPunct, "::"})
			i += 2
		case strings.IndexByte("+-*/<>=~!@#%^&|`?:", c) >= 0:
			j := i
			for j < len(s) && strings.IndexByte("+-*/<>=~!@#%^&|`?", s[j]) >= 0 {
				if j > i && s[j] == '-' && j+1 < len(s) && s[j+1] == '-' {
					break
				}
				j++
			}
			if j == i {
				j++
			}
			toks = append(toks, token{tPunct, s[i:j]})
			i = j
		default:
			toks = append(toks, token{tPunct, string(c)})
			i++
		}
	}
	return toks
}

// quotedString is the string quoted at i, and the index after it; escapes tells backslashes escape
func quotedString(s string, i int, escapes bool) (string, int) {
	var b strings.Builder
	j := i + 1
	for j < len(s) {
		switch {
		case escapes && s[j] == '\\' && j+1 < len(s):
			b.WriteByte(s[j+1])
			j += 2
			continue
		case s[j] == '\'':
			if j+1 < len(s) && s[j+1] == '\'' {
				b.WriteByte('\'')
				j += 2
				continue
			}
			return b.String(), j + 1
		}
		b.WriteByte(s[j])
		j++
	}
	return b.String(), j
}

func isIdentStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 0x80
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9' || c == '$'
}
