/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/shopspring/decimal"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// FounderAllocation is the name of the allocation of a first block that names none: the founder
// takes consts.FounderAmount
const FounderAllocation = "founder"

var allocationName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// amountDigits is the precision of the amounts of the accounts, decimal(30)
const amountDigits = 30

// amountOf parses an amount of the first block: a positive integer of the smallest unit of at
// most amountDigits digits, written as decimal.Decimal writes it, so that a first block has a
// single form of each amount
func amountOf(s string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(s)
	if err != nil || d.String() != s || !d.IsInteger() || d.Sign() <= 0 || len(s) > amountDigits {
		return decimal.Decimal{}, fmt.Errorf("%q is not a positive integer of the smallest unit of at most %d digits", s, amountDigits)
	}
	return d, nil
}

// FirstBlockSupply is the supply of the coin of ecosystem 1 the first block fixes and the
// allocations it credits, checked: the supply is a positive amount; each allocation has a name
// of its own, of lower case letters, digits and _, and credits a positive amount to an account
// of its own; and they sum to at most the supply, the rest of which is not issued. A first block
// that names no supply has consts.TotalSupply, and one that allocates nothing credits
// consts.FounderAmount to the founder.
func FirstBlockSupply(data *types.FirstBlock) (decimal.Decimal, []types.Allocation, error) {
	supply := decimal.New(consts.TotalSupply, int32(consts.MoneyDigits))
	if data.Supply != "" {
		var err error
		if supply, err = amountOf(data.Supply); err != nil {
			return decimal.Decimal{}, nil, fmt.Errorf("the supply: %w", err)
		}
	}
	allocs := data.Allocations
	if len(allocs) == 0 {
		allocs = []types.Allocation{{Name: FounderAllocation, KeyID: data.KeyID,
			Amount: decimal.New(consts.FounderAmount, int32(consts.MoneyDigits)).String()}}
	}
	names, keys := map[string]bool{}, map[int64]bool{}
	sum := decimal.Zero
	for i, a := range allocs {
		if !allocationName.MatchString(a.Name) {
			return decimal.Decimal{}, nil, fmt.Errorf("allocation %d: the name %q is not 1 to 64 lower case letters, digits and _, from a letter", i, a.Name)
		}
		if names[a.Name] {
			return decimal.Decimal{}, nil, fmt.Errorf("allocation %s: another has its name", a.Name)
		}
		names[a.Name] = true
		if a.KeyID == 0 || converter.IDToAddress(a.KeyID) == `invalid` {
			return decimal.Decimal{}, nil, fmt.Errorf("allocation %s: %d is not an account", a.Name, a.KeyID)
		}
		if keys[a.KeyID] {
			return decimal.Decimal{}, nil, fmt.Errorf("allocation %s: another credits account %s", a.Name, converter.AddressToString(a.KeyID))
		}
		keys[a.KeyID] = true
		amount, err := amountOf(a.Amount)
		if err != nil {
			return decimal.Decimal{}, nil, fmt.Errorf("allocation %s: %w", a.Name, err)
		}
		sum = sum.Add(amount)
	}
	if sum.GreaterThan(supply) {
		return decimal.Decimal{}, nil, errors.New("the allocations sum to more than the supply")
	}
	return supply, allocs, nil
}
