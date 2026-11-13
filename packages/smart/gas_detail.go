/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/shopspring/decimal"
)

// The detail of a payment (value_detail of 1_history) is the text the history row is given: what
// the detail was once taken through JSONEncode and JSONDecode and then through json.Marshal of the
// query builder. That gave an object of the keys in byte order, without spaces; decimals as strings,
// integers and the conversion rate as numbers. The text is written here at once. A detail JSON
// does not take, a conversion rate that is not finite or a string with the text of an HTML escape,
// left the history nothing: the detail is nil then.

// detailEntry is a key of a detail with the JSON text of its value
type detailEntry struct {
	key string
	val []byte
}

// detailObject is the object of a detail. Its keys are names and numbers, which JSON takes as they
// are; a key set again keeps its last value, as it did in the map the detail was. A nil value is
// one JSON did not take.
type detailObject []detailEntry

func (o *detailObject) set(key string, val []byte) {
	for i := range *o {
		if (*o)[i].key == key {
			(*o)[i].val = val
			return
		}
	}
	*o = append(*o, detailEntry{key: key, val: val})
}

// appendTo appends the object, its keys in byte order
func (o detailObject) appendTo(b []byte) []byte {
	slices.SortFunc(o, func(x, y detailEntry) int { return strings.Compare(x.key, y.key) })
	b = append(b, '{')
	for i, e := range o {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '"')
		b = append(b, e.key...)
		b = append(b, '"', ':')
		b = append(b, e.val...)
	}
	return append(b, '}')
}

// appendDetailDecimal appends a decimal as a string, as its MarshalJSON gives it
func appendDetailDecimal(b []byte, d decimal.Decimal) []byte {
	b = append(b, '"')
	b = append(b, d.String()...)
	return append(b, '"')
}

// appendDetailFloat appends a number as JSON gives it and takes it back: in the form of
// json.Marshal, but an integer that fits int64, -0 too, as that integer. false for a number that
// is not finite, which json.Marshal refuses.
func appendDetailFloat(b []byte, f float64) ([]byte, bool) {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return b, false
	}
	if f == math.Trunc(f) && math.Abs(f) < 1<<53 {
		return strconv.AppendInt(b, int64(f), 10), true
	}
	format := byte('f')
	if abs := math.Abs(f); abs < 1e-6 || abs >= 1e21 {
		format = 'e'
	}
	b = strconv.AppendFloat(b, f, format, -1, 64)
	if format == 'e' {
		// e-09 is e-9
		if n := len(b); n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}
	return b, true
}

// appendDetailString appends a string as JSON gives it after a round trip of JSONEncode and
// JSONDecode. A string of printable ASCII that JSON does not escape is as it is; any other takes the
// round trip itself. false for a string the round trip fails: JSONEncode turns the text of an
// escape of < > & back into the character, which leaves the escape of a backslash before it broken.
func appendDetailString(b []byte, s string) ([]byte, bool) {
	plain := true
	for i := 0; i < len(s) && plain; i++ {
		c := s[i]
		plain = c >= 0x20 && c < 0x7f && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&'
	}
	if plain {
		b = append(b, '"')
		b = append(b, s...)
		return append(b, '"'), true
	}
	enc, err := json.Marshal(s)
	if err != nil {
		return b, false
	}
	text := string(enc)
	text = strings.Replace(text, `\u003c`, `<`, -1)
	text = strings.Replace(text, `\u003e`, `>`, -1)
	text = strings.Replace(text, `\u0026`, `&`, -1)
	var v any
	if err = unmarshalJSON([]byte(text), &v, "unmarshalling json"); err != nil {
		return b, false
	}
	out, err := json.Marshal(v)
	if err != nil {
		return b, false
	}
	return append(b, out...), true
}

