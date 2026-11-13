/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"math/big"
	"strconv"
	"strings"
	"testing"
)

// fastTypes are the types of the columns the fast paths of inputText serve
var fastTypes = []colType{
	newColType(21, "int2", -1), newColType(23, "int4", -1), newColType(20, "int8", -1),
	newColType(1700, "numeric", -1),
	newColType(1700, "numeric", 30<<16|0+4), newColType(1700, "numeric", 10<<16|2+4),
	newColType(1700, "numeric", 1<<16|0+4), newColType(1700, "numeric", 5<<16|5+4),
	newColType(1700, "numeric", 40<<16|18+4), newColType(1700, "numeric", 3<<16|1+4),
	newColType(3802, "jsonb", -1),
}

// checkFast checks each fast path with the texts a and b against the full function it stands for
func checkFast(t *testing.T, a, b string) {
	for _, ct := range fastTypes {
		want, wantOK := ct.inputFull(a)
		got, ok := ct.fastInput(a)
		if ok && (!wantOK || want != got) {
			t.Fatalf("%s %q: fastInput %q, inputFull %q %v", ct.name, a, got, want, wantOK)
		}
		in, inOK := ct.inputText(a)
		if inOK != wantOK || wantOK && *in != want {
			t.Fatalf("%s %q: inputText %v %v, inputFull %q %v", ct.name, a, deref(in), inOK, want, wantOK)
		}
		// a value bound to the type converts the text as inputText does, and keeps its text when
		// it stays
		src := tval{t: typeUnknown, s: &a}
		cv, cvOK := convert(src, ct)
		if cvOK != wantOK || wantOK && (*cv.s != want || (cv.s == &a) != (want == a)) {
			t.Fatalf("%s %q: convert %v %v, inputFull %q %v", ct.name, a, deref(cv.s), cvOK, want, wantOK)
		}
		if ct.kind == kNumeric || ct.kind == kInt {
			n, ok := parseNumeric(a)
			wantKey := a
			if ct.kind == kNumeric && ok {
				wantKey = n.normal()
			}
			if got := ct.key(a); got != wantKey {
				t.Fatalf("%s key %q: %q, want %q", ct.name, a, got, wantKey)
			}
		}
	}
	if canonInt(a) {
		n, ok := parseNumeric(a)
		if !ok || n.scale != 0 || n.String() != a || n.normal() != a {
			t.Fatalf("canonInt %q: parseNumeric %v %v", a, n.v, ok)
		}
		if canonInt(b) {
			m, _ := parseNumeric(b)
			if got, want := compareCanonInt(a, b), n.add(m.neg()).sign(); got != want {
				t.Fatalf("compareCanonInt %q %q = %d, want %d", a, b, got, want)
			}
		}
	}
	for _, sub := range []bool{false, true} {
		if got, ok := addInt64(a, b, sub); ok {
			p, _ := parseNumeric(a)
			q, _ := parseNumeric(b)
			if sub {
				q = q.neg()
			}
			if want := p.add(q).String(); got != want {
				t.Fatalf("addInt64 %q %q %v = %q, want %q", a, b, sub, got, want)
			}
		}
	}
	p, err1 := strconv.ParseInt(a, 10, 64)
	q, err2 := strconv.ParseInt(b, 10, 64)
	if err1 == nil && err2 == nil {
		sum := new(big.Int).Add(big.NewInt(p), big.NewInt(q))
		got, ok := addInt64Checked(p, q)
		if ok != sum.IsInt64() || ok && got != sum.Int64() {
			t.Fatalf("addInt64Checked %d %d = %d %v, want %s", p, q, got, ok, sum)
		}
	}
	want, wantOK := canonJSONB(a)
	if got, ok := fastJSONB(a); ok && (!wantOK || got != want) {
		t.Fatalf("fastJSONB %q = %q, canonJSONB %q %v", a, got, want, wantOK)
	}
	checkKeyOf(t, a, b)
}

