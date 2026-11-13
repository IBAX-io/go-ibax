/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// The values the state reads are mostly integers in their canonical text and JSON objects of a few
// levels. The functions here make the text PostgreSQL outputs for those without math/big and
// without the values of JSON: each tells when the value is not one it knows, and the full
// functions make the text then. Each makes the text the full function makes.

// canonInt tells the text is an integer as PostgreSQL outputs it: -?(0|[1-9][0-9]*) but -0
func canonInt(s string) bool {
	d := s
	if strings.HasPrefix(d, "-") {
		d = d[1:]
	}
	if d == "" || d[0] == '0' && (len(d) > 1 || len(s) > 1) {
		return false
	}
	for i := 0; i < len(d); i++ {
		if d[i] < '0' || d[i] > '9' {
			return false
		}
	}
	return true
}

// compareCanonInt compares two texts canonInt tells are integers
func compareCanonInt(a, b string) int {
	na, nb := strings.HasPrefix(a, "-"), strings.HasPrefix(b, "-")
	switch {
	case na && !nb:
		return -1
	case !na && nb:
		return 1
	}
	c := len(a) - len(b)
	if c == 0 {
		c = strings.Compare(a, b)
	}
	if c > 0 {
		c = 1
	} else if c < 0 {
		c = -1
	}
	if na {
		return -c
	}
	return c
}

// fastInput is inputText of an integer in its canonical text for a column of an integer or numeric
// type, and of jsonb; false when it is another value, which inputText reads in full
func (t colType) fastInput(s string) (string, bool) {
	switch t.kind {
	case kInt:
		if !canonInt(s) {
			return "", false
		}
		if _, err := strconv.ParseInt(s, 10, t.bits); err != nil {
			return "", false
		}
		return s, true
	case kNumeric:
		if !canonInt(s) {
			return "", false
		}
		return numericTypmod(s, t.precision, t.scale)
	case kJSONB:
		return fastJSONB(s)
	}
	return "", false
}

// numericTypmod is the text a column numeric(precision, scale) stores of the integer of the
// canonical text s; false when it overflows
func numericTypmod(s string, precision, scale int) (string, bool) {
	if precision < 0 {
		return s, true
	}
	digits := len(s)
	if s[0] == '-' {
		digits--
	}
	if s != "0" && digits > precision-scale {
		return "", false
	}
	if scale <= 0 {
		return s, true
	}
	b := make([]byte, len(s)+1+scale)
	n := copy(b, s)
	b[n] = '.'
	for i := n + 1; i < len(b); i++ {
		b[i] = '0'
	}
	return string(b), true
}

// numericKey is n.normal() of the numeric text s: s itself for an integer in its canonical text
func numericKey(s string) (string, bool) {
	if canonInt(s) {
		return s, true
	}
	n, ok := parseNumeric(s)
	if !ok {
		return "", false
	}
	return n.normal(), true
}

// addInt64 is the text of p+q of the integers in their canonical text a and b, -q when sub; false
// when the sum is not an int64 or a value is not one of 18 digits at most
func addInt64(a, b string, sub bool) (string, bool) {
	if len(a) > 19 || len(b) > 19 || !canonInt(a) || !canonInt(b) {
		return "", false
	}
	p, err1 := strconv.ParseInt(a, 10, 64)
	q, err2 := strconv.ParseInt(b, 10, 64)
	if err1 != nil || err2 != nil || p > 999999999999999999 || p < -999999999999999999 ||
		q > 999999999999999999 || q < -999999999999999999 {
		return "", false
	}
	if sub {
		q = -q
	}
	return strconv.FormatInt(p+q, 10), true
}

// addInt64Checked is the sum of p and q; false when it is not an int64
func addInt64Checked(p, q int64) (int64, bool) {
	if q > 0 && p > math.MaxInt64-q || q < 0 && p < math.MinInt64-q {
		return 0, false
	}
	return p + q, true
}

// fastJSONB is canonJSONB of a text whose strings have no escapes and whose numbers have no
// exponent nor -0 in front: the members of each object are sorted, and their keys differ; false
// for other texts, which canonJSONB reads in full
func fastJSONB(s string) (string, bool) {
	if !utf8.ValidString(s) {
		return "", false
	}
	w := jsonbWriters.Get().(*jsonbWriter)
	defer jsonbWriters.Put(w)
	w.s, w.i, w.out, w.spans = s, 0, w.out[:0], w.spans[:0]
	w.ws()
	if !w.value(0) {
		return "", false
	}
	w.ws()
	if w.i != len(s) {
		return "", false
	}
	if string(w.out) == s {
		return s, true
	}
	return string(w.out), true
}

// jsonbFastDepth bounds the nesting fastJSONB follows
const jsonbFastDepth = 64

var jsonbWriters = sync.Pool{New: func() any { return new(jsonbWriter) }}

