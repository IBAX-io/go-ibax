/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"bytes"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/transaction"
)

// TestSignedTransactionsPassTheNode checks that the transactions the bench signs, of every account
// algorithm, are those a node takes: it parses each as a node parses what a client sends, which
// checks the signature, and refuses each with a byte of its signature changed
func TestSignedTransactionsPassTheNode(t *testing.T) {
	var every syspar.AccountAlgorithmSet
	for _, a := range []crypto.AsymAlgo{crypto.AsymAlgo_ECC_P256, crypto.AsymAlgo_ECC_Secp256k1, crypto.AsymAlgo_SM2, crypto.AsymAlgo_MLDSA65, crypto.AsymAlgo_MLDSA87} {
		every = append(every, syspar.AccountAlgorithm{Algo: a})
	}
	syspar.SetAccountAlgorithms(every)
	defer syspar.SetAccountAlgorithms(nil)
	info := &chainInfo{NetworkID: 7, Cryptoer: "ECC_Secp256k1", Hasher: "KECCAK256"}
	if err := useChain(info); err != nil {
		t.Fatal(err)
	}
	conf.Config.LocalConf.NetworkID = info.NetworkID
	s := newSigner(info)

	for _, a := range every {
		accounts, err := genAccounts(a.Algo.String(), 2)
		if err != nil {
			t.Fatal(err)
		}
		// New accounts: the header carries the key, which a node checks without its database
		from, to := accounts[0], accounts[1]
		from.unregistered = true
		self, err := s.transferSelf(from, "1000", "UTXO", "Account")
		if err != nil {
			t.Fatal(err)
		}
		utxo, err := s.utxo(from, to.keyID, "1", "bench")
		if err != nil {
			t.Fatal(err)
		}
		for name, tx := range map[string]*signedTx{"TransferSelf": self, "UTXO": utxo} {
			parsed := &transaction.Transaction{}
			if err := parsed.UnmarshallClient(bytes.NewBuffer(bytes.Clone(tx.data)), true); err != nil {
				t.Fatalf("%s %s: refused: %v", a.Algo, name, err)
			}
			if got := parsed.Hash(); string(got) != string(mustHex(t, tx.hash)) {
				t.Fatalf("%s %s: the node's hash %x is not the bench's %s", a.Algo, name, got, tx.hash)
			}
			forged := bytes.Clone(tx.data)
			forged[len(forged)-1] ^= 1
			if err := (&transaction.Transaction{}).UnmarshallClient(bytes.NewBuffer(forged), true); err == nil {
				t.Fatalf("%s %s: a changed signature passed", a.Algo, name)
			}
		}
	}
}

// TestAccountsFileKeepsTheKeys checks that the accounts setup saves are those run loads
func TestAccountsFileKeepsTheKeys(t *testing.T) {
	accounts, err := genAccounts(crypto.AsymAlgo_MLDSA65.String(), 3)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "accounts.json")
	if err := saveAccounts(file, accounts); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadAccounts(file)
	if err != nil {
		t.Fatal(err)
	}
	for i := range accounts {
		if loaded[i].keyID != accounts[i].keyID || !bytes.Equal(loaded[i].public.Bytes(), accounts[i].public.Bytes()) {
			t.Fatalf("account %d: loaded %d, saved %d", i, loaded[i].keyID, accounts[i].keyID)
		}
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