// keyMeta is a table of a column of each type of fastTypes and of text
var keyMeta = func() *tableMeta {
	m := &tableMeta{byName: map[string]*colMeta{}}
	for i, ct := range append(fastTypes, newColType(25, "text", -1)) {
		name := strconv.Itoa(i)
		m.byName[name] = &colMeta{name: name, typ: ct}
	}
	return m
}()

// keyOfBuilder is keyOf as a strings.Builder wrote the key
func keyOfBuilder(m *tableMeta, cols []string, val func(col string) *string) (string, bool) {
	var b strings.Builder
	for i, c := range cols {
		v := val(c)
		if v == nil {
			return "", false
		}
		if i > 0 {
			b.WriteByte(0)
		}
		b.WriteString(m.byName[c].typ.key(*v))
	}
	return b.String(), true
}

// checkKeyOf checks keyOf of keys of up to six columns of each type, with the values a and b and
// NULL, against keyOfBuilder
func checkKeyOf(t *testing.T, a, b string) {
	n := len(keyMeta.byName)
	vals := []*string{&a, &b, nil}
	for k := 0; k <= 6; k++ {
		cols := make([]string, k)
		for i := range cols {
			cols[i] = strconv.Itoa((len(a) + i*7 + len(b)*3) % n)
		}
		for shift := range 3 {
			val := func(col string) *string {
				i, _ := strconv.Atoi(col)
				return vals[(i+shift)%3]
			}
			if k > 0 && shift == 2 {
				val = func(string) *string { return &a }
			}
			want, wantOK := keyOfBuilder(keyMeta, cols, val)
			got, ok := keyMeta.keyOf(cols, val)
			if got != want || ok != wantOK {
				t.Fatalf("keyOf %v %q %q: %q %v, want %q %v", cols, a, b, got, ok, want, wantOK)
			}
		}
	}
}

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// fastSeeds are texts of the values the fast paths serve and of those near them
var fastSeeds = []string{
	"0", "-0", "1", "-1", "00", "01", "+1", " 1", "1 ", "1.0", "1e3", "10", "-10", "99999", "100000",
	"32767", "32768", "-32768", "-32769", "2147483647", "2147483648", "9223372036854775807",
	"9223372036854775808", "-9223372036854775808", "-9223372036854775809", "999999999999999999",
	"1000000000000000000", "-999999999999999999", "123456789012345678901234567890",
	"1234567890123456789012345678901234567890123", "999999029000", "-8800004", "",
	`{}`, `[]`, `null`, `true`, `false`, `1`, `-0`, `-0.5`, `0.50`, `1e2`, `"s"`, `{"b":1,"a":2}`,
	`{"a":1,"a":2}`, `{"a":1,"b":{"a":2,"a":3}}`, `{"k":"\n"}`, `{"aa":1,"b":2,"a":[1,2,{"z":null}]}`,
	` { "a" : [ 1 , 2 ] } `, `{"long key":1,"short":2,"s":3}`, `[1,[2,[3,{"x":[]}]]]`, `{"a": 01}`,
	`{"expedite_fee":{"arithmetic":"NATIVE","conversion_rate":100,"decimal":"0","flag":1,"value":"0"},"fuel_rate":"1000000"}`,
	`{"fuel_rate": "1000000", "taxes_size": 3, "vmCost_fee": {"flag": 1, "value": "123456789"}}`,
	"{\"a\":\"\xff\"}", `"é"`, `{"é":1,"e":2,"ee":3}`, `[1,]`, `{"a":1,}`, `{"a" 1}`, `tru`, `nul`,
}

func TestFastPaths(t *testing.T) {
	for _, a := range fastSeeds {
		for _, b := range fastSeeds {
			checkFast(t, a, b)
		}
	}
}

// FuzzFastPaths checks the fast paths of inputText, key, the conditions, eval and the checks of
// rows against the full functions
func FuzzFastPaths(f *testing.F) {
	for i, a := range fastSeeds {
		f.Add(a, fastSeeds[(i*7+3)%len(fastSeeds)])
	}
	for _, in := range typeInputs {
		for _, a := range in {
			f.Add(a, "1")
		}
	}
	f.Fuzz(checkFast)
}
