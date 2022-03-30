/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"math"
	"strconv"
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
