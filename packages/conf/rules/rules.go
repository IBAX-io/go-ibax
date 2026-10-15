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

import "github.com/IBAX-io/go-ibax/packages/conf"

// Rule is a change to block execution.
type Rule int

const (
	// TransferSelfHistory: TransferSelf writes a 1_history row, and a transaction that is not a
	// contract is logged in ecosystem 1 rather than 0 (#2064).
	TransferSelfHistory Rule = iota
	// ByteRollbackRaw: the rollback data of a bytea column holds the column's bytes as they are
	// rather than their hex (f1a3650c).
	ByteRollbackRaw
	// FeeByDigits: fees are counted in the digits of the paying ecosystem rather than through
	// price_create_rate and have no element fee; a contract's base cost is charged once per call,
	// and a run that goes over its cost limit fails (6729de64..d967f160).
	FeeByDigits
	// JSONColumnsSorted: the jsonb columns of a statement are written in sorted order rather than in
	// no fixed order, which only the rollback rows a block carries record (d967f160).
	JSONColumnsSorted
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
		// the last bytea rollback data in hex is at block 2755; any block in (2755, 3055] fits.
		ByteRollbackRaw: 3055,
		// block 2560 was made before d967f160, block 2561 after.
		FeeByDigits: 2561,
		// block 2560 was made before d967f160, block 2561 after.
		JSONColumnsSorted: 2561,
	},
	mainnet: {},
}

// Active reports whether rule r applies to block blockID of this node's network.
func Active(r Rule, blockID int64) bool {
	return activeOn(conf.Config.LocalConf.NetworkID, r, blockID)
}

func activeOn(networkID int64, r Rule, blockID int64) bool {
	from, ok := activation[networkID][r]
	return !ok || blockID >= from
}
