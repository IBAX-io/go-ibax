/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"bytes"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// On a network of ML-DSA-65 nodes, a transaction a node builds for an account (NewTransactionInProc)
// is signed with the algorithm of the account key: ML-DSA, P-256 and RSA accounts each carry their
// own key in the header and pass the entry check, and the private key used with the node
// algorithm makes no signature of the account.
func TestTransactionSignedWithTheAccountAlgorithm(t *testing.T) {
	crypto.InitAsymAlgo(crypto.AsymAlgo_MLDSA65.String())
	crypto.InitHashAlgo(crypto.HashAlgo_SHA256.String())
	conf.Config.LocalConf.NetworkID = 1
	algos := []crypto.AsymAlgo{crypto.AsymAlgo_MLDSA65, crypto.AsymAlgo_ECC_P256, crypto.AsymAlgo_RSA2048, crypto.AsymAlgo_RSA3072}
	var set syspar.AccountAlgorithmSet
	for _, a := range algos {
		set = append(set, syspar.AccountAlgorithm{Algo: a})
	}
	syspar.SetAccountAlgorithms(set)
	defer syspar.SetAccountAlgorithms(nil)

	for _, algo := range algos {
		raw, pub, err := crypto.GenAccountKey(algo)
		if err != nil {
			t.Fatal(err)
		}
		key, err := crypto.NewAccountPrivateKey(algo, raw)
		if err != nil {
			t.Fatal(err)
		}
		data, _, err := NewTransactionInProc(types.SmartTransaction{
			Header: &types.Header{
				EcosystemID: 1,
				KeyID:       pub.Address(),
				Time:        time.Now().Unix(),
				NetworkID:   conf.Config.LocalConf.NetworkID,
			},
			TransferSelf: &types.TransferSelf{Source: "Account", Target: "UTXO", Value: "1"},
		}, key)
		if err != nil {
			t.Fatalf("%s: building the transaction: %v", algo, err)
		}
		tx := &Transaction{}
		if err := tx.Unmarshall(bytes.NewBuffer(data), true); err != nil {
			t.Fatalf("%s: parsing the transaction: %v", algo, err)
		}
		if err := tx.Verify(); err != nil {
			t.Fatalf("%s: the transaction refused: %v", algo, err)
		}
		if got := tx.SmartContract().TxSmart.PublicKey; !bytes.Equal(got, pub.Bytes()) {
			t.Errorf("%s: the header carries %x, not the account key %s", algo, got, pub.Hex())
		}
		if algo == crypto.NodeAlgo() {
			continue
		}
		// Signed as a key of the node algorithm, the private key makes no signature of the account
		if sig, err := crypto.NodeAccountPrivateKey(raw).Sign(data); err == nil {
			if ok, _ := pub.Verify(data, sig); ok {
				t.Errorf("%s: a signature made with the node algorithm %s verified with the account key", algo, crypto.NodeAlgo())
			}
		}
	}
}
