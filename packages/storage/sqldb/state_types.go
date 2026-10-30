/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// The state of a block keeps the values of the columns as the text PostgreSQL outputs for them.
// A column type turns the values bound to a statement into that text, the way the input function
// of the type would, and the text into the value the driver returns for it. A value it cannot
// turn into exactly what PostgreSQL would store is not supported, and the statement then runs in
// PostgreSQL.

type colKind int

const (
	kOther colKind = iota
	kInt
	kNumeric
	kText
	kVarchar
	kBpchar
	kBool
	kBytea
	kJSONB
	kTimestamp
	kTimestamptz
	kUUID
)

// colType is the type of a column
type colType struct {
	oid    uint32
	name   string
	kind   colKind
	length int // of varchar and bpchar; -1 for none
	// precision and scale of numeric; -1 for none
	precision, scale int
	bits             int // of the integer types
}

func newColType(oid uint32, name string, typmod int32) colType {
	t := colType{oid: oid, name: name, length: -1, precision: -1, scale: -1}
	switch name {
	case "int2":
		t.kind, t.bits = kInt, 16
	case "int4":
		t.kind, t.bits = kInt, 32
	case "int8":
		t.kind, t.bits = kInt, 64
	case "numeric":
		t.kind = kNumeric
		if typmod >= 4 {
			mod := typmod - 4
			t.precision, t.scale = int((mod>>16)&0xffff), int(mod&0xffff)
			if t.scale > 1000 {
				t.kind = kOther // a negative scale
			}
		}
	case "text":
		t.kind = kText
	case "varchar":
		t.kind = kVarchar
		if typmod >= 4 {
			t.length = int(typmod - 4)
		}
	case "bpchar":
		t.kind = kBpchar
		if typmod >= 4 {
			t.length = int(typmod - 4)
		}
	case "bool":
		t.kind = kBool
	case "bytea":
		t.kind = kBytea
	case "jsonb":
		t.kind = kJSONB
	case "timestamp":
		if typmod < 0 {
			t.kind = kTimestamp
		}
	case "timestamptz":
		if typmod < 0 {
			t.kind = kTimestamptz
		}
	case "uuid":
		t.kind = kUUID
	}
	return t
}

// supported tells the state keeps values of the type
func (t colType) supported() bool { return t.kind != kOther }

// argText is the text of a value bound to a statement, as the driver sends it, and whether it is
// binary: the bytes of a bytea. false for a value the state does not take.
func argText(v any) (s string, binary, ok bool) {
	switch x := v.(type) {
	case string:
		return x, false, true
	case []byte:
		return string(x), true, true
	case int:
		return strconv.FormatInt(int64(x), 10), false, true
	case int8:
		return strconv.FormatInt(int64(x), 10), false, true
	case int16:
		return strconv.FormatInt(int64(x), 10), false, true
	case int32:
		return strconv.FormatInt(int64(x), 10), false, true
	case int64:
		return strconv.FormatInt(x, 10), false, true
	case uint8:
		return strconv.FormatUint(uint64(x), 10), false, true
	case uint16:
		return strconv.FormatUint(uint64(x), 10), false, true
	case uint32:
		return strconv.FormatUint(uint64(x), 10), false, true
	case uint64:
		return strconv.FormatUint(x, 10), false, true
	case decimal.Decimal:
		return x.String(), false, true
	case *decimal.Decimal:
		if x == nil {
			return "", false, false
		}
		return x.String(), false, true
	}
	return "", false, false
}

// input is the text PostgreSQL stores for a value bound to a column of the type; a nil value is
// NULL. false for a value the state does not take, or one PostgreSQL rejects.
func (t colType) input(v any) (*string, bool) {
	if v == nil {
		return nil, true
	}
	s, binary, ok := argText(v)
	if !ok {
		return nil, false
	}
	if binary {
		if t.kind == kBytea {
			out := byteaText([]byte(s))
			return &out, true
		}
		if !utf8.ValidString(s) {
			return nil, false
		}
	}
	return t.inputText(s)
}

