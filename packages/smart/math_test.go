/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestISqrt(t *testing.T) {
	cases := []struct {
		in  any
		out string
	}{
		{int64(0), "0"},
		{int64(1), "1"},
		{int64(3), "1"},
		{int64(4), "2"},
		{int64(99), "9"},
		{"1000000000000000000000000000000000000", "1000000000000000000"},
		{"999999999999999999999999999999999999", "999999999999999999"},
		{decimal.RequireFromString("340282366920938463463374607431768211455"), "18446744073709551615"},
	}
	for _, c := range cases {
		got, err := ISqrt(c.in)
		require.NoError(t, err)
		require.Equal(t, c.out, got.String(), "ISqrt(%v)", c.in)
	}
	_, err := ISqrt(int64(-1))
	require.ErrorIs(t, err, errNegativeSqrt)
	_, err = ISqrt("2.5")
	require.ErrorIs(t, err, errInteger)
	_, err = ISqrt(1.5)
	require.ErrorIs(t, err, errInteger)

	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		n := new(big.Int).Rand(rnd, new(big.Int).Lsh(big.NewInt(1), uint(1+rnd.Intn(200))))
		got, err := ISqrt(decimal.NewFromBigInt(n, 0))
		require.NoError(t, err)
		r := got.BigInt()
		next := new(big.Int).Add(r, big.NewInt(1))
		require.True(t, new(big.Int).Mul(r, r).Cmp(n) <= 0, "root too big for %s", n)
		require.True(t, new(big.Int).Mul(next, next).Cmp(n) > 0, "root too small for %s", n)
	}
}

func TestMulDiv(t *testing.T) {
	cases := []struct {
		a, b, c any
		out     string
	}{
		{int64(7), int64(1), int64(2), "3"},
		{int64(-7), int64(1), int64(2), "-4"},
		{int64(7), int64(1), int64(-2), "-4"},
		{int64(6), int64(1), int64(-2), "-3"},
		{"10000000000000000000000000000", "3", "10000000000000000000000000001", "2"},
		// The money division operator returns 1 here because the quotient
		// 0.99999999999999999999 rounds up at 16 decimal places.
		{"99999999999999999999", int64(1), "100000000000000000000", "0"},
	}
	for _, c := range cases {
		got, err := MulDiv(c.a, c.b, c.c)
		require.NoError(t, err)
		require.Equal(t, c.out, got.String(), "MulDiv(%v, %v, %v)", c.a, c.b, c.c)
	}
	_, err := MulDiv(int64(1), int64(1), int64(0))
	require.ErrorIs(t, err, errIntegerDivZero)
	_, err = MulDiv("1.5", int64(1), int64(1))
	require.ErrorIs(t, err, errInteger)

	rnd := rand.New(rand.NewSource(2))
	for i := 0; i < 2000; i++ {
		a := new(big.Int).Rand(rnd, new(big.Int).Lsh(big.NewInt(1), 128))
		b := new(big.Int).Rand(rnd, new(big.Int).Lsh(big.NewInt(1), 128))
		c := new(big.Int).Add(new(big.Int).Rand(rnd, new(big.Int).Lsh(big.NewInt(1), 160)), big.NewInt(1))
		got, err := MulDiv(decimal.NewFromBigInt(a, 0), decimal.NewFromBigInt(b, 0), decimal.NewFromBigInt(c, 0))
		require.NoError(t, err)
		want := new(big.Int).Quo(new(big.Int).Mul(a, b), c)
		require.Equal(t, want.String(), got.String())
	}
}
