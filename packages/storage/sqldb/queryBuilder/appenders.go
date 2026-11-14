/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package queryBuilder

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// AppendBoundStatement appends to dst a statement together with the values bound to its
// placeholders, the form in which a statement is hashed: the query, and when there are values a
// space and the values as fmt writes a []any with the verb %q. The values the query builder binds
// are strings, which %q writes as strconv.AppendQuote does; a value of another type is written by
// fmt itself.
func AppendBoundStatement(dst []byte, query string, args []any) []byte {
	dst = append(dst, query...)
	if len(args) == 0 {
		return dst
	}
	for _, arg := range args {
		if _, ok := arg.(string); !ok {
			return fmt.Appendf(dst, " %q", args)
		}
	}
	from := len(dst)
	dst = append(dst, ' ', '[')
	for i, arg := range args {
		if i > 0 {
			dst = append(dst, ' ')
		}
		dst = appendQuote(dst, arg.(string))
	}
	dst = append(dst, ']')
	if appendCheck {
		if want := fmt.Sprintf(" %q", args); string(dst[from:]) != want {
			panic(fmt.Sprintf("b7check: the values %#v are bound as %s, not %s", args, dst[from:], want))
		}
	}
	return dst
}

// appendQuote appends to dst the string s as strconv.AppendQuote does. A byte of ASCII is a rune
// of its own: printable ASCII but the quote and the backslash as it is, the quote and the backslash
// after a backslash, the controls of a short escape as that escape, and the other controls and DEL
// as \x and two hex digits. Of the other bytes, a byte that does not start a valid UTF-8 sequence
// is \x and its two hex digits, a printable rune is its bytes, and a rune that is not printable
// takes strconv.AppendQuote itself.
func appendQuote(dst []byte, s string) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && plainByte(s[j]) {
			j++
		}
		dst = append(dst, s[i:j]...)
		if j == len(s) {
			break
		}
		c := s[j]
		if c < utf8.RuneSelf {
			i = j + 1
			switch c {
			case '"', '\\':
				dst = append(dst, '\\', c)
			case '\a':
				dst = append(dst, '\\', 'a')
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			case '\v':
				dst = append(dst, '\\', 'v')
			default:
				dst = append(dst, '\\', 'x', lowerHex[c>>4], lowerHex[c&0xf])
			}
			continue
		}
		r, w := utf8.DecodeRuneInString(s[j:])
		i = j + w
		switch {
		case r == utf8.RuneError && w == 1:
			dst = append(dst, '\\', 'x', lowerHex[c>>4], lowerHex[c&0xf])
		case strconv.IsPrint(r):
			dst = append(dst, s[j:i]...)
		default:
			n := len(dst)
			dst = strconv.AppendQuote(dst, s[j:i])
			dst = append(dst[:n], dst[n+1:len(dst)-1]...)
		}
	}
	return append(dst, '"')
}

// lowerHex are the digits strconv writes the \x escapes with
const lowerHex = "0123456789abcdef"

// plainByte tells strconv.AppendQuote writes the byte as it is: printable ASCII but the quote and
// the backslash
func plainByte(c byte) bool {
	return c >= 0x20 && c < 0x7f && c != '"' && c != '\\'
}

// AppendRollBackInfo appends to dst the data of the rollback record of a row an update changes:
// the columns of the row but its id, as encoding/json writes the map of them, with the keys in
// order and the strings escaped as json.Marshal escapes them. keys is a scratch slice of the
// caller, which it returns to be used again.
func (b *SQLQueryBuilder) AppendRollBackInfo(dst []byte, keys []string, logData map[string]string) ([]byte, []string) {
	var eco bool
	idNames := strings.SplitN(b.Table, `_`, 2)
	if len(idNames) == 2 && b.KeyTableChkr.IsKeyTable(idNames[1]) {
		eco = true
	}
	keys = keys[:0]
	if !b.isKeyTable || eco {
		for k := range logData {
			if k != `id` {
				keys = append(keys, k)
			}
		}
	}
	slices.Sort(keys)
	dst = append(dst, '{')
	for i, k := range keys {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = AppendJSONString(dst, k)
		dst = append(dst, ':')
		dst = AppendJSONString(dst, logData[k])
	}
	return append(dst, '}'), keys
}

// jsonHex are the digits of the \u escapes of encoding/json
const jsonHex = "0123456789abcdef"

// AppendJSONString appends to dst the string s as encoding/json writes a string with HTML
// escaping, the escaping of json.Marshal: \" and \\, the short escapes \b \f \n \r \t, \u00XX for
// the other control characters and for < > &, � for a byte of invalid UTF-8, and   and
//   for the line and paragraph separators.
func AppendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		if c := s[i]; c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch c {
			case '\\', '"':
				dst = append(dst, '\\', c)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', jsonHex[c>>4], jsonHex[c&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, `�`...)
			i += size
			start = i
			continue
		}
		if r == ' ' || r == ' ' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', jsonHex[r&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}
