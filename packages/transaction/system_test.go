/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"bytes"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// The hash of the system transaction of a delayed contract is of the height and the row: the same
// on every node, another for another height or row
func TestDelayedHash(t *testing.T) {
	crypto.InitHashAlgo(crypto.HashAlgo_SHA256.String())
	seen := map[string]bool{}
	for _, h := range []int64{1, 2, 256} {
		for _, id := range []int64{1, 2, 256} {
			hash := DelayedHash(h, id)
			if !bytes.Equal(hash, DelayedHash(h, id)) {
				t.Fatalf("height %d row %d: the hash changes", h, id)
			}
			if seen[string(hash)] {
				t.Fatalf("height %d row %d: the hash of another transaction", h, id)
			}
			seen[string(hash)] = true
		}
	}
}

// The system transaction of a delayed contract runs as the row's key, signed by the node it is
// given, at the time of the block, and its clones are system transactions too. A row of a contract
// the chain does not have makes a transaction without a contract, which its check discards.
func TestNewDelayed(t *testing.T) {
	crypto.InitHashAlgo(crypto.HashAlgo_SHA256.String())
	header := &types.BlockHeader{BlockId: 20, Timestamp: 1700000000, KeyId: 99}
	tx := NewDelayed(header, 42, Delayed{ID: 3, Contract: "@1NoSuchDelayed", KeyID: 7})
	if !tx.IsSmartContract() {
		t.Fatal("not a contract transaction")
	}
	for _, tr := range []*Transaction{tx, tx.Clone()} {
		s := tr.SmartContract()
		switch {
		case !s.System:
			t.Fatal("not a system transaction")
		case s.TxSmart.SignedBy != 42 || s.TxSmart.KeyID != 7 || s.TxSmart.EcosystemID != 1:
			t.Fatalf("signer %d, key %d, ecosystem %d", s.TxSmart.SignedBy, s.TxSmart.KeyID, s.TxSmart.EcosystemID)
		case s.TxSmart.Time != header.Timestamp || tr.Timestamp() != header.Timestamp*1000:
			t.Fatalf("time %d, timestamp %d", s.TxSmart.Time, tr.Timestamp())
		case !bytes.Equal(tr.Hash(), DelayedHash(20, 3)):
			t.Fatal("hash")
		case s.TxContract != nil || s.TxSmart.ID != 0:
			t.Fatal("a contract the chain does not have")
		case len(s.TxSignature) != 0 || len(s.Payload) != 0 || tr.FullData != nil:
			t.Fatal("a system transaction carries no signature nor stored form")
		}
	}
}
