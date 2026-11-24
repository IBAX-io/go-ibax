/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"encoding/binary"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// Delayed is a delayed contract due at a height of an ordered chain, as its row in
// 1_delayed_contracts names it
type Delayed struct {
	ID       int64
	Contract string
	KeyID    int64
}

// DelayedHash is the hash of the system transaction of the delayed contract of row id at the
// height: no transaction a client signs has it, as a client's hash is that of its payload
func DelayedHash(height, id int64) []byte {
	buf := append([]byte("delayed"), make([]byte, 16)...)
	binary.BigEndian.PutUint64(buf[7:], uint64(height))
	binary.BigEndian.PutUint64(buf[15:], uint64(id))
	return crypto.DoubleHash(buf)
}

// NewDelayed is the system transaction that runs the delayed contract d in the block of the header
// on an ordered chain (direction A): made by the execution of the block on every node, never
// received, signed by no key. It runs with the key of the row, as the node signer (the address of
// the node key of the validator that proposed the block), at the time of the block.
func NewDelayed(header *types.BlockHeader, signer int64, d Delayed) *Transaction {
	// a row of a contract the chain does not have makes a transaction without one, which its check
	// discards: every node discards it alike
	var id int
	contract := smart.VMGetContract(script.GetVM(), d.Contract, 1)
	if contract != nil {
		id = int(contract.Info().ID)
	}
	sc := &smart.SmartContract{
		TxSmart: &types.SmartTransaction{
			Header: &types.Header{
				ID:          id,
				EcosystemID: 1,
				KeyID:       d.KeyID,
				Time:        header.Timestamp,
				NetworkID:   conf.Config.LocalConf.NetworkID,
			},
			SignedBy: signer,
			Params:   map[string]any{},
		},
		TxContract: contract,
		TxData:     map[string]any{},
		Hash:       DelayedHash(header.BlockId, d.ID),
		Timestamp:  header.Timestamp * 1000,
		System:     true,
	}
	return &Transaction{Inner: &SmartTransactionParser{SmartContract: sc}}
}
