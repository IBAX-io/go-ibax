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
	dst = append(dst, ' ', '[')
	for i, arg := range args {
		if i > 0 {
			dst = append(dst, ' ')
		}
		dst = strconv.AppendQuote(dst, arg.(string))
	}
	return append(dst, ']')
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
