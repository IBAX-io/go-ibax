/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package rules says which version of an execution rule applies to a block.
//
// The blocks of a network are executed again by every node that syncs it, so a change to how
// blocks are executed must leave the blocks made before it as they were. Each such change is a
// Rule; on the networks whose history predates it, it takes effect from a fixed block, kept here
// per network ID. On any other network every rule applies from the first block.
package rules

import (
	"encoding/hex"

	"github.com/IBAX-io/go-ibax/packages/conf"
)

// Rule is a change to block execution.
type Rule int

const (
	// TransferSelfHistory: TransferSelf writes a 1_history row, and a transaction that is not a
	// contract is logged in ecosystem 1 rather than 0 (#2064).
	TransferSelfHistory Rule = iota
	// ByteRollbackHexTwice: the rollback data of a bytea column holds the hex of the column's hex
	// rather than its hex, since a row is read with its bytea columns in hex (630191f5).
	ByteRollbackHexTwice
	// ByteRollbackHexOnce: the rollback data of a bytea column holds the column's hex again, the hex
	// the row is read in no longer being encoded a second time (f1a3650c).
	ByteRollbackHexOnce
	// FeeByDigits: fees are counted in the digits of the paying ecosystem rather than through
	// price_create_rate and have no element fee; a contract's base cost is charged once per call,
	// and a run that goes over its cost limit fails (6729de64..d967f160).
	FeeByDigits
	// JSONColumnsSorted: the jsonb columns of a statement are written in sorted order rather than in
	// no fixed order, which only the rollback rows a block carries record (d967f160).
	JSONColumnsSorted
	// UTXOFeeParam: a UTXO transfer in an ecosystem other than 1 pays that ecosystem's fee only when
	// its utxo_fee parameter is 1, and burns its combustion percent whatever its combustion flag,
	// rather than paying whenever the ecosystem has a fuel rate and burning only with flag 2 (#2058).
	UTXOFeeParam
	// TeEmissionAccess: the first block gives @1TeEmission access to the emission of the
	// ecosystems table (#2072).
	TeEmissionAccess
	// DigitsAccess: the first block gives @1NewToken access to the digits of the ecosystems table
	// (#2133).
	DigitsAccess
)

const (
	testnet = 5
	mainnet = 7
)

// activation holds, per network ID, the first block each rule applies to.
var activation = map[int64]map[Rule]int64{
	testnet: {
		// TransferSelf transactions end at block 1609; any block in (1609, 2545] fits.
		TransferSelfHistory: 1729,
		// the last bytea rollback data in single hex is at block 1764, the first in double hex at
		// block 2657; any block in (1764, 2657] fits.
		ByteRollbackHexTwice: 2657,
		// the last bytea rollback data in double hex is at block 2755; any block in (2755, 3055] fits.
		ByteRollbackHexOnce: 3055,
		// block 2560 was made before d967f160, block 2561 after.
		FeeByDigits: 2561,
		// block 2560 was made before d967f160, block 2561 after.
		JSONColumnsSorted: 2561,
		// UTXO transfers in ecosystems other than 1 end at block 1091; any block in (1091, 2425] fits.
		UTXOFeeParam: 2425,
		// these concern the first block only, which predates them.
		TeEmissionAccess: 2,
		DigitsAccess:     2,
	},
	mainnet: {
		// mainnet's first block (2022-11-13 11:03 UTC) was made after #2058 and #2064 and before
		// #2072; it has no UTXO transfers outside ecosystem 1 and no bytea rollback data up to block
		// 20577.
		// block 16992 was made before d967f160, block 16993 (2023-06-09) after.
		FeeByDigits: 16993,
		// block 16992 was made before d967f160, block 16993 after.
		JSONColumnsSorted: 16993,
		// mainnet carries no bytea rollback data; it is never double hex.
		ByteRollbackHexTwice: 16993,
		ByteRollbackHexOnce:  16993,
		// these concern the first block only, which predates them.
		TeEmissionAccess: 2,
		DigitsAccess:     2,
	},
}

// genesis holds, per network ID, the hash of the network's first block.
var genesis = map[int64]string{
	testnet: "cbd5a088106f238487022defee4ef77dbfe1d376e9e206c1b0525a539e4340a5",
	mainnet: "5f0c4ffab6e8821bb99327d8174c4a2478dc088eb9ee6220aeeb4d5c5e8de0d8",
}

// GenesisHash returns the hash of the first block of this node's network, and false on a network
// whose first block is not kept here.
func GenesisHash() ([]byte, bool) {
	return genesisHash(conf.Config.LocalConf.NetworkID)
}

func genesisHash(networkID int64) ([]byte, bool) {
	h, ok := genesis[networkID]
	if !ok {
		return nil, false
	}
	b, _ := hex.DecodeString(h)
	return b, true
}

// Active reports whether rule r applies to block blockID of this node's network.
func Active(r Rule, blockID int64) bool {
	return activeOn(conf.Config.LocalConf.NetworkID, r, blockID)
}

func activeOn(networkID int64, r Rule, blockID int64) bool {
	from, ok := activation[networkID][r]
	return !ok || blockID >= from
}