// inputText is the text PostgreSQL stores for a text given to the input function of the type
func (t colType) inputText(s string) (*string, bool) {
	if !utf8.ValidString(s) {
		return nil, false
	}
	var out string
	switch t.kind {
	case kInt:
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, t.bits)
		if err != nil {
			return nil, false
		}
		out = strconv.FormatInt(n, 10)
	case kNumeric:
		n, ok := parseNumeric(s)
		if !ok {
			return nil, false
		}
		if n, ok = n.typmod(t.precision, t.scale); !ok {
			return nil, false
		}
		out = n.String()
	case kText:
		if strings.IndexByte(s, 0) >= 0 {
			return nil, false
		}
		out = s
	case kVarchar, kBpchar:
		if strings.IndexByte(s, 0) >= 0 {
			return nil, false
		}
		var ok bool
		if out, ok = t.fit(s); !ok {
			return nil, false
		}
	case kBool:
		b, ok := parseBool(s)
		if !ok {
			return nil, false
		}
		out = boolText(b)
	case kBytea:
		b, ok := parseByteaText(s)
		if !ok {
			return nil, false
		}
		out = byteaText(b)
	case kJSONB:
		var ok bool
		if out, ok = canonJSONB(s); !ok {
			return nil, false
		}
	case kTimestamp, kTimestamptz:
		tm, ok := parseTimestamp(s)
		if !ok {
			return nil, false
		}
		out = t.timestampText(tm)
	case kUUID:
		var ok bool
		if out, ok = parseUUID(s); !ok {
			return nil, false
		}
	default:
		return nil, false
	}
	return &out, true
}

// parseUUID is the text PostgreSQL outputs for a text the input function of uuid reads: 32 hex
// digits, in braces or not, with a hyphen after any group of four but the last
func parseUUID(s string) (string, bool) {
	braces := strings.HasPrefix(s, "{")
	if braces {
		s = s[1:]
	}
	var b [36]byte
	n := 0
	for i := range 16 {
		if len(s) < 2 || !isHex(s[0]) || !isHex(s[1]) {
			return "", false
		}
		if i == 4 || i == 6 || i == 8 || i == 10 {
			b[n] = '-'
			n++
		}
		b[n], b[n+1] = lowerHex(s[0]), lowerHex(s[1])
		n += 2
		s = s[2:]
		if i%2 == 1 && i < 15 && strings.HasPrefix(s, "-") {
			s = s[1:]
		}
	}
	if braces {
		if !strings.HasPrefix(s, "}") {
			return "", false
		}
		s = s[1:]
	}
	if s != "" {
		return "", false
	}
	return string(b[:]), true
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

func lowerHex(c byte) byte {
	if 'A' <= c && c <= 'F' {
		return c + 'a' - 'A'
	}
	return c
}

// fit is a text for a varchar or bpchar column: PostgreSQL drops the spaces past the length and
// rejects other characters past it, and pads a bpchar with spaces to the length
func (t colType) fit(s string) (string, bool) {
	n := utf8.RuneCountInString(s)
	if t.length >= 0 && n > t.length {
		cut := 0
		for i := 0; i < t.length; i++ {
			_, size := utf8.DecodeRuneInString(s[cut:])
			cut += size
		}
		if strings.Trim(s[cut:], " ") != "" {
			return "", false
		}
		s, n = s[:cut], t.length
	}
	if t.kind == kBpchar {
		length := t.length
		if length < 0 {
			// a bpchar with no length keeps the text as it is
			return s, true
		}
		if n < length {
			s += strings.Repeat(" ", length-n)
		}
	}
	return s, true
}

// key is the text of a value the equality of the type compares: equal values have the same key
func (t colType) key(s string) string {
	switch t.kind {
	case kNumeric:
		if n, ok := parseNumeric(s); ok {
			return n.normal()
		}
	case kBpchar:
		return strings.TrimRight(s, " ")
	}
	return s
}

// comparable tells the state compares values of the type for equality
func (t colType) comparable() bool {
	switch t.kind {
	case kInt, kNumeric, kText, kVarchar, kBpchar, kBool, kBytea, kUUID:
		return true
	}
	return false
}

// decode is the value the driver returns for a text PostgreSQL outputs for the type, as the pgx
// driver of database/sql decodes it
func decodeText(m *pgtype.Map, oid uint32, src string) (driver.Value, error) {
	b := []byte(src)
	switch oid {
	case pgtype.BoolOID:
		var d bool
		err := m.PlanScan(oid, pgtype.TextFormatCode, &d).Scan(b, &d)
		return d, err
	case pgtype.ByteaOID:
		var d []byte
		err := m.PlanScan(oid, pgtype.TextFormatCode, &d).Scan(b, &d)
		return d, err
	case pgtype.Int2OID, pgtype.Int4OID, pgtype.Int8OID:
		var d int64
		err := m.PlanScan(oid, pgtype.TextFormatCode, &d).Scan(b, &d)
		return d, err
	case pgtype.Float4OID, pgtype.Float8OID:
		var d float64
		err := m.PlanScan(oid, pgtype.TextFormatCode, &d).Scan(b, &d)
		return d, err
	case pgtype.JSONOID, pgtype.JSONBOID:
		var d []byte
		if err := m.PlanScan(oid, pgtype.TextFormatCode, &d).Scan(b, &d); err != nil {
			return nil, err
		}
		return d, nil
	case pgtype.TimestampOID:
		var d pgtype.Timestamp
		if err := m.PlanScan(oid, pgtype.TextFormatCode, &d).Scan(b, &d); err != nil {
			return nil, err
		}
		return d.Value()
	case pgtype.TimestamptzOID:
		var d pgtype.Timestamptz
		if err := m.PlanScan(oid, pgtype.TextFormatCode, &d).Scan(b, &d); err != nil {
			return nil, err
		}
		return d.Value()
	case pgtype.DateOID:
		var d pgtype.Date
		if err := m.PlanScan(oid, pgtype.TextFormatCode, &d).Scan(b, &d); err != nil {
			return nil, err
		}
		return d.Value()
	}
	var d string
	err := m.PlanScan(oid, pgtype.TextFormatCode, &d).Scan(b, &d)
	return d, err
}

// parseBool reads what boolin reads: a prefix of true, false, yes or no, on, off (at least two
// letters of these), 1 or 0
func parseBool(s string) (bool, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "":
		return false, false
	case strings.HasPrefix("true", s), strings.HasPrefix("yes", s), s == "on", s == "1":
		return true, true
	case strings.HasPrefix("false", s), strings.HasPrefix("no", s), len(s) >= 2 && strings.HasPrefix("off", s), s == "0":
		return false, true
	}
	return false, false
}

