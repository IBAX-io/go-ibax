/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// TestValidateWithoutHeaderKey covers a transaction whose header carries no public key: it is
// checked with the key the account registered, and a transaction another key signs for the
// account names that key in its header.
func TestValidateWithoutHeaderKey(t *testing.T) {
	crypto.InitAsymAlgo(crypto.AsymAlgo_ECC_P256.String())
	crypto.InitHashAlgo(crypto.HashAlgo_SHA256.String())
	syspar.SetAccountAlgorithms(syspar.AccountAlgorithmSet{{Algo: crypto.AsymAlgo_ECC_P256}})
	defer syspar.SetAccountAlgorithms(nil)
	conf.Config.LocalConf.NetworkID = 1

	priv, key, err := crypto.GenAccountKey(crypto.AsymAlgo_ECC_P256)
	if err != nil {
		t.Fatal(err)
	}
	const eco = 1
	sign := func(header []byte, signedBy int64) *SmartTransactionParser {
		t.Helper()
		tx := &types.SmartTransaction{
			Header: &types.Header{
				EcosystemID: eco,
				KeyID:       key.Address(),
				NetworkID:   1,
				Time:        time.Now().Unix(),
				PublicKey:   header,
			},
			SignedBy: signedBy,
			UTXO:     &types.UTXO{ToID: key.Address(), Value: "1"},
		}
		p := &SmartTransactionParser{SmartContract: &smart.SmartContract{TxSmart: tx}}
		if p.Payload, err = tx.Marshal(); err != nil {
			t.Fatal(err)
		}
		p.Hash = crypto.DoubleHash(p.Payload)
		signature, err := crypto.NewAsymAlgo(key.Algo).Sign(priv, crypto.Hash(p.Hash))
		if err != nil {
			t.Fatal(err)
		}
		p.TxSignature = converter.EncodeLengthPlusData(signature)
		return p
	}

	if err := sign(key.Bytes(), 0).Validate(); err != nil {
		t.Fatalf("the key in the header: %v", err)
	}
	registeredKeys.Put(registeredKeyID{eco, key.Address()}, key.Bytes())
	if err := sign(nil, 0).Validate(); err != nil {
		t.Fatalf("no key in the header, the account registered one: %v", err)
	}
	if err := sign(nil, 1).Validate(); err == nil {
		t.Fatal("a transaction another key signs is taken without that key in the header")
	}
}
