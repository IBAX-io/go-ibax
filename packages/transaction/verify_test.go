/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"strings"
	"testing"
	"time"

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
//
// The stored form carries the payload, the hash of that payload and the transaction the node runs
// as fields of their own, and the signature covers the hash. A body the payload does not encode, or
// a hash that is not the hash of the payload, must not be parsed at all: with the three unrelated,
// a party who saw a signed transaction could keep its payload, hash and signature, send a body of
// their own, and have the coins of the account the header names pay them.
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
		// unmarshall is what a node makes of the form of a stored transaction, before it asks who
		// signed it
		unmarshall := func(data []byte) error {
			return (&Transaction{}).Unmarshall(bytes.NewBuffer(bytes.Clone(data)), true)
		}
		verify := func(data []byte) error {
			tr := &Transaction{}
			if err := tr.Unmarshall(bytes.NewBuffer(bytes.Clone(data)), true); err != nil {
				t.Fatalf("%s: parsing the stored transaction: %v", kind.name, err)
			}
			return tr.Verify()
		}
		// tampered replaces one field of the stored transaction and keeps everything else, as a
		// party who saw it can
		tampered := func(data []byte, change func(*SmartTransactionParser)) []byte {
			var parser SmartTransactionParser
			if err := msgpack.Unmarshal(data[1:], &parser); err != nil {
				t.Fatalf("%s: decoding the stored transaction: %v", kind.name, err)
			}
			change(&parser)
			body, err := msgpack.Marshal(&parser)
			if err != nil {
				t.Fatal(err)
			}
			return append([]byte{data[0]}, body...)
		}

		honest := types.Header{KeyID: keyID, EcosystemID: 1, NetworkID: conf.Config.LocalConf.NetworkID}
		signed := build(honest)
		if err := verify(signed); err != nil {
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

		// Another body under the payload, the hash and the signature of the signed transfer: what a
		// party who saw the transfer sends to have the coins of the account it names pay them. The
		// three are the ones the key signed, so the form itself must refuse the transaction.
		swapped := tampered(signed, func(p *SmartTransactionParser) { p.TxSmart.KeyID++ })
		if err := unmarshall(swapped); err == nil {
			t.Errorf("%s: a stored transaction carrying a body its payload does not encode was accepted", kind.name)
		}

		// Another hash over the payload and the signature.
		if err := unmarshall(tampered(signed, func(p *SmartTransactionParser) { p.Hash[0] ^= 0xff })); err == nil {
			t.Errorf("%s: a stored transaction whose hash is not the hash of its payload was accepted", kind.name)
		}

		// The signature itself is damaged; everything else is intact.
		var parser SmartTransactionParser
		if err := msgpack.Unmarshal(signed[1:], &parser); err != nil {
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
		if err := verify(append([]byte{signed[0]}, body...)); err == nil {
			t.Errorf("%s: a transfer with a damaged signature passed Verify", kind.name)
		}
	}
}

// A contract call travels in the same stored fields as a transfer, and a signature covers only the
// hash of the payload, so a call is held to the same relation: its body has to be the one the
// payload encodes. A party who saw a call could otherwise keep its payload, hash and signature, put
// a body of their own under them, and have the account the header names run a contract it never
// asked for.
//
// The honest half of the relation is not asserted here: the parse of a call goes on to resolve the
// contract the header names, which takes a chain a unit test has none of. What is asserted is what
// the binding is for -- a body the payload does not encode is refused as the call is parsed, and
// refused for that reason, before anything looks for the contract.
func TestVerifyRefusesContractCallOfABodyItsPayloadDoesNotEncode(t *testing.T) {
	crypto.InitAsymAlgo("ECC_Secp256k1")
	crypto.InitHashAlgo("KECCAK256")
	previous := conf.Config.LocalConf.NetworkID
	conf.Config.LocalConf.NetworkID = 1
	defer func() { conf.Config.LocalConf.NetworkID = previous }()

	private, _, err := crypto.GenKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	stored, _, err := NewTransactionInProc(types.SmartTransaction{
		Header: &types.Header{
			ID:          1,
			EcosystemID: 1,
			KeyID:       1,
			Time:        time.Now().Unix(),
			NetworkID:   conf.Config.LocalConf.NetworkID,
		},
		Params: map[string]any{"Name": "honest"},
	}, private)
	if err != nil {
		t.Fatalf("building the contract call: %v", err)
	}
	if stored[0] != types.SmartContractTxType {
		t.Fatalf("the transaction is of type %d, not a contract call", stored[0])
	}

	// The parameters the call runs with change; the payload, the hash and the signature stay the
	// ones the key signed.
	var parser SmartTransactionParser
	if err := msgpack.Unmarshal(stored[1:], &parser); err != nil {
		t.Fatalf("decoding the stored transaction: %v", err)
	}
	if parser.TxSmart == nil {
		t.Fatal("the stored transaction carries no body")
	}
	params := parser.TxSmart.Params
	if params == nil {
		params = map[string]any{}
	}
	params["Name"] = "another"
	params["Amount"] = "1000000000000000000"
	parser.TxSmart.Params = params

	body, err := msgpack.Marshal(&parser)
	if err != nil {
		t.Fatal(err)
	}
	err = (&Transaction{}).Unmarshall(bytes.NewBuffer(append([]byte{stored[0]}, body...)), true)
	if err == nil {
		t.Errorf("a contract call carrying a body its payload does not encode was accepted")
	} else if !strings.Contains(err.Error(), "a body its payload does not encode") {
		t.Errorf("the call was refused for another reason than its body: %v", err)
	}
}