func boolText(b bool) string {
	if b {
		return "t"
	}
	return "f"
}

func byteaText(b []byte) string { return `\x` + hex.EncodeToString(b) }

// parseByteaText reads the hex format of bytea; the escape format is not supported
func parseByteaText(s string) ([]byte, bool) {
	if !strings.HasPrefix(s, `\x`) {
		return nil, false
	}
	b, err := hex.DecodeString(s[2:])
	return b, err == nil
}

// decodeHex is decode(s, 'hex'): pairs of hex digits, with no spaces
func decodeHex(s string) ([]byte, bool) {
	b, err := hex.DecodeString(s)
	return b, err == nil
}

// parseTimestamp reads a timestamp in the ISO form YYYY-MM-DD[ HH:MM:SS[.ffffff]]
func parseTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02T15:04:05.999999", "2006-01-02"} {
		if tm, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			// PostgreSQL rounds digits past the microseconds, which the state leaves to it
			if tm.Year() < 1 || tm.Year() > 9999 || tm.Nanosecond()%1000 != 0 {
				return time.Time{}, false
			}
			return tm, true
		}
	}
	return time.Time{}, false
}

// unixTimestamp is to_timestamp(s): the seconds since the epoch, rounded to microseconds
func unixTimestamp(s string) (time.Time, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return time.Time{}, false
	}
	// as float8_timestamptz: the seconds since 2000-01-01 in float8, then rounded to microseconds
	const pgEpoch = 946684800
	us := math.RoundToEven((f - pgEpoch) * 1e6)
	if math.Abs(us) > 1e18 {
		return time.Time{}, false
	}
	tm := time.UnixMicro(int64(us) + pgEpoch*1e6).UTC()
	if tm.Year() < 1 || tm.Year() > 9999 {
		return time.Time{}, false
	}
	return tm, true
}