// feesDetail is the detail of the fees of the category. The fees come first, as they did when
// the category gave its part of the detail before any was written.
func (f *FuelCategory) feesDetail() ([]byte, bool) {
	fees := f.Fees()
	b := make([]byte, 0, 128)
	b = append(b, `{"arithmetic":`...)
	b, ok := appendDetailString(b, f.Arithmetic.String())
	if !ok {
		return nil, false
	}
	b = append(b, `,"conversion_rate":`...)
	if b, ok = appendDetailFloat(b, f.ConversionRate); !ok {
		return nil, false
	}
	b = append(b, `,"decimal":`...)
	b = appendDetailDecimal(b, f.Decimal)
	b = append(b, `,"flag":`...)
	b = strconv.AppendInt(b, int64(f.Flag), 10)
	b = append(b, `,"value":`...)
	b = appendDetailDecimal(b, fees)
	return append(b, '}'), true
}

// combustionDetail is the detail of the combustion of sum
func (c Combustion) combustionDetail(sum decimal.Decimal) []byte {
	combustion := c.Fees(sum)
	b := make([]byte, 0, 96)
	b = append(b, `{"after":`...)
	b = appendDetailDecimal(b, sum.Sub(combustion))
	b = append(b, `,"before":`...)
	b = appendDetailDecimal(b, sum)
	b = append(b, `,"flag":`...)
	b = strconv.AppendInt(b, c.Flag, 10)
	b = append(b, `,"percent":`...)
	b = strconv.AppendInt(b, c.Percent, 10)
	b = append(b, `,"value":`...)
	b = appendDetailDecimal(b, combustion)
	return append(b, '}')
}

// detailText is the text of a detail; nil for a detail with a value JSON did not take, which the
// round trip of the whole failed for
func detailText(o detailObject) any {
	n := 2
	for _, e := range o {
		if e.val == nil {
			return nil
		}
		n += len(e.key) + len(e.val) + 4
	}
	return string(o.appendTo(make([]byte, 0, n)))
}

// detailString is the JSON text of a string of a detail, nil for one the round trip fails
func detailString(s string) []byte {
	b, ok := appendDetailString(nil, s)
	if !ok {
		return nil
	}
	return b
}

// Detail is the detail of the payment as the history keeps it: the JSON text of the fees of each
// category, the taxes, the payment type, the fuel rate and the token symbol; nil for none.
func (pay *PaymentInfo) Detail() any {
	detail := make(detailObject, 0, len(pay.FuelCategories)+4)
	for i := 0; i < len(pay.FuelCategories); i++ {
		f := &pay.FuelCategories[i]
		fees, _ := f.feesDetail()
		detail.set(f.FuelType.String(), fees)
	}
	detail.set("taxes_size", strconv.AppendInt(nil, pay.TaxesSize, 10))
	detail.set("payment_type", detailString(pay.PaymentType.String()))
	detail.set("fuel_rate", appendDetailDecimal(nil, pay.FuelRate))
	detail.set("token_symbol", detailString(pay.Ecosystem.TokenSymbol))
	return detailText(detail)
}

// DetailCombustion is the detail of the combustion of the payment as the history keeps it: the JSON
// text of the part of the fees of each category burnt, the combustion, the token symbol and the
// fuel rate; nil for none.
func (pay *PaymentInfo) DetailCombustion() any {
	c := pay.Combustion
	detail := make(detailObject, 0, len(pay.FuelCategories)+3)
	var money decimal.Decimal
	for i := 0; i < len(pay.FuelCategories); i++ {
		f := pay.FuelCategories[i]
		s := f.Fees().Mul(decimal.NewFromInt(c.Percent)).Div(decimal.New(100, 0)).Floor()
		detail.set(f.FuelType.String(), appendDetailDecimal(nil, s))
		money = money.Add(f.Fees())
	}
	if !pay.Indirect && pay.TokenEco != consts.DefaultTokenEcosystem {
		detail.set("combustion", pay.Combustion.combustionDetail(money))
	}
	detail.set("token_symbol", detailString(pay.Ecosystem.TokenSymbol))
	detail.set("fuel_rate", appendDetailDecimal(nil, pay.FuelRate))
	return detailText(detail)
}
