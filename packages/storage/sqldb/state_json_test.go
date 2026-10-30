/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"encoding/json"
	"io"
	"math/rand/v2"
	"strings"
	"testing"
)

// loneSurrogate tells a text has an escape of a UTF-16 surrogate, which the decoder of Go takes
// for U+FFFD where PostgreSQL rejects a lone one: the scanner rejects all of them
func loneSurrogate(s string) bool {
	l := strings.ToLower(s)
	for i := strings.Index(l, `\ud`); i >= 0; i = strings.Index(l, `\ud`) {
		if i+3 < len(l) && l[i+3] >= '8' && l[i+3] <= 'f' {
			return true
		}
		l = l[i+3:]
	}
	return false
}

// decodeJSON is readJSON as it read through encoding/json, the oracle of the scanner
func decodeJSON(s string) (any, bool) {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	v, ok := decodeValue(d)
	if !ok {
		return nil, false
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, false
	}
	return v, true
}

func decodeValue(d *json.Decoder) (any, bool) {
	t, err := d.Token()
	if err != nil {
		return nil, false
	}
	switch x := t.(type) {
	case json.Delim:
		switch x {
		case '{':
			o := &jsonObject{vals: make(map[string]any)}
			for d.More() {
				kt, err := d.Token()
				if err != nil {
					return nil, false
				}
				k, ok := kt.(string)
				if !ok || strings.IndexByte(k, 0) >= 0 {
					return nil, false
				}
				v, ok := decodeValue(d)
				if !ok {
					return nil, false
				}
				if _, seen := o.vals[k]; !seen {
					o.keys = append(o.keys, k)
				}
				o.vals[k] = v
			}
			if _, err := d.Token(); err != nil {
				return nil, false
			}
			return o, true
		case '[':
			a := []any{}
			for d.More() {
				v, ok := decodeValue(d)
				if !ok {
					return nil, false
				}
				a = append(a, v)
			}
			if _, err := d.Token(); err != nil {
				return nil, false
			}
			return a, true
		}
		return nil, false
	case string:
		if strings.IndexByte(x, 0) >= 0 {
			return nil, false
		}
		return x, true
	case json.Number, bool, nil:
		return x, true
	}
	return nil, false
}

// checkJSON fails when the scanner takes a text the oracle rejects or reads it otherwise. The
// scanner may reject more: those texts go to PostgreSQL.
func checkJSON(t *testing.T, s string) bool {
	t.Helper()
	if loneSurrogate(s) {
		return false
	}
	got, ok := readJSON(s)
	want, wantOK := decodeJSON(s)
	if !ok {
		if wantOK && strings.Count(s, `\u0000`) == 0 && validUTF8Strings(s) {
			t.Errorf("scanner rejects %q, which encoding/json reads", s)
		}
		return false
	}
	if !wantOK {
		t.Errorf("scanner reads %q, which encoding/json rejects", s)
		return false
	}
	var a, b strings.Builder
	if writeJSONB(&a, got) != writeJSONB(&b, want) || a.String() != b.String() {
		t.Errorf("%q: scanner %s, encoding/json %s", s, a.String(), b.String())
	}
	return true
}

// validUTF8Strings is false for texts with bytes that are not UTF-8, which the scanner rejects
// where encoding/json takes them for U+FFFD
func validUTF8Strings(s string) bool { return strings.ToValidUTF8(s, "�") == s }