// timestampText is the text PostgreSQL outputs for a time of the type, in the time zone UTC of
// the connections of the node
func (t colType) timestampText(tm time.Time) string {
	tm = tm.UTC()
	s := tm.Format("2006-01-02 15:04:05")
	if us := tm.Nanosecond() / 1000; us != 0 {
		s += strings.TrimRight(fmt.Sprintf(".%06d", us), "0")
	}
	if t.kind == kTimestamptz {
		s += "+00"
	}
	return s
}

// numeric is a value of the numeric type: digits scaled by 10^-scale, with the scale PostgreSQL
// displays
type numeric struct {
	v     *big.Int
	scale int
}

// parseNumeric reads the text the input function of numeric reads: no NaN nor infinities
func parseNumeric(s string) (numeric, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return numeric{}, false
	}
	neg := false
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		neg, s = true, s[1:]
	}
	mant, exp := s, 0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mant = s[:i]
		e, err := strconv.Atoi(s[i+1:])
		if err != nil || e > 1000 || e < -1000 {
			return numeric{}, false
		}
		exp = e
	}
	intPart, frac := mant, ""
	if i := strings.IndexByte(mant, '.'); i >= 0 {
		intPart, frac = mant[:i], mant[i+1:]
	}
	if intPart == "" && frac == "" {
		return numeric{}, false
	}
	for _, c := range intPart + frac {
		if c < '0' || c > '9' {
			return numeric{}, false
		}
	}
	v, ok := new(big.Int).SetString("0"+intPart+frac, 10)
	if !ok {
		return numeric{}, false
	}
	scale := len(frac) - exp
	if scale < 0 {
		v.Mul(v, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-scale)), nil))
		scale = 0
	}
	if neg {
		v.Neg(v)
	}
	return numeric{v: v, scale: scale}, true
}

// rescale is the value with the scale, rounded half away from zero
func (n numeric) rescale(scale int) numeric {
	if scale >= n.scale {
		v := new(big.Int).Mul(n.v, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale-n.scale)), nil))
		return numeric{v: v, scale: scale}
	}
	d := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n.scale-scale)), nil)
	q, r := new(big.Int).QuoRem(n.v, d, new(big.Int))
	r.Abs(r).Mul(r, big.NewInt(2))
	if r.Cmp(d) >= 0 {
		if n.v.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return numeric{v: q, scale: scale}
}

// typmod is the value a column numeric(precision, scale) stores; false when it overflows
func (n numeric) typmod(precision, scale int) (numeric, bool) {
	if precision < 0 {
		return n, true
	}
	n = n.rescale(scale)
	digits := len(new(big.Int).Abs(n.v).String())
	if n.v.Sign() == 0 {
		digits = 0
	}
	if digits-scale > precision-scale {
		return numeric{}, false
	}
	return n, true
}

func (n numeric) add(m numeric) numeric {
	s := max(n.scale, m.scale)
	a, b := n.rescale(s), m.rescale(s)
	return numeric{v: new(big.Int).Add(a.v, b.v), scale: s}
}

func (n numeric) neg() numeric { return numeric{v: new(big.Int).Neg(n.v), scale: n.scale} }

func (n numeric) sign() int { return n.v.Sign() }

// String is the text PostgreSQL outputs
func (n numeric) String() string {
	digits := new(big.Int).Abs(n.v).String()
	if n.scale > 0 {
		if len(digits) <= n.scale {
			digits = strings.Repeat("0", n.scale-len(digits)+1) + digits
		}
		digits = digits[:len(digits)-n.scale] + "." + digits[len(digits)-n.scale:]
	}
	if n.v.Sign() < 0 {
		return "-" + digits
	}
	return digits
}

// normal is the text of the value with no trailing zeros of the fraction
func (n numeric) normal() string {
	s := n.String()
	if strings.IndexByte(s, '.') >= 0 {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		s = "0"
	}
	return s
}

