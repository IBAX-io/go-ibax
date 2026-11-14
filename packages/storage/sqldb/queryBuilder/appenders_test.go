/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package queryBuilder

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// oldBoundStatement is BoundStatement as it was before the appender, the reference of the tests
func oldBoundStatement(query string, args []any) string {
	if len(args) == 0 {
		return query
	}
	return query + " " + fmt.Sprintf("%q", args)
}

// oldGenerateRollBackInfoString is GenerateRollBackInfoString as it was before the appender, the
// reference of the tests
func (b SQLQueryBuilder) oldGenerateRollBackInfoString(logData map[string]string) (string, error) {
	rollbackInfo := make(map[string]string)
	for k, v := range logData {
		var eco bool
		idNames := strings.SplitN(b.Table, `_`, 2)
		if len(idNames) == 2 {
			b.keyName = idNames[1]
			if b.KeyTableChkr.IsKeyTable(b.keyName) {
				eco = true
			}
		}
		if k == `id` || (b.isKeyTable && !eco) {
			continue
		}
		rollbackInfo[k] = v
	}

	jsonRollbackInfo, err := json.Marshal(rollbackInfo)
	if err != nil {
		return "", err
	}

	return string(jsonRollbackInfo), nil
}

// appenderSeeds are the strings of the edges of the escapes of fmt %q and encoding/json: control
// characters, quotes, HTML characters, DEL, invalid UTF-8, the separators U+2028 and U+2029,
// runes past the basic plane, the replacement rune and unprintable runes
var appenderSeeds = []string{
	"", "a", "id", `"`, `\`, "<>&", "\x00\x01\x1f", "\b\f\n\r\t\v\a", "\x7f", "\x80", "\xff\xfe",
	"\xc3", "\xe2\x80", "\xe2\x80\xa8", "\xe2\x80\xa9", "\u2028\u2029", "\U0001F600", "\ufffd",
	"\u00a0\u00ad\u200b\ufeff", "\U000E0001", "日本語", "'", "`", "%q", "{}", "NULL", "\\x01",
	"a\x00b", "\xed\xa0\x80", "\xf4\x90\x80\x80", "timestamp 2020-01-01",
	`{"fuel_rate":"1000000","taxes_size":3,"token_symbol":"IBXC"}`,
	"\xb7\x2d\x68\x00\x9e\xe1\x80\x7f\xc2\xa0\xef\xbf\xbd\x1b\xf0\x9f\x98",
}

// splitArgs splits the input of a fuzz test into strings at the byte 0x1e
func splitArgs(s string) []any {
	var args []any
	for _, v := range strings.Split(s, "\x1e") {
		args = append(args, v)
	}
	return args
}

func checkBound(t *testing.T, query string, args []any) {
	t.Helper()
	want := oldBoundStatement(query, args)
	got := string(AppendBoundStatement([]byte("prefix"), query, args)[len("prefix"):])
	if got != want {
		t.Fatalf("query %q args %#v:\nwant %s\ngot  %s", query, args, want, got)
	}
}

func TestAppendBoundStatement(t *testing.T) {
	query := `UPDATE "1_keys" SET amount=amount+?::numeric WHERE "id" = ?`
	checkBound(t, query, nil)
	checkBound(t, query, []any{})
	for _, a := range appenderSeeds {
		checkBound(t, query, []any{a})
		for _, b := range appenderSeeds {
			checkBound(t, query, []any{a, b})
		}
	}
	// values of other types than string are written by fmt, as before
	checkBound(t, query, []any{"1", int64(2)})
	checkBound(t, query, []any{nil, "x"})
	checkBound(t, query, []any{[]byte("ab"), 1.5, true})
}

