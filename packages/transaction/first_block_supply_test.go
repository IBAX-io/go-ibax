/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// coins is n coins in the smallest unit
func coins(n int64) string { return decimal.New(n, 12).String() }

// supplyTestBlock is a first block of the supply 2,100,000,000 and its seven allocations, the
// rest, 624,750,000, not issued
func supplyTestBlock() *types.FirstBlock {
	crypto.InitHashAlgo(crypto.HashAlgo_KECCAK256.String())
	parts := []struct {
		name   string
		amount int64
	}{
		{"migration", 362250000}, {"assign", 168000000}, {"ecosystem", 210000000},
		{"liquidity", 105000000}, {"validators", 420000000}, {"foundation", 210000000},
	}
	fb := &types.FirstBlock{KeyID: crypto.AddressSeed("founder"), Supply: coins(2100000000)}
	for _, p := range parts {
		fb.Allocations = append(fb.Allocations, types.Allocation{Name: p.name, KeyID: crypto.AddressSeed(p.name), Amount: coins(p.amount)})
	}
	return fb
}

func TestFirstBlockSupply(t *testing.T) {
	fb := supplyTestBlock()
	supply, allocs, err := FirstBlockSupply(fb)
	if err != nil {
		t.Fatal(err)
	}
	sum := decimal.Zero
	for _, a := range allocs {
		sum = sum.Add(decimal.RequireFromString(a.Amount))
	}
	if got := supply.Sub(sum).String(); got != coins(624750000) {
		t.Fatalf("not issued: %s, want %s", got, coins(624750000))
	}
	if supply.String() != coins(2100000000) || len(allocs) != 6 {
		t.Fatalf("supply %s, %d allocations", supply, len(allocs))
	}

	// the allocations may take the whole supply, to the smallest unit
	fb.Allocations = append(fb.Allocations, types.Allocation{Name: "rest", KeyID: crypto.AddressSeed("rest"), Amount: coins(624750000)})
	if _, _, err := FirstBlockSupply(fb); err != nil {
		t.Fatalf("the whole supply: %v", err)
	}
	fb.Allocations[6].Amount = decimal.RequireFromString(coins(624750000)).Add(decimal.New(1, 0)).String()
	if _, _, err := FirstBlockSupply(fb); err == nil || !strings.Contains(err.Error(), "more than the supply") {
		t.Fatalf("a unit over the supply: %v", err)
	}
}

func TestFirstBlockSupplyDefault(t *testing.T) {
	crypto.InitHashAlgo(crypto.HashAlgo_KECCAK256.String())
	fb := &types.FirstBlock{KeyID: crypto.AddressSeed("founder")}
	supply, allocs, err := FirstBlockSupply(fb)
	if err != nil {
		t.Fatal(err)
	}
	want := []types.Allocation{{Name: FounderAllocation, KeyID: fb.KeyID, Amount: coins(5250000)}}
	if supply.String() != coins(2100000000) || len(allocs) != 1 || allocs[0] != want[0] {
		t.Fatalf("supply %s, allocations %v", supply, allocs)
	}
	if len(fb.Allocations) != 0 {
		t.Fatal("the default changed the first block")
	}
}

func TestFirstBlockSupplyRefuses(t *testing.T) {
	cases := []struct {
		name string
		edit func(*types.FirstBlock)
		want string
	}{
		{"supply of a fraction", func(fb *types.FirstBlock) { fb.Supply = "2100000000.5" }, "the supply"},
		{"supply of another form", func(fb *types.FirstBlock) { fb.Supply = "21e20" }, "the supply"},
		{"supply with zeros before", func(fb *types.FirstBlock) { fb.Supply = "0" + coins(2100000000) }, "the supply"},
		{"supply of zero", func(fb *types.FirstBlock) { fb.Supply = "0" }, "the supply"},
		{"supply of 31 digits", func(fb *types.FirstBlock) { fb.Supply = "1" + strings.Repeat("0", 30) }, "the supply"},
		{"no name", func(fb *types.FirstBlock) { fb.Allocations[0].Name = "" }, "the name"},
		{"name of capitals", func(fb *types.FirstBlock) { fb.Allocations[0].Name = "Migration" }, "the name"},
		{"name from a digit", func(fb *types.FirstBlock) { fb.Allocations[0].Name = "1migration" }, "the name"},
		{"name of 65", func(fb *types.FirstBlock) { fb.Allocations[0].Name = "m" + strings.Repeat("a", 64) }, "the name"},
		{"name twice", func(fb *types.FirstBlock) { fb.Allocations[1].Name = "migration" }, "another has its name"},
		{"account 0", func(fb *types.FirstBlock) { fb.Allocations[0].KeyID = 0 }, "is not an account"},
		{"account of a bad checksum", func(fb *types.FirstBlock) { fb.Allocations[0].KeyID = crypto.AddressSeed("x") + 1 }, "is not an account"},
		{"account twice", func(fb *types.FirstBlock) { fb.Allocations[1].KeyID = fb.Allocations[0].KeyID }, "another credits account"},
		{"amount of zero", func(fb *types.FirstBlock) { fb.Allocations[0].Amount = "0" }, "allocation migration"},
		{"amount below zero", func(fb *types.FirstBlock) { fb.Allocations[0].Amount = "-" + coins(1) }, "allocation migration"},
		{"amount of a fraction", func(fb *types.FirstBlock) { fb.Allocations[0].Amount = "1.5" }, "allocation migration"},
		{"empty amount", func(fb *types.FirstBlock) { fb.Allocations[0].Amount = "" }, "allocation migration"},
		{"more than the supply", func(fb *types.FirstBlock) { fb.Supply = coins(1475249999) }, "more than the supply"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fb := supplyTestBlock()
			c.edit(fb)
			if _, _, err := FirstBlockSupply(fb); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
		})
	}
}