// canonJSONB is the text PostgreSQL outputs for a jsonb value read from the text
func canonJSONB(s string) (string, bool) {
	v, ok := readJSON(s)
	if !ok {
		return "", false
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/4)
	if !writeJSONB(&b, v) {
		return "", false
	}
	return b.String(), true
}

// jsonObject is an object of jsonb: the last value of a key stays
type jsonObject struct {
	keys []string
	vals map[string]any
}

// readJSON reads the one JSON value of a text, with nothing but white space around it. Numbers
// stay json.Number, an empty array is []any{}. false for a text that is not JSON and for those
// PostgreSQL may take otherwise than the state: a NUL in a string or key, a byte that is not UTF-8.
func readJSON(s string) (any, bool) {
	sc := jsonScanner{s: s}
	sc.ws()
	v, ok := sc.value(0)
	if !ok {
		return nil, false
	}
	sc.ws()
	return v, sc.i == len(s)
}

// jsonMaxDepth bounds the nesting the scanner follows; deeper texts are PostgreSQL's
const jsonMaxDepth = 10000

type jsonScanner struct {
	s string
	i int
}

func (sc *jsonScanner) ws() {
	for sc.i < len(sc.s) {
		switch sc.s[sc.i] {
		case ' ', '\t', '\n', '\r':
			sc.i++
		default:
			return
		}
	}
}

func (sc *jsonScanner) value(depth int) (any, bool) {
	if sc.i >= len(sc.s) || depth >= jsonMaxDepth {
		return nil, false
	}
	switch c := sc.s[sc.i]; {
	case c == '{':
		sc.i++
		o := &jsonObject{vals: make(map[string]any)}
		sc.ws()
		if sc.i < len(sc.s) && sc.s[sc.i] == '}' {
			sc.i++
			return o, true
		}
		for {
			if sc.i >= len(sc.s) || sc.s[sc.i] != '"' {
				return nil, false
			}
			k, ok := sc.str()
			if !ok {
				return nil, false
			}
			sc.ws()
			if sc.i >= len(sc.s) || sc.s[sc.i] != ':' {
				return nil, false
			}
			sc.i++
			sc.ws()
			v, ok := sc.value(depth + 1)
			if !ok {
				return nil, false
			}
			if _, seen := o.vals[k]; !seen {
				o.keys = append(o.keys, k)
			}
			o.vals[k] = v
			sc.ws()
			if sc.i >= len(sc.s) {
				return nil, false
			}
			switch sc.s[sc.i] {
			case ',':
				sc.i++
				sc.ws()
			case '}':
				sc.i++
				return o, true
			default:
				return nil, false
			}
		}
	case c == '[':
		sc.i++
		a := []any{}
		sc.ws()
		if sc.i < len(sc.s) && sc.s[sc.i] == ']' {
			sc.i++
			return a, true
		}
		for {
			v, ok := sc.value(depth + 1)
			if !ok {
				return nil, false
			}
			a = append(a, v)
			sc.ws()
			if sc.i >= len(sc.s) {
				return nil, false
			}
			switch sc.s[sc.i] {
			case ',':
				sc.i++
				sc.ws()
			case ']':
				sc.i++
				return a, true
			default:
				return nil, false
			}
		}
	case c == '"':
		return sc.str()
	case c == '-' || c >= '0' && c <= '9':
		return sc.number()
	case strings.HasPrefix(sc.s[sc.i:], "true"):
		sc.i += 4
		return true, true
	case strings.HasPrefix(sc.s[sc.i:], "false"):
		sc.i += 5
		return false, true
	case strings.HasPrefix(sc.s[sc.i:], "null"):
		sc.i += 4
		return nil, true
	}
	return nil, false
}