// TestAppendQuoteRunes compares appendQuote with strconv.AppendQuote on each byte alone, on each
// rune, valid or not, between plain text, and on each byte before and after the bytes of a rune
func TestAppendQuoteRunes(t *testing.T) {
	check := func(s string) {
		if got, want := string(appendQuote(nil, s)), strconv.Quote(s); got != want {
			t.Fatalf("%x: want %s got %s", s, want, got)
		}
	}
	for c := 0; c < 256; c++ {
		check(string([]byte{byte(c)}))
		check("a" + string([]byte{byte(c)}) + "b")
		for d := 0; d < 256; d++ {
			check(string([]byte{byte(c), byte(d)}))
		}
	}
	var buf []byte
	for r := rune(0); r <= utf8.MaxRune; r++ {
		buf = utf8.AppendRune(buf[:0], r)
		if r >= 0xd800 && r < 0xe000 {
			// utf8 encodes a surrogate as U+FFFD; write the bytes of the surrogate itself
			buf = append(buf[:0], 0xed, byte(0x80|(r>>6)&0x3f), byte(0x80|r&0x3f))
		}
		check(string(buf))
		check("x" + string(buf) + "\"")
		check(string(buf[:len(buf)-1]) + "y")
	}
}

func checkRollBack(t *testing.T, b *SQLQueryBuilder, logData map[string]string) {
	t.Helper()
	want, err := b.oldGenerateRollBackInfoString(logData)
	if err != nil {
		t.Fatal(err)
	}
	got, keys := b.AppendRollBackInfo([]byte("prefix"), make([]string, 3), logData)
	if string(got[len("prefix"):]) != want {
		t.Fatalf("table %s key table %v data %#v:\nwant %s\ngot  %s", b.Table, b.isKeyTable, logData, want, got[len("prefix"):])
	}
	if len(keys) > len(logData) {
		t.Fatalf("keys %v of %d columns", keys, len(logData))
	}
}

// rollbackBuilders are the builders of the cases of GenerateRollBackInfoString: a table which is
// not a key table, a key table, and a key table whose builder was not prepared
func rollbackBuilders() []*SQLQueryBuilder {
	return []*SQLQueryBuilder{
		{Table: "1_history", KeyTableChkr: TestKeyTableChecker{false}},
		{Table: "1_keys", isKeyTable: true, KeyTableChkr: TestKeyTableChecker{true}},
		{Table: "1_keys", KeyTableChkr: TestKeyTableChecker{true}},
		{Table: "keys", isKeyTable: true, KeyTableChkr: TestKeyTableChecker{true}},
		{Table: "2_b7_x", isKeyTable: true, KeyTableChkr: TestKeyTableChecker{false}},
	}
}

func TestAppendRollBackInfo(t *testing.T) {
	for _, b := range rollbackBuilders() {
		checkRollBack(t, b, nil)
		checkRollBack(t, b, map[string]string{"id": "1"})
		for _, a := range appenderSeeds {
			checkRollBack(t, b, map[string]string{a: a, "id": a, "amount": a + "x"})
		}
		data := map[string]string{}
		for i, a := range appenderSeeds {
			data[a] = appenderSeeds[len(appenderSeeds)-1-i]
		}
		checkRollBack(t, b, data)
	}
}

// FuzzDataHash compares the text the DataHash of a statement is the hash of with BoundStatement
// as it was: the query and the values, strings split at the byte 0x1e
func FuzzDataHash(f *testing.F) {
	for _, s := range appenderSeeds {
		f.Add(`UPDATE "1_keys" SET "amount"=? WHERE "id" = ?`, s+"\x1e"+s)
	}
	f.Add(`INSERT INTO "1_history" (a) VALUES (?)`, strings.Join(appenderSeeds, "\x1e"))
	f.Fuzz(func(t *testing.T, query, values string) {
		checkBound(t, query, splitArgs(values))
		if values == "" {
			checkBound(t, query, nil)
		}
	})
}

// FuzzRollbackJSON compares the data of a rollback record with json.Marshal of the map as before:
// the keys and values are the strings of the input split at the byte 0x1e, in pairs
func FuzzRollbackJSON(f *testing.F) {
	for _, s := range appenderSeeds {
		f.Add(s + "\x1e" + s + "\x1eid\x1e" + s)
	}
	f.Add(strings.Join(appenderSeeds, "\x1e"))
	builders := rollbackBuilders()
	f.Fuzz(func(t *testing.T, in string) {
		parts := strings.Split(in, "\x1e")
		data := make(map[string]string, len(parts)/2)
		for i := 0; i+1 < len(parts); i += 2 {
			data[parts[i]] = parts[i+1]
		}
		for _, b := range builders {
			checkRollBack(t, b, data)
		}
	})
}
