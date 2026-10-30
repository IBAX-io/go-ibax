/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"maps"

	"github.com/IBAX-io/go-ibax/packages/smart"
)

// Clone is the transaction as the block holds it before it runs: what its parse set, and nothing a
// run sets. A block runs a transaction in parallel on a clone, and again on a clone when the run
// does not hold.
func (t *Transaction) Clone() *Transaction {
	c := &Transaction{FullData: t.FullData, Inner: t.Inner, packed: t.packed}
	s, ok := t.Inner.(*SmartTransactionParser)
	if !ok {
		return c
	}
	sc := &smart.SmartContract{
		TxSmart:     s.TxSmart,
		TxData:      maps.Clone(s.TxData),
		Hash:        s.Hash,
		Payload:     s.Payload,
		Timestamp:   s.Timestamp,
		TxSignature: s.TxSignature,
		TxSize:      s.TxSize,
		Size:        s.Size,
	}
	if s.TxContract != nil {
		sc.TxContract = &smart.Contract{Name: s.TxContract.Name, Block: s.TxContract.Block}
	}
	c.Inner = &SmartTransactionParser{SmartContract: sc, storedSize: s.storedSize}
	return c
}

// CheckLimits checks the limits of the block on a transaction that ran in parallel, in the order
// of the block: as its run in order checks them when it succeeds
func (t *Transaction) CheckLimits(limits *Limits) error {
	if s, ok := t.Inner.(*SmartTransactionParser); ok {
		return limits.CheckLimit(s)
	}
	return nil
}