// str reads a string at its opening quote. Its value has no NUL and is UTF-8. An escape of a UTF-16
// surrogate, alone or in a pair, leaves the text to PostgreSQL, which rejects a lone one.
func (sc *jsonScanner) str() (string, bool) {
	start := sc.i + 1
	i := start
	for i < len(sc.s) {
		c := sc.s[i]
		if c == '"' {
			v := sc.s[start:i]
			if !utf8.ValidString(v) {
				return "", false
			}
			sc.i = i + 1
			return v, true
		}
		if c == '\\' {
			break
		}
		if c < 0x20 {
			return "", false
		}
		i++
	}
	b := []byte(sc.s[start:i])
	for i < len(sc.s) {
		c := sc.s[i]
		switch {
		case c == '"':
			if !utf8.Valid(b) {
				return "", false
			}
			sc.i = i + 1
			return string(b), true
		case c < 0x20:
			return "", false
		case c != '\\':
			b = append(b, c)
			i++
			continue
		}
		if i+1 >= len(sc.s) {
			return "", false
		}
		switch e := sc.s[i+1]; e {
		case '"', '\\', '/':
			b = append(b, e)
		case 'b':
			b = append(b, '\b')
		case 'f':
			b = append(b, '\f')
		case 'n':
			b = append(b, '\n')
		case 'r':
			b = append(b, '\r')
		case 't':
			b = append(b, '\t')
		case 'u':
			if i+6 > len(sc.s) {
				return "", false
			}
			r, err := strconv.ParseUint(sc.s[i+2:i+6], 16, 32)
			if err != nil || r == 0 || r >= 0xd800 && r < 0xe000 {
				return "", false
			}
			b = utf8.AppendRune(b, rune(r))
			i += 4
		default:
			return "", false
		}
		i += 2
	}
	return "", false
}

// number reads -?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)? as it stands in the text
func (sc *jsonScanner) number() (json.Number, bool) {
	s, i := sc.s, sc.i
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
		i++
	} else if !digits() {
		return "", false
	}
	if i < len(s) && s[i] == '.' {
		i++
		if !digits() {
			return "", false
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if !digits() {
			return "", false
		}
	}
	n := json.Number(s[sc.i:i])
	sc.i = i
	return n, true
}

// writeJSONB writes a value as jsonb outputs it: the keys of an object by length and then by
// bytes, a space after each comma and colon, numbers as numeric outputs them
func writeJSONB(b *strings.Builder, v any) bool {
	switch x := v.(type) {
	case *jsonObject:
		keys := slices.Clone(x.keys)
		slices.SortFunc(keys, func(a, c string) int {
			if len(a) != len(c) {
				return len(a) - len(c)
			}
			return strings.Compare(a, c)
		})
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			writeJSONString(b, k)
			b.WriteString(": ")
			if !writeJSONB(b, x.vals[k]) {
				return false
			}
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteString(", ")
			}
			if !writeJSONB(b, e) {
				return false
			}
		}
		b.WriteByte(']')
	case string:
		writeJSONString(b, x)
	case json.Number:
		// numeric outputs a number of JSON without an exponent as it stands, but for the sign of zero
		if t := string(x); !strings.ContainsAny(t, "eE") && !strings.HasPrefix(t, "-0") {
			b.WriteString(t)
			break
		}
		n, ok := parseNumeric(string(x))
		if !ok {
			return false
		}
		b.WriteString(n.String())
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case nil:
		b.WriteString("null")
	default:
		return false
	}
	return true
}

// writeJSONString writes a string as escape_json of PostgreSQL escapes it
func writeJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				fmt.Fprintf(b, `\u%04x`, c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
}

// concatJSONB is a || b of two jsonb objects: the keys of b replace those of a. false for values
// that are not both objects.
func concatJSONB(a, c string) (string, bool) {
	read := func(s string) (*jsonObject, bool) {
		v, ok := readJSON(s)
		o, isObj := v.(*jsonObject)
		return o, ok && isObj
	}
	x, ok := read(a)
	if !ok {
		return "", false
	}
	y, ok := read(c)
	if !ok {
		return "", false
	}
	for _, k := range y.keys {
		if _, seen := x.vals[k]; !seen {
			x.keys = append(x.keys, k)
		}
		x.vals[k] = y.vals[k]
	}
	var b strings.Builder
	if !writeJSONB(&b, x) {
		return "", false
	}
	return b.String(), true
}
