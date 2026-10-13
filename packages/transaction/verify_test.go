/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"testing"

	"github.com/vmihailenco/msgpack/v5"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// The transfer transactions run no contract, so before Verify their signature was checked nowhere
// on the way in: the stored form a node queues, a block keeps and nodes gossip was parsed without
// a check, and its Action returns before the contract call that held the only execution-time
// check. A transaction whose header names an account its key did not sign for moved that account's
// coins.
//
// Verify is called when a transaction enters the node, before the queue, and must hold: the honest
// transfer of the key that signs it passes, and a transfer naming another account, or carrying a
// damaged signature, does not.
func TestVerifyRefusesTransferOfAKeyThatDidNotSign(t *testing.T) {
	crypto.InitAsymAlgo("ECC_Secp256k1")
	crypto.InitHashAlgo("KECCAK256")
	previous := conf.Config.LocalConf.NetworkID
	conf.Config.LocalConf.NetworkID = 1
	defer func() { conf.Config.LocalConf.NetworkID = previous }()

	private, public, err := crypto.GenKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	keyID := crypto.Address(public)
	if keyID == 0 {
		t.Fatal("the generated key has the null address")
	}

	kinds := []struct {
		name string
		fill func(*types.SmartTransaction)
	}{
		{"TransferSelf", func(tx *types.SmartTransaction) {
			tx.TransferSelf = &types.TransferSelf{Value: "1", Source: "UTXO", Target: "Account"}
		}},
		{"UTXO", func(tx *types.SmartTransaction) {
			tx.UTXO = &types.UTXO{ToID: keyID, Value: "1"}
		}},
	}

	for _, kind := range kinds {
		// build makes the stored form of the transfer, as a node keeps it: the header is the one
		// given and the transaction is signed by private.
		build := func(header types.Header) []byte {
			tx := types.SmartTransaction{Header: &header}
			kind.fill(&tx)
			data, _, err := NewTransactionInProc(tx, private)
			if err != nil {
				t.Fatalf("%s: building the transaction: %v", kind.name, err)
			}
			return data
		}
		verify := func(data []byte) error {
			tr := &Transaction{}
			if err := tr.Unmarshall(bytes.NewBuffer(bytes.Clone(data)), true); err != nil {
				t.Fatalf("%s: parsing the stored transaction: %v", kind.name, err)
			}
			return tr.Verify()
		}

		honest := types.Header{KeyID: keyID, EcosystemID: 1, NetworkID: conf.Config.LocalConf.NetworkID}
		if err := verify(build(honest)); err != nil {
			t.Errorf("%s: the transfer of the key that signs it was refused: %v", kind.name, err)
		}

		// The header names the next account while the same key signs and the same signature stays:
		// what a party on the node's API or port sends to move another account's coins.
		forged := honest
		forged.KeyID = keyID + 1
		stored := build(forged)
		if err := verify(stored); err == nil {
			t.Errorf("%s: a transfer naming key %d, signed by key %d, passed Verify", kind.name, forged.KeyID, keyID)
		}

		// The signature itself is damaged; everything else is intact.
		var parser SmartTransactionParser
		if err := msgpack.Unmarshal(stored[1:], &parser); err != nil {
			t.Fatalf("%s: decoding the stored transaction: %v", kind.name, err)
		}
		if len(parser.TxSignature) == 0 {
			t.Fatalf("%s: the stored transaction carries no signature", kind.name)
		}
		parser.TxSignature[len(parser.TxSignature)-1] ^= 0xff
		body, err := msgpack.Marshal(&parser)
		if err != nil {
			t.Fatal(err)
		}
		if err := verify(append([]byte{stored[0]}, body...)); err == nil {
			t.Errorf("%s: a transfer with a damaged signature passed Verify", kind.name)
		}
	}
}
