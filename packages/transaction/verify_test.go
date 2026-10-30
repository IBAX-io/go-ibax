/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/vmihailenco/msgpack/v5"
)

// TestTransferSignatureIsCheckedOnEntry covers the two forms a transfer transaction travels in and
// the checks on each:
//
//   - the client frame (0x80) is validated as it is parsed, so a transaction whose signature does
//     not match the key in its header is refused there;
//   - the stored format a block keeps and nodes gossip (type byte 5/6) is read back without
//     validating anything, so Verify is what a node calls on that half before the queue. It must
//     refuse a stored transaction whose header key is not the key of the signer it names, and one
//     whose signature does not hold;
//   - the stored format carries the payload the client signed, the signature and the time the node
//     took the transaction, nothing else: the hash and the body the node runs are derived from the
//     payload, so another body under a signature is another payload, which the signature does not
//     cover.
func TestTransferSignatureIsCheckedOnEntry(t *testing.T) {
	var every syspar.AccountAlgorithmSet
	for _, a := range []crypto.AsymAlgo{crypto.AsymAlgo_ECC_P256, crypto.AsymAlgo_ECC_Secp256k1, crypto.AsymAlgo_SM2, crypto.AsymAlgo_MLDSA65, crypto.AsymAlgo_MLDSA87} {
		every = append(every, syspar.AccountAlgorithm{Algo: a})
	}
	syspar.SetAccountAlgorithms(every)
	defer syspar.SetAccountAlgorithms(nil)

	checked := map[byte]int{}
	for _, c := range loadTransfers(t) {
		crypto.InitAsymAlgo(c.Cryptoer)
		crypto.InitHashAlgo(c.Hasher)
		conf.Config.LocalConf.NetworkID = c.NetworkID

		data, err := hex.DecodeString(c.Data)
		if err != nil {
			t.Fatal(err)
		}
		client := &Transaction{}
		if err := client.UnmarshallClient(bytes.NewBuffer(bytes.Clone(data)), true); err != nil {
			t.Fatalf("%s: honest client transaction refused: %v", c.Cryptoer, err)
		}
		txType := client.Type()
		if txType != types.TransferSelfTxType && txType != types.UtxoTxType {
			continue
		}
		checked[txType]++
		stored := bytes.Clone(client.FullData)
		name := c.Cryptoer + "/" + txTypeName(txType)

		// The honest stored transaction passes the entry check.
		honest := &Transaction{}
		if err := honest.Unmarshall(bytes.NewBuffer(bytes.Clone(stored)), true); err != nil {
			t.Fatalf("%s: honest stored transaction refused at parse: %v", name, err)
		}
		if err := honest.Verify(); err != nil {
			t.Fatalf("%s: honest stored transaction refused by Verify: %v", name, err)
		}

		// The stored form holds what the client signed and nothing a node could run instead: the
		// payload, the signature and the time the node took it.
		var st storedTx
		if err := msgpack.Unmarshal(stored[1:], &st); err != nil {
			t.Fatalf("%s: decoding stored transaction: %v", name, err)
		}
		if !bytes.Equal(st.Payload, client.SmartContract().Payload) || !bytes.Equal(st.TxSignature, client.SmartContract().TxSignature) {
			t.Fatalf("%s: the stored transaction is not the payload and the signature the client sent", name)
		}

		// Another body under the same signature: the hash and the body the node runs follow the
		// payload, so the signature no longer holds and the entry check refuses it.
		var body types.SmartTransaction
		if err := msgpack.Unmarshal(st.Payload, &body); err != nil {
			t.Fatalf("%s: decoding the payload: %v", name, err)
		}
		body.KeyID++
		other := st
		if other.Payload, err = msgpack.Marshal(&body); err != nil {
			t.Fatal(err)
		}
		if err := parsedButRefused(stored[0], &other); err != nil {
			t.Errorf("%s: another body under the signature: %v", name, err)
		}

		// The signature itself is damaged, everything else is intact.
		damaged := st
		damaged.TxSignature = bytes.Clone(st.TxSignature)
		damaged.TxSignature[len(damaged.TxSignature)-1] ^= 0xff
		if err := parsedButRefused(stored[0], &damaged); err != nil {
			t.Errorf("%s: a damaged signature: %v", name, err)
		}

		// The same payload, signature and time in another encoding: a map that also carries a body
		// and a hash of its own, as the stored form before held them, and the stored form with a
		// byte after it. Each is refused as it is parsed, not read as the transaction it names.
		var swindle types.SmartTransaction
		if err := msgpack.Unmarshal(st.Payload, &swindle); err != nil {
			t.Fatalf("%s: decoding the payload: %v", name, err)
		}
		swindle.KeyID++
		asMap, err := msgpack.Marshal(map[string]any{
			"TxSmart": &swindle, "Hash": crypto.DoubleHash(st.Payload),
			"Payload": st.Payload, "TxSignature": st.TxSignature, "Timestamp": st.Timestamp,
		})
		if err != nil {
			t.Fatal(err)
		}
		for label, enc := range map[string][]byte{
			"a map carrying a body": asMap,
			"a byte after the form": append(bytes.Clone(stored[1:]), 0),
		} {
			err := (&Transaction{}).Unmarshall(bytes.NewBuffer(append([]byte{stored[0]}, enc...)), true)
			if !errors.Is(err, errNotStoredForm) {
				t.Errorf("%s: %s: got %v, want %v", name, label, err, errNotStoredForm)
			}
		}

		// The header names another signer while the key it carries does the signing: consistent, its
		// signature holds, and still not the key of the account it names. The parse cannot tell, the
		// entry check must.
		key, err := hex.DecodeString(c.PrivateKey)
		if err != nil {
			t.Fatalf("%s: decoding the key of the vector: %v", name, err)
		}
		impersonating, _, err := NewTransactionInProc(types.SmartTransaction{
			Header: &types.Header{
				EcosystemID: client.SmartContract().TxSmart.EcosystemID,
				KeyID:       1,
				Time:        time.Now().Unix(),
				NetworkID:   c.NetworkID,
			},
			TransferSelf: &types.TransferSelf{Source: "Account", Target: "UTXO", Value: "1"},
		}, key)
		if err != nil {
			t.Fatalf("%s: building the impersonating transaction: %v", name, err)
		}
		imp := &Transaction{}
		if err := imp.Unmarshall(bytes.NewBuffer(impersonating), true); err != nil {
			t.Fatalf("%s: the impersonating transaction refused at parse: %v", name, err)
		}
		if err := imp.Verify(); err == nil {
			t.Errorf("%s: a stored transaction whose header key is not that of the account it names passed Verify", name)
		}

		// The client frame with a damaged signature is refused as it is parsed, as before.
		garbled := bytes.Clone(data)
		for i := len(garbled) - 1; i >= 0 && i > len(garbled)-8; i-- {
			garbled[i] ^= 0xff
		}
		if err := (&Transaction{}).UnmarshallClient(bytes.NewBuffer(garbled), true); err == nil {
			t.Errorf("%s: client frame with a damaged signature was accepted", name)
		}
	}

	for _, want := range []byte{types.TransferSelfTxType, types.UtxoTxType} {
		if checked[want] == 0 {
			t.Fatalf("the vectors had no %s transaction to check", txTypeName(want))
		}
	}
}

// parsedButRefused encodes a stored transaction and reports an error unless the node parses it and
// its entry check refuses it
func parsedButRefused(txType byte, st *storedTx) error {
	body, err := msgpack.Marshal(st)
	if err != nil {
		return err
	}
	tx := &Transaction{}
	if err := tx.Unmarshall(bytes.NewBuffer(append([]byte{txType}, body...)), true); err != nil {
		return fmt.Errorf("refused at parse: %w", err)
	}
	if err := tx.Verify(); err == nil {
		return errors.New("passed Verify")
	}
	return nil
}

func txTypeName(b byte) string {
	switch b {
	case types.TransferSelfTxType:
		return "TransferSelf"
	case types.UtxoTxType:
		return "UTXO"
	case types.SmartContractTxType:
		return "SmartContract"
	}
	return "unknown"
}
