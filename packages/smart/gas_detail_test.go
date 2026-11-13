package smart

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
)

// refFeesInfo, refCombustionDetail, refDetail and refDetailCombustion are the details of payments
// as they were made: maps taken through JSONEncode and JSONDecode, which the query builder took
// through json.Marshal.
func refFeesInfo(f *FuelCategory) any {
	detail := types.NewMap()
	detail.Set("decimal", f.Decimal)
	detail.Set("value", f.Fees())
	detail.Set("conversion_rate", f.ConversionRate)
	detail.Set("flag", f.Flag)
	detail.Set("arithmetic", f.Arithmetic.String())
	return detail
}

func refCombustionDetail(c Combustion, sum decimal.Decimal) any {
	detail := types.NewMap()
	combustion := c.Fees(sum)
	detail.Set("flag", c.Flag)
	detail.Set("percent", c.Percent)
	detail.Set("before", sum)
	detail.Set("value", combustion)
	detail.Set("after", sum.Sub(combustion))
	return detail
}

func refDetail(pay *PaymentInfo) any {
	detail := types.NewMap()
	for i := 0; i < len(pay.FuelCategories); i++ {
		f := &pay.FuelCategories[i]
		detail.Set(f.FuelType.String(), refFeesInfo(f))
	}
	detail.Set("taxes_size", pay.TaxesSize)
	detail.Set("payment_type", pay.PaymentType.String())
	detail.Set("fuel_rate", pay.FuelRate)
	detail.Set("token_symbol", pay.Ecosystem.TokenSymbol)
	b, _ := JSONEncode(detail)
	s, _ := JSONDecode(b)
	return s
}

func refDetailCombustion(pay *PaymentInfo) any {
	c := pay.Combustion
	detail := types.NewMap()
	var money decimal.Decimal
	for i := 0; i < len(pay.FuelCategories); i++ {
		f := pay.FuelCategories[i]
		s := f.Fees().Mul(decimal.NewFromInt(c.Percent)).Div(decimal.New(100, 0)).Floor()
		detail.Set(f.FuelType.String(), s)
		money = money.Add(f.Fees())
	}
	if !pay.Indirect && pay.TokenEco != consts.DefaultTokenEcosystem {
		detail.Set("combustion", refCombustionDetail(*pay.Combustion, money))
	}
	detail.Set("token_symbol", pay.Ecosystem.TokenSymbol)
	detail.Set("fuel_rate", pay.FuelRate)
	b, _ := JSONEncode(detail)
	s, _ := JSONDecode(b)
	return s
}

// sentText is the text of value_detail the query builder sends for a detail, and whether the
// detail is one. A detail that panics is reported as such.
func sentText(detail func(*PaymentInfo) any, pay *PaymentInfo) (text string, isNil, panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	v := detail(pay)
	text, err := converter.InterfaceToStr(v)
	if err != nil {
		text = "error: " + err.Error()
	}
	return text, v == nil, false
}

// checkDetails checks that the details of the payment are the texts the maps gave
func checkDetails(t *testing.T, pay *PaymentInfo) {
	t.Helper()
	for _, c := range []struct {
		name     string
		ref, new func(*PaymentInfo) any
	}{
		{"Detail", refDetail, (*PaymentInfo).Detail},
		{"DetailCombustion", refDetailCombustion, (*PaymentInfo).DetailCombustion},
	} {
		refText, refNil, refPanic := sentText(c.ref, pay)
		newText, newNil, newPanic := sentText(c.new, pay)
		if refText != newText || refNil != newNil || refPanic != newPanic {
			t.Fatalf("%s:\n old %q nil %v panic %v\n new %q nil %v panic %v",
				c.name, refText, refNil, refPanic, newText, newNil, newPanic)
		}
	}
}

var detailSymbols = []string{
	"IBXC", "", "<a&b>", `q"t`, `back\slash`, `\u003c`, `x\u003ey`, `\\u0026`, `\u003C`, `\u0041`,
	"tab\tnew\nline\r", "\x00\x01\x1f\x7f", "\b\f", "\u2028\u2029", "\xff\xfe", "a\xc3", "é中文🙂",
	"\ufffd", "</script>", `{"k":1}`, "'single'",
}