// jsonbWriter writes the text of jsonb of the text s at i to out
type jsonbWriter struct {
	s     string
	i     int
	out   []byte
	tmp   []byte
	spans []jsonbMember
}

// jsonbMember is a member of an object: its key in the text read and its value in out
type jsonbMember struct {
	key        string
	start, end int
}

func (w *jsonbWriter) ws() {
	for w.i < len(w.s) {
		switch w.s[w.i] {
		case ' ', '\t', '\n', '\r':
			w.i++
		default:
			return
		}
	}
}

// str is the text of a string at its opening quote that has no escape
func (w *jsonbWriter) str() (string, bool) {
	for j := w.i + 1; j < len(w.s); j++ {
		switch c := w.s[j]; {
		case c == '"':
			v := w.s[w.i+1 : j]
			w.i = j + 1
			return v, true
		case c == '\\' || c < 0x20:
			return "", false
		}
	}
	return "", false
}

func (w *jsonbWriter) value(depth int) bool {
	if w.i >= len(w.s) || depth >= jsonbFastDepth {
		return false
	}
	switch c := w.s[w.i]; {
	case c == '{':
		return w.object(depth)
	case c == '[':
		w.i++
		w.out = append(w.out, '[')
		w.ws()
		if w.i < len(w.s) && w.s[w.i] == ']' {
			w.i++
			w.out = append(w.out, ']')
			return true
		}
		for {
			if !w.value(depth + 1) {
				return false
			}
			w.ws()
			if w.i >= len(w.s) {
				return false
			}
			switch w.s[w.i] {
			case ',':
				w.i++
				w.ws()
				w.out = append(w.out, ", "...)
			case ']':
				w.i++
				w.out = append(w.out, ']')
				return true
			default:
				return false
			}
		}
	case c == '"':
		v, ok := w.str()
		if !ok {
			return false
		}
		w.out = append(append(append(w.out, '"'), v...), '"')
		return true
	case c == '-' || c >= '0' && c <= '9':
		return w.number()
	case strings.HasPrefix(w.s[w.i:], "true"):
		w.i += 4
		w.out = append(w.out, "true"...)
		return true
	case strings.HasPrefix(w.s[w.i:], "false"):
		w.i += 5
		w.out = append(w.out, "false"...)
		return true
	case strings.HasPrefix(w.s[w.i:], "null"):
		w.i += 4
		w.out = append(w.out, "null"...)
		return true
	}
	return false
}

// object writes the members of an object by the length of their keys and then by their bytes
func (w *jsonbWriter) object(depth int) bool {
	w.i++
	w.out = append(w.out, '{')
	w.ws()
	if w.i < len(w.s) && w.s[w.i] == '}' {
		w.i++
		w.out = append(w.out, '}')
		return true
	}
	from, first := len(w.spans), len(w.out)
	for {
		if w.i >= len(w.s) || w.s[w.i] != '"' {
			return false
		}
		k, ok := w.str()
		if !ok {
			return false
		}
		w.ws()
		if w.i >= len(w.s) || w.s[w.i] != ':' {
			return false
		}
		w.i++
		w.ws()
		start := len(w.out)
		if !w.value(depth + 1) {
			return false
		}
		w.spans = append(w.spans, jsonbMember{key: k, start: start, end: len(w.out)})
		w.ws()
		if w.i >= len(w.s) {
			return false
		}
		if w.s[w.i] == '}' {
			w.i++
			break
		}
		if w.s[w.i] != ',' {
			return false
		}
		w.i++
		w.ws()
	}
	members := w.spans[from:]
	slices.SortStableFunc(members, func(a, b jsonbMember) int {
		if len(a.key) != len(b.key) {
			return len(a.key) - len(b.key)
		}
		return strings.Compare(a.key, b.key)
	})
	for i := 1; i < len(members); i++ {
		if members[i].key == members[i-1].key {
			// the last value of a key stays: canonJSONB writes it
			return false
		}
	}
	w.tmp = append(w.tmp[:0], w.out[first:]...)
	w.out = w.out[:first]
	for i, m := range members {
		if i > 0 {
			w.out = append(w.out, ", "...)
		}
		w.out = append(append(append(w.out, '"'), m.key...), `": `...)
		w.out = append(w.out, w.tmp[m.start-first:m.end-first]...)
	}
	w.out = append(w.out, '}')
	w.spans = w.spans[:from]
	return true
}

// number writes a number of the text as it stands: one with an exponent or -0 in front is not
// one fastJSONB writes
func (w *jsonbWriter) number() bool {
	s, i := w.s, w.i
	digits := func() bool {
		j := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		return i > j
	}
	if s[i] == '-' {
		i++
	}
	if i < len(s) && s[i] == '0' {
		if s[w.i] == '-' {
			return false
		}
		i++
	} else if !digits() {
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		if !digits() {
			return false
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		return false
	}
	w.out = append(w.out, s[w.i:i]...)
	w.i = i
	return true
}
