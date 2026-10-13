/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/vmihailenco/msgpack/v5"
)

// TestTransferSignatureIsCheckedOnEntry covers the gap between the two halves of a transfer and
// the checks that close it:
//
//   - the client frame (0x80) is validated, so a transaction whose signature does not match the
//     key in its header is refused as it is parsed;
//   - the stored/internal format a block keeps and nodes gossip (type byte 5/6) is parsed without
//     validating anything. Verify is what a node calls on that half before the queue, and it must
//     refuse a transaction whose header key is not the key of the signer it names.
//
// The signature must also hold: a stored transfer with a rewritten KeyID and the old signature, or
// with a damaged signature, must not pass Verify, while the honest transaction must.
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

		// The honest stored transaction passes the entry check.
		honest := &Transaction{}
		if err := honest.Unmarshall(bytes.NewBuffer(bytes.Clone(stored)), true); err != nil {
			t.Fatalf("%s/%s: honest stored transaction refused at parse: %v", c.Cryptoer, txTypeName(txType), err)
		}
		if err := honest.Verify(); err != nil {
			t.Fatalf("%s/%s: honest stored transaction refused by Verify: %v", c.Cryptoer, txTypeName(txType), err)
		}

		// The KeyID is rewritten and the old signature, key and hash are kept.
		var p SmartTransactionParser
		if err := msgpack.Unmarshal(stored[1:], &p); err != nil {
			t.Fatalf("%s: decoding stored transaction: %v", c.Cryptoer, err)
		}
		originalKey := p.TxSmart.KeyID
		p.TxSmart.KeyID = originalKey + 1
		body, err := msgpack.Marshal(&p)
		if err != nil {
			t.Fatal(err)
		}
		tampered := append([]byte{stored[0]}, body...)
		impersonated := &Transaction{}
		if err := impersonated.Unmarshall(bytes.NewBuffer(tampered), true); err != nil {
			t.Fatalf("%s: tampered stored transaction refused at parse (parse should not verify): %v", c.Cryptoer, err)
		}
		if err := impersonated.Verify(); err == nil {
			t.Errorf("%s/%s: stored transaction with KeyID rewritten %d->%d and the old signature passed Verify",
				c.Cryptoer, txTypeName(txType), originalKey, p.TxSmart.KeyID)
		}

		// The signature itself is damaged, everything else is intact.
		if err := msgpack.Unmarshal(stored[1:], &p); err != nil {
			t.Fatalf("%s: decoding stored transaction: %v", c.Cryptoer, err)
		}
		if len(p.TxSignature) == 0 {
			t.Fatalf("%s/%s: stored transaction carries no signature", c.Cryptoer, txTypeName(txType))
		}
		p.TxSignature[len(p.TxSignature)-1] ^= 0xff
		body, err = msgpack.Marshal(&p)
		if err != nil {
			t.Fatal(err)
		}
		damaged := append([]byte{stored[0]}, body...)
		bad := &Transaction{}
		if err := bad.Unmarshall(bytes.NewBuffer(damaged), true); err != nil {
			t.Fatalf("%s: damaged stored transaction refused at parse: %v", c.Cryptoer, err)
		}
		if err := bad.Verify(); err == nil {
			t.Errorf("%s/%s: stored transaction with a damaged signature passed Verify", c.Cryptoer, txTypeName(txType))
		}

		// The client frame with a damaged signature is refused as it is parsed, as before.
		garbled := bytes.Clone(data)
		for i := len(garbled) - 1; i >= 0 && i > len(garbled)-8; i-- {
			garbled[i] ^= 0xff
		}
		if err := (&Transaction{}).Unmarshall(bytes.NewBuffer(garbled), true); err == nil {
			t.Errorf("%s: client frame with a damaged signature was accepted", c.Cryptoer)
		}
	}

	for _, want := range []byte{types.TransferSelfTxType, types.UtxoTxType} {
		if checked[want] == 0 {
			t.Fatalf("the vectors had no %s transaction to check", txTypeName(want))
		}
	}
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
