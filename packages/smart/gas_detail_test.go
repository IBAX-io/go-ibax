package smart

import (
	"math/rand/v2"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
)

// roundTrip is the value through JSON, as the details took each of their parts before they took
// the whole only
func roundTrip(v any) any {
	b, _ := JSONEncode(v)
	s, _ := JSONDecode(b)
	return s
}

// oldDetail and oldDetailCombustion are PaymentInfo.Detail and PaymentInfo.DetailCombustion as
// they were, with a round trip of each part
func oldDetail(pay *PaymentInfo) any {
	detail := types.NewMap()
	for _, f := range pay.FuelCategories {
		detail.Set(f.FuelType.String(), roundTrip(f.FeesInfo()))
	}
	detail.Set("taxes_size", pay.TaxesSize)
	detail.Set("payment_type", pay.PaymentType.String())
	detail.Set("fuel_rate", pay.FuelRate)
	detail.Set("token_symbol", pay.Ecosystem.TokenSymbol)
	return roundTrip(detail)
}

func oldDetailCombustion(pay *PaymentInfo) any {
	c := pay.Combustion
	detail := types.NewMap()
	var money decimal.Decimal
	for _, f := range pay.FuelCategories {
		detail.Set(f.FuelType.String(), f.Fees().Mul(decimal.NewFromInt(c.Percent)).Div(decimal.New(100, 0)).Floor())
		money = money.Add(f.Fees())
	}
	if !pay.Indirect && pay.TokenEco != consts.DefaultTokenEcosystem {
		detail.Set("combustion", roundTrip(pay.Combustion.Detail(money)))
	}
	detail.Set("token_symbol", pay.Ecosystem.TokenSymbol)
	detail.Set("fuel_rate", pay.FuelRate)
	return roundTrip(detail)
}

// TestDetailOneRoundTrip checks the details of payments, which take the whole through JSON once,
// are those that took each part through it too
func TestDetailOneRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	rates := []float64{0, 1, 100, 1.5, 33.333333333333336, 1e21, 1e-7, 123456789.125}
	for range 2000 {
		pay := &PaymentInfo{
			TokenEco:    r.Int64N(3) + 1,
			PaymentType: PaymentType(r.IntN(4)),
			FuelRate:    decimal.New(r.Int64N(1e12), -int32(r.IntN(8))),
			TaxesSize:   r.Int64N(10),
			Indirect:    r.IntN(2) == 0,
			Ecosystem:   new(sqldb.Ecosystem),
			Combustion:  &Combustion{Flag: r.Int64N(3), Percent: r.Int64N(101)},
		}
		pay.Ecosystem.TokenSymbol = []string{"IBXC", "<a&b>", `q"t`, ""}[r.IntN(4)]
		for k := range r.IntN(4) {
			pay.PushFuelCategories(FuelCategory{
				FuelType:       FuelType(k + 1),
				Decimal:        decimal.New(r.Int64N(1e15), -int32(r.IntN(4))),
				ConversionRate: rates[r.IntN(len(rates))],
				Flag:           GasPayAbleType(r.IntN(3)),
				Arithmetic:     Arithmetic(r.IntN(4)),
			})
		}
		for _, c := range []struct {
			name     string
			old, new any
		}{{"Detail", oldDetail(pay), pay.Detail()}, {"DetailCombustion", oldDetailCombustion(pay), pay.DetailCombustion()}} {
			a, _ := JSONEncode(c.old)
			b, _ := JSONEncode(c.new)
			if a != b {
				t.Fatalf("%s:\n old %s\n new %s", c.name, a, b)
			}
		}
	}
}
