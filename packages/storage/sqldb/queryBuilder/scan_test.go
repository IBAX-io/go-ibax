/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package queryBuilder

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// The regular expressions the builder used, the references of hasNow and whereCols
var (
	checkNowRE = regexp.MustCompile(`(now\s*\(\s*\)|localtime|current_date|current_time|current_timestamp)`)
	whereColRE = regexp.MustCompile(`;([\w\-]+)`)
)

// scanParts expands the bytes of a fuzz input to parts of columns and conditions, so that the
// inputs hold the words looked for often
var scanParts = []string{"now", "NOW", "(", ")", " ", "\t", "\n", "\f", "\r", "\v", " ", "localtime",
	"current_", "date", "time", "stamp", "local", "current", "_", "x", ";", "->", "-", "a1", "Z", "ſ",
	"K", "İ", "LOCALTİME", "é", "\xff", "\xe2\x84", "'", ",", "now(", "�", "9"}

func expandScan(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		b.WriteString(scanParts[int(s[i])%len(scanParts)])
	}
	return b.String()
}

func checkScanners(t *testing.T, s string) {
	t.Helper()
	if got, want := hasNow(s), checkNowRE.MatchString(s); got != want {
		t.Fatalf("hasNow(%q) = %v, want %v", s, got, want)
	}
	wantErr := error(nil)
	if checkNowRE.Match([]byte(strings.ToLower(s))) {
		wantErr = ErrNow
	}
	if err := CheckNow(s); err != wantErr {
		t.Fatalf("CheckNow(%q) = %v, want %v", s, err, wantErr)
	}
	if got, want := whereCols(s), whereColRE.FindAllStringSubmatchIndex(s, -1); !reflect.DeepEqual(got, want) {
		t.Fatalf("whereCols(%q) = %v, want %v", s, got, want)
	}
}

func TestScanners(t *testing.T) {
	for _, s := range []string{
		"", "now", "now()", "now ()", "now ( )", "now\t(\n)", "now\v()", "now ()", "now(", "now)",
		"now(x)", "nownow()", "NOW()", "x now ( \r\f) y", "localtime", "localtim", "LocalTime",
		"LOCALTİME", "current_date", "current_time", "current_timestamp", "current_dat", "current_",
		"currentdate", "xcurrent_datex", "amount", "id", "name->x", "a;b", ";", ";;", ";a;b", "x;a-b;c_d",
		";-", "a;ſ", "a;é", ";\xff", "a;b;c->d", ";now()", "now(\xff)", "\xffnow()",
	} {
		checkScanners(t, s)
	}
}

func FuzzScanners(f *testing.F) {
	for _, s := range []string{"amount", "now()", "NOW ( )", "current_timestamp", "LOCALTIME", "x;a;b",
		"data->a->b", "", "\xff"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		checkScanners(t, s)
		checkScanners(t, expandScan(s))
	})
}

var benchColumns = []string{"id", "amount", "pub", "deleted", "blocked", "ecosystem", "account"}

func BenchmarkCheckNow(b *testing.B) {
	for b.Loop() {
		CheckNow(benchColumns...)
	}
}

func BenchmarkCheckNowRegexp(b *testing.B) {
	for b.Loop() {
		for _, item := range benchColumns {
			checkNowRE.Match([]byte(strings.ToLower(item)))
		}
	}
}

var benchWheres = []string{`"id"`, `"amount"`, `"data"->'a'`, `"ecosystem"`}

func BenchmarkWhereCols(b *testing.B) {
	for b.Loop() {
		for _, s := range benchWheres {
			whereCols(strings.Replace(s, "->", ";", -1))
		}
	}
}

func BenchmarkWhereColsRegexp(b *testing.B) {
	for b.Loop() {
		for _, s := range benchWheres {
			whereColRE.FindAllStringSubmatchIndex(strings.Replace(s, "->", ";", -1), -1)
		}
	}
}