var jsonCases = []string{
	`{}`, `[]`, `""`, `0`, `-0`, `1`, `-1.5e+10`, `1E-3`, `0.0`, `123456789012345678901234567890`,
	`true`, `false`, `null`, ` { "a" : 1 , "b" : [ 1 , 2 ] } `, "\t\n\r[1]\n",
	`{"a":1,"a":2}`, `{"b":1,"a":2,"b":3}`, `{"aa":1,"b":2,"":3}`, `[[[[]]]]`, `[{}, {"x": []}]`,
	`"\"\\\/\b\f\n\r\t"`, `"Aé中￿"`, `"é中😀"`, `"\u0000"`, `{"\u0000":1}`,
	`"a\u0000b"`, `"😀"`, `"\udc00"`,
	// rejected
	``, ` `, `{`, `}`, `[`, `]`, `[1,]`, `{"a":1,}`, `{,}`, `[,1]`, `{"a"}`, `{"a":}`, `{a:1}`,
	`01`, `-`, `-01`, `1.`, `.5`, `1e`, `1e+`, `+1`, `0x10`, `NaN`, `Infinity`, `tru`, `nul`,
	`truex`, `[true false]`, `1 2`, `"a" "b"`, `"abc`, `"\x"`, `"\u12"`, `"\u12g4"`, `"\'"`,
	"\"a\x01b\"", "\"a\tb\"", "\"\xff\"", "\"\xc3\"", "\xef\xbb\xbf{}", `{"a":1}x`, `[1]]`,
	`{"a":1}}`, `'a'`, "[1, 2]", `[1e400]`, `[-0.0e-0]`,
}

func TestJSONScanner(t *testing.T) {
	for _, s := range jsonCases {
		checkJSON(t, s)
	}
	for _, n := range []int{jsonMaxDepth, jsonMaxDepth + 1} {
		checkJSON(t, strings.Repeat("[", n)+strings.Repeat("]", n))
	}
	r := rand.New(rand.NewPCG(3, 4))
	read := 0
	for range 200000 {
		if checkJSON(t, randomJSON(r)) {
			read++
		}
		if t.Failed() {
			return
		}
	}
	if read < 50000 {
		t.Fatalf("only %d random texts were JSON", read)
	}
}

// randomJSON is a JSON value, broken now and then by a byte changed, cut or added
func randomJSON(r *rand.Rand) string {
	var b strings.Builder
	writeRandomJSON(r, &b, 0)
	s := b.String()
	switch r.IntN(4) {
	case 0:
		if len(s) > 0 {
			i := r.IntN(len(s))
			s = s[:i] + string(jsonBytes[r.IntN(len(jsonBytes))]) + s[i+1:]
		}
	case 1:
		s = s[:r.IntN(len(s)+1)]
	case 2:
		i := r.IntN(len(s) + 1)
		s = s[:i] + string(jsonBytes[r.IntN(len(jsonBytes))]) + s[i:]
	}
	return s
}

const jsonBytes = "{}[],:\"\\ \t\n0123456789-+.eEtrufalsn/bu\x00\x01\x7f\xc3\xa9\xffx"

func writeRandomJSON(r *rand.Rand, b *strings.Builder, depth int) {
	ws := func() {
		if r.IntN(4) == 0 {
			b.WriteString([]string{" ", "\t", "\n", "\r", "  "}[r.IntN(5)])
		}
	}
	ws()
	n := r.IntN(10)
	if depth > 4 {
		n = 4 + r.IntN(6)
	}
	switch n {
	case 0, 1:
		b.WriteByte('{')
		for i := range r.IntN(4) {
			if i > 0 {
				b.WriteByte(',')
			}
			ws()
			writeRandomString(r, b)
			ws()
			b.WriteByte(':')
			writeRandomJSON(r, b, depth+1)
		}
		ws()
		b.WriteByte('}')
	case 2, 3:
		b.WriteByte('[')
		for i := range r.IntN(4) {
			if i > 0 {
				b.WriteByte(',')
			}
			writeRandomJSON(r, b, depth+1)
		}
		ws()
		b.WriteByte(']')
	case 4, 5:
		writeRandomString(r, b)
	case 6, 7:
		b.WriteString([]string{"0", "-0", "7", "-12", "3.25", "0.000", "1e5", "-2E-3", "1.5e+300",
			"99999999999999999999", "1e-400", "100"}[r.IntN(12)])
	default:
		b.WriteString([]string{"true", "false", "null"}[r.IntN(3)])
	}
	ws()
}