var detailRates = []float64{
	0, math.Copysign(0, -1), 1, 100, 1.5, -2.5, 33.333333333333336, 1e21, 1e20, 1e-6, 1e-7, 5e-324,
	123456789.125, 1 << 53, 1<<53 + 2, 1 << 62, 1 << 63, 9.2e18, 1e300, math.MaxFloat64,
	math.SmallestNonzeroFloat64, -1e-9, math.Inf(1), math.Inf(-1), math.NaN(),
}

// TestDetailText checks the texts of the details against those of the maps over payments that try
// the escapes of strings, the forms of numbers, the keys and their order
func TestDetailText(t *testing.T) {
	for _, symbol := range detailSymbols {
		for _, rate := range detailRates {
			for _, eco := range []int64{1, 2} {
				pay := &PaymentInfo{
					TokenEco:    eco,
					PaymentType: PaymentType_ContractBinder,
					FuelRate:    decimal.New(1234567, -3),
					TaxesSize:   3,
					Ecosystem:   &sqldb.Ecosystem{TokenSymbol: symbol},
					Combustion:  &Combustion{Flag: 2, Percent: 7},
				}
				pay.PushFuelCategories(
					FuelCategory{FuelType: FuelType_vmCost_fee, Decimal: decimal.New(98765, -2), ConversionRate: rate, Flag: GasPayAbleType_Capable},
					FuelCategory{FuelType: FuelType_storage_fee, Decimal: decimal.NewFromInt(-42), ConversionRate: 100, Flag: GasPayAbleType_Unable, Arithmetic: Arithmetic_MUL},
				)
				checkDetails(t, pay)
			}
		}
	}
	// keys that are numbers, keys set twice, no categories, large integers, a missing combustion
	for _, pay := range []*PaymentInfo{
		{Ecosystem: &sqldb.Ecosystem{}, Combustion: &Combustion{}},
		{Ecosystem: &sqldb.Ecosystem{}, TokenEco: 5, TaxesSize: math.MinInt64, PaymentType: 77,
			Combustion: &Combustion{Flag: math.MaxInt64, Percent: math.MinInt64},
			FuelCategories: []FuelCategory{
				{FuelType: 12, Decimal: decimal.NewFromInt(1), Flag: -3, Arithmetic: 9, ConversionRate: 100},
				{FuelType: -1, Decimal: decimal.New(5, 40), Flag: math.MaxInt32},
				{FuelType: FuelType_vmCost_fee, Decimal: decimal.New(1, -30)},
				{FuelType: 12, Decimal: decimal.NewFromInt(2), ConversionRate: 0.5, Arithmetic: Arithmetic_DIV},
			}},
		{Ecosystem: &sqldb.Ecosystem{}, Combustion: nil,
			FuelCategories: []FuelCategory{{FuelType: FuelType_vmCost_fee}}},
		{Ecosystem: nil, Combustion: &Combustion{}},
		{Ecosystem: &sqldb.Ecosystem{}, Combustion: &Combustion{},
			FuelCategories: []FuelCategory{{ConversionRate: math.Inf(1), Arithmetic: Arithmetic_MUL}}},
		// a category JSON does not take, replaced by a later one of its type, and the reverse
		{Ecosystem: &sqldb.Ecosystem{}, Combustion: &Combustion{},
			FuelCategories: []FuelCategory{{ConversionRate: math.Inf(1)}, {ConversionRate: 100}}},
		{Ecosystem: &sqldb.Ecosystem{}, Combustion: &Combustion{},
			FuelCategories: []FuelCategory{{ConversionRate: 100}, {ConversionRate: math.Inf(1)}}},
	} {
		checkDetails(t, pay)
	}
}

