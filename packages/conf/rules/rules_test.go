/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package rules

import "testing"

func TestActiveOn(t *testing.T) {
	cases := []struct {
		network int64
		block   int64
		want    bool
	}{
		{testnet, 1728, false},
		{testnet, 1729, true},
		{mainnet, 1, true},
		{1, 1, true},
	}
	for _, c := range cases {
		if got := activeOn(c.network, TransferSelfHistory, c.block); got != c.want {
			t.Errorf("network %d block %d: got %v, want %v", c.network, c.block, got, c.want)
		}
	}
}

func TestGenesisHash(t *testing.T) {
	for _, network := range []int64{testnet, mainnet} {
		if h, ok := genesisHash(network); !ok || len(h) != 32 {
			t.Errorf("network %d: %x, %v", network, h, ok)
		}
	}
	if _, ok := genesisHash(1); ok {
		t.Error("network 1 has a first block kept")
	}
}