func writeRandomString(r *rand.Rand, b *strings.Builder) {
	parts := []string{"a", "b", "ab", "é", "中", "😀", `\"`, `\\`, `\/`, `\b`, `\f`, `\n`, `\r`,
		`\t`, `A`, `é`, `中`, `\u001f`, `\u007f`, " ", "<", "&"}
	b.WriteByte('"')
	for range r.IntN(5) {
		b.WriteString(parts[r.IntN(len(parts))])
	}
	b.WriteByte('"')
}

// TestConcatJSONB checks a || b against reading both and writing them merged by the oracle
func TestConcatJSONB(t *testing.T) {
	for _, c := range [][3]string{
		{`{}`, `{}`, `{}`},
		{`{"a": 1}`, `{"b": 2}`, `{"a": 1, "b": 2}`},
		{`{"a": 1, "b": 2}`, `{"a": [3]}`, `{"a": [3], "b": 2}`},
		{`{"bb": {"x": 1}}`, `{"a": null}`, `{"a": null, "bb": {"x": 1}}`},
	} {
		got, ok := concatJSONB(c[0], c[1])
		if !ok || got != c[2] {
			t.Errorf("%s || %s = %s %v, want %s", c[0], c[1], got, ok, c[2])
		}
	}
	for _, c := range [][2]string{{`[]`, `{}`}, {`{}`, `1`}, {`{`, `{}`}} {
		if _, ok := concatJSONB(c[0], c[1]); ok {
			t.Errorf("%s || %s is read", c[0], c[1])
		}
	}
}

func FuzzJSONScanner(f *testing.F) {
	for _, s := range jsonCases {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { checkJSON(t, s) })
}

func BenchmarkCanonJSONB(b *testing.B) {
	s := `{"type": "transfer", "amount": "1000000000000000000", "fees": {"vm": "123.5", "storage": 0, ` +
		`"element": [1, 2, 3]}, "comment": "payment é for order #42", "ok": true, "to": null}`
	b.ReportAllocs()
	for b.Loop() {
		canonJSONB(s)
	}
}

func BenchmarkDecodeJSONOracle(b *testing.B) {
	s := `{"type": "transfer", "amount": "1000000000000000000", "fees": {"vm": "123.5", "storage": 0, ` +
		`"element": [1, 2, 3]}, "comment": "payment é for order #42", "ok": true, "to": null}`
	b.ReportAllocs()
	for b.Loop() {
		decodeJSON(s)
	}
}

func BenchmarkReadJSON(b *testing.B) {
	s := `{"type": "transfer", "amount": "1000000000000000000", "fees": {"vm": "123.5", "storage": 0, ` +
		`"element": [1, 2, 3]}, "comment": "payment é for order #42", "ok": true, "to": null}`
	b.ReportAllocs()
	for b.Loop() {
		readJSON(s)
	}
}

// TestJSONNumberAsItStands checks the numbers writeJSONB writes as they stand are those numeric
// outputs so
func TestJSONNumberAsItStands(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	digits := func(n int) string {
		var b strings.Builder
		for range n {
			b.WriteByte(byte('0' + r.IntN(10)))
		}
		return b.String()
	}
	for range 100000 {
		s := "0"
		if r.IntN(3) > 0 {
			s = string(byte('1'+r.IntN(9))) + digits(r.IntN(30))
		}
		if r.IntN(2) == 0 {
			s += "." + digits(1+r.IntN(30))
		}
		if r.IntN(2) == 0 {
			s = "-" + s
		}
		if r.IntN(4) == 0 {
			s += []string{"e", "E", "e+", "e-"}[r.IntN(4)] + digits(1+r.IntN(3))
		}
		var b strings.Builder
		if !writeJSONB(&b, json.Number(s)) {
			if n, ok := parseNumeric(s); ok {
				t.Fatalf("%s is not written, numeric reads %s", s, n)
			}
			continue
		}
		n, ok := parseNumeric(s)
		if !ok || b.String() != n.String() {
			t.Fatalf("%s is written %s, numeric outputs %s %v", s, b.String(), n, ok)
		}
	}
}
