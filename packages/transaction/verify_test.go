/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"bytes"
	"encoding/hex"
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
//   - the stored format carries the payload, the hash of that payload and the body the node runs as
//     fields of their own. The signature covers the hash, so a body the payload does not encode, or
//     a hash that is not the hash of the payload, is refused as the transaction is parsed: with the
//     three unrelated, anyone who saw a signed transaction could put a body of their own under its
//     hash and its signature, paying themselves with the coins of the account it names.
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
		if err := client.Unmarshall(bytes.NewBuffer(bytes.Clone(data)), true); err != nil {
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

		// Another body under the same payload, hash and signature. The body that arrives pays
		// another account; the payload, the hash and the signature are the ones the honest
		// transaction carries, so only the relation between the three can refuse it.
		var p SmartTransactionParser
		if err := msgpack.Unmarshal(stored[1:], &p); err != nil {
			t.Fatalf("%s: decoding stored transaction: %v", name, err)
		}
		originalKey := p.TxSmart.KeyID
		p.TxSmart.KeyID = originalKey + 1
		if err := unmarshallRefused(name, stored[0], &p, fmt.Sprintf("a body its payload does not encode (KeyID %d -> %d)", originalKey, p.TxSmart.KeyID)); err != nil {
			t.Error(err)
		}

		// Another hash over the same payload and signature.
		var h SmartTransactionParser
		if err := msgpack.Unmarshal(stored[1:], &h); err != nil {
			t.Fatalf("%s: decoding stored transaction: %v", name, err)
		}
		h.Hash[0] ^= 0xff
		if err := unmarshallRefused(name, stored[0], &h, "a hash that is not the hash of its payload"); err != nil {
			t.Error(err)
		}

		// The signature itself is damaged, everything else is intact: the parse is not what refuses
		// it, the entry check is.
		if err := msgpack.Unmarshal(stored[1:], &p); err != nil {
			t.Fatalf("%s: decoding stored transaction: %v", name, err)
		}
		if len(p.TxSignature) == 0 {
			t.Fatalf("%s: stored transaction carries no signature", name)
		}
		p.TxSignature[len(p.TxSignature)-1] ^= 0xff
		damaged, err := msgpack.Marshal(&p)
		if err != nil {
			t.Fatal(err)
		}
		bad := &Transaction{}
		if err := bad.Unmarshall(bytes.NewBuffer(append([]byte{stored[0]}, damaged...)), true); err != nil {
			t.Fatalf("%s: stored transaction with a damaged signature refused at parse: %v", name, err)
		}
		if err := bad.Verify(); err == nil {
			t.Errorf("%s: stored transaction with a damaged signature passed Verify", name)
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
		if err := (&Transaction{}).Unmarshall(bytes.NewBuffer(garbled), true); err == nil {
			t.Errorf("%s: client frame with a damaged signature was accepted", name)
		}
	}

	for _, want := range []byte{types.TransferSelfTxType, types.UtxoTxType} {
		if checked[want] == 0 {
			t.Fatalf("the vectors had no %s transaction to check", txTypeName(want))
		}
	}
}

// unmarshallRefused encodes the parser as a stored transaction and reports an error unless the node
// refuses it as it is parsed, which is what keeps a body apart from a payload that does not carry it.
func unmarshallRefused(name string, txType byte, p *SmartTransactionParser, what string) error {
	body, err := msgpack.Marshal(p)
	if err != nil {
		return fmt.Errorf("%s: encoding the transaction with %s: %w", name, what, err)
	}
	if err := (&Transaction{}).Unmarshall(bytes.NewBuffer(append([]byte{txType}, body...)), true); err == nil {
		return fmt.Errorf("%s: stored transaction with %s was accepted", name, what)
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