// TestDetailRandom checks the texts of the details against those of the maps over random payments
func TestDetailRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 5000 {
		pay := &PaymentInfo{
			TokenEco:    r.Int64N(3) + 1,
			PaymentType: PaymentType(r.IntN(5)),
			FuelRate:    decimal.New(r.Int64()-r.Int64(), -int32(r.IntN(20))),
			TaxesSize:   r.Int64() - r.Int64(),
			Indirect:    r.IntN(2) == 0,
			Ecosystem:   &sqldb.Ecosystem{TokenSymbol: detailSymbols[r.IntN(len(detailSymbols))]},
			Combustion:  &Combustion{Flag: r.Int64N(3), Percent: r.Int64N(101)},
		}
		for range r.IntN(5) {
			pay.PushFuelCategories(FuelCategory{
				FuelType:       FuelType(r.IntN(5)),
				Decimal:        decimal.New(r.Int64N(1e18), -int32(r.IntN(10))),
				ConversionRate: detailRates[r.IntN(len(detailRates))] * float64(r.IntN(3)+1),
				Flag:           GasPayAbleType(r.IntN(4)),
				Arithmetic:     Arithmetic(r.IntN(5)),
			})
		}
		checkDetails(t, pay)
	}
}

// FuzzDetailText checks the texts of the details against those of the maps
func FuzzDetailText(f *testing.F) {
	for i, symbol := range detailSymbols {
		rate := detailRates[i%len(detailRates)]
		f.Add(symbol, rate, int64(98765), int8(-2), int32(1), int32(2), int32(0), int64(3), int32(1),
			int64(7), int64(2), int64(1234567), int8(-3), false, int64(2), uint8(3))
	}
	f.Add("IBXC", 100.0, int64(math.MaxInt64), int8(30), int32(-1), int32(-7), int32(3), int64(math.MinInt64),
		int32(9), int64(math.MinInt64), int64(math.MaxInt64), int64(-1), int8(-40), true, int64(1), uint8(4))
	f.Fuzz(func(t *testing.T, symbol string, rate float64, dec int64, exp int8, fuelType, flag, arithmetic int32,
		taxes int64, paymentType int32, percent, combFlag, fuelRate int64, fuelExp int8, indirect bool,
		tokenEco int64, categories uint8) {
		pay := &PaymentInfo{
			TokenEco:    tokenEco,
			PaymentType: PaymentType(paymentType),
			FuelRate:    decimal.New(fuelRate, int32(fuelExp)),
			TaxesSize:   taxes,
			Indirect:    indirect,
			Ecosystem:   &sqldb.Ecosystem{TokenSymbol: symbol},
			Combustion:  &Combustion{Flag: combFlag, Percent: percent},
		}
		for k := range int(categories % 6) {
			pay.PushFuelCategories(FuelCategory{
				FuelType:       FuelType(fuelType + int32(k%3)),
				Decimal:        decimal.New(dec+int64(k), int32(exp)),
				ConversionRate: rate * float64(k+1),
				Flag:           GasPayAbleType(flag),
				Arithmetic:     Arithmetic(arithmetic + int32(k)),
			})
		}
		checkDetails(t, pay)
	})
}

func benchPayment() *PaymentInfo {
	pay := &PaymentInfo{
		TokenEco: 1, PaymentType: PaymentType_ContractCaller, FuelRate: decimal.NewFromInt(1000000),
		TaxesSize: 3, Ecosystem: &sqldb.Ecosystem{TokenSymbol: "IBXC"}, Combustion: &Combustion{Flag: 1},
	}
	pay.PushFuelCategories(
		NewFuelCategory(FuelType_vmCost_fee, decimal.NewFromInt(123456789), GasPayAbleType_Unable, 100),
		NewFuelCategory(FuelType_storage_fee, decimal.NewFromInt(2500000000), GasPayAbleType_Unable, 100),
		NewFuelCategory(FuelType_expedite_fee, decimal.Zero, GasPayAbleType_Unable, 100),
	)
	return pay
}

// BenchmarkDetailText compares the text of a detail with the maps it was made of
func BenchmarkDetailText(b *testing.B) {
	pay := benchPayment()
	b.Run("maps", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = converter.InterfaceToStr(refDetail(pay))
		}
	})
	b.Run("text", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = converter.InterfaceToStr(pay.Detail())
		}
	})
}
