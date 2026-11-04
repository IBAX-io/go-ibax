/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"math"
	"math/big"
	"strconv"

	"github.com/shopspring/decimal"
)

func parseFloat(x any) (float64, error) {
	var (
		fx  float64
		err error
	)
	switch v := x.(type) {
	case float64:
		fx = v
	case int64:
		fx = float64(v)
	case string:
		if fx, err = strconv.ParseFloat(v, 64); err != nil {
			return 0, errFloat
		}
	default:
		return 0, errFloat
	}
	return fx, nil
}

func isValidFloat(x float64) bool {
	return !(math.IsNaN(x) || math.IsInf(x, 1) || math.IsInf(x, -1))
}

// Floor returns the greatest integer value less than or equal to x
func Floor(x any) (int64, error) {
	fx, err := parseFloat(x)
	if err != nil {
		return 0, err
	}
	if fx = math.Floor(fx); isValidFloat(fx) {
		return int64(fx), nil
	}
	return 0, errFloatResult
}

// Log returns the natural logarithm of x
func Log(x any) (float64, error) {
	fx, err := parseFloat(x)
	if err != nil {
		return 0, err
	}
	if fx = math.Log(fx); isValidFloat(fx) {
		return fx, nil
	}
	return 0, errFloatResult
}

// Log10 returns the decimal logarithm of x
func Log10(x any) (float64, error) {
	fx, err := parseFloat(x)
	if err != nil {
		return 0, err
	}
	if fx = math.Log10(fx); isValidFloat(fx) {
		return fx, nil
	}
	return 0, errFloatResult
}

// Pow returns x**y, the base-x exponential of y
func Pow(x, y any) (float64, error) {
	fx, err := parseFloat(x)
	if err != nil {
		return 0, err
	}
	fy, err := parseFloat(y)
	if err != nil {
		return 0, err
	}
	if fx = math.Pow(fx, fy); isValidFloat(fx) {
		return fx, nil
	}
	return 0, errFloatResult
}

// Round returns the nearest integer, rounding half away from zero
func Round(x any) (int64, error) {
	fx, err := parseFloat(x)
	if err != nil {
		return 0, err
	}
	if fx = math.Round(fx); isValidFloat(fx) {
		return int64(fx), nil
	}
	return 0, errFloatResult
}

// Sqrt returns the square root of x
func Sqrt(x any) (float64, error) {
	fx, err := parseFloat(x)
	if err != nil {
		return 0, err
	}
	if fx = math.Sqrt(fx); isValidFloat(fx) {
		return fx, nil
	}
	return 0, errFloatResult
}

func SqrtDecimal(str string) decimal.Decimal {
	y, _ := decimal.NewFromString(str)

	var z decimal.Decimal
	var three = decimal.NewFromInt(3)
	var two = decimal.NewFromInt(2)
	var one = decimal.NewFromInt(1)
	if y.GreaterThan(three) {
		z = y
		x := y.Div(two).Add(one)
		for x.LessThan(z) {
			z = x
			x = (y.Div(x).Add(x)).Div(two)
		}
	} else if !y.IsZero() {
		z = one
	}
	return z
}

// parseInteger converts an int64, a money value or a numeric string to a big integer.
// Fractions are rejected, so the integer functions below never round their input.
func parseInteger(x any) (*big.Int, error) {
	var d decimal.Decimal
	switch v := x.(type) {
	case int64:
		return big.NewInt(v), nil
	case decimal.Decimal:
		d = v
	case string:
		var err error
		if d, err = decimal.NewFromString(v); err != nil {
			return nil, errInteger
		}
	default:
		return nil, errInteger
	}
	if !d.Equal(d.Truncate(0)) {
		return nil, errInteger
	}
	return d.BigInt(), nil
}

// ISqrt returns the integer square root of x, the greatest integer whose square
// does not exceed x. The result is exact for integers of any size.
func ISqrt(x any) (decimal.Decimal, error) {
	n, err := parseInteger(x)
	if err != nil {
		return decimal.Zero, err
	}
	if n.Sign() < 0 {
		return decimal.Zero, errNegativeSqrt
	}
	return decimal.NewFromBigInt(new(big.Int).Sqrt(n), 0), nil
}

// MulDiv returns floor(a*b/c) computed on integers. Unlike the money division
// operator, which rounds the quotient to 16 decimal places before it is floored,
// the result always equals the exact integer quotient.
func MulDiv(a, b, c any) (decimal.Decimal, error) {
	x, err := parseInteger(a)
	if err != nil {
		return decimal.Zero, err
	}
	y, err := parseInteger(b)
	if err != nil {
		return decimal.Zero, err
	}
	z, err := parseInteger(c)
	if err != nil {
		return decimal.Zero, err
	}
	if z.Sign() == 0 {
		return decimal.Zero, errIntegerDivZero
	}
	x.Mul(x, y)
	// DivMod is Euclidean division; it equals floor division for a positive
	// divisor and is one above it when a negative divisor leaves a remainder.
	q, m := new(big.Int).DivMod(x, z, new(big.Int))
	if m.Sign() != 0 && z.Sign() < 0 {
		q.Sub(q, big.NewInt(1))
	}
	return decimal.NewFromBigInt(q, 0), nil
}

func Div(str1 string, str2 string) string {
	x, _ := decimal.NewFromString(str1)
	y, _ := decimal.NewFromString(str2)
	return x.Div(y).String()
}

func GreaterThan(str1 string, str2 string) bool {
	x, _ := decimal.NewFromString(str1)
	y, _ := decimal.NewFromString(str2)

	return x.GreaterThan(y)
}

func GreaterThanOrEqual(str1 string, str2 string) bool {
	x, _ := decimal.NewFromString(str1)
	y, _ := decimal.NewFromString(str2)

	return x.GreaterThanOrEqual(y)
}

func LessThan(str1 string, str2 string) bool {
	x, _ := decimal.NewFromString(str1)
	y, _ := decimal.NewFromString(str2)

	return x.LessThan(y)
}

func LessThanOrEqual(str1 string, str2 string) bool {
	x, _ := decimal.NewFromString(str1)
	y, _ := decimal.NewFromString(str2)

	return x.LessThanOrEqual(y)
}
