/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"bytes"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/transaction"
)

// TestSignerClock: each signature takes the time of the clock when it signs, as the node reads it,
// so that the transactions of a run longer than consts.MaxTxBack are not stamped with its start
func TestSignerClock(t *testing.T) {
	syspar.SetAccountAlgorithms(syspar.AccountAlgorithmSet{{Algo: crypto.AsymAlgo_ECC_P256}})
	defer syspar.SetAccountAlgorithms(nil)
	info := &chainInfo{NetworkID: 7, Cryptoer: "ECC_Secp256k1", Hasher: "KECCAK256"}
	if err := useChain(info); err != nil {
		t.Fatal(err)
	}
	conf.Config.LocalConf.NetworkID = info.NetworkID
	accounts, err := genAccounts(crypto.AsymAlgo_ECC_P256.String(), 2)
	if err != nil {
		t.Fatal(err)
	}
	accounts[0].unregistered = true
	now := int64(1_000)
	s := newSigner(info)
	s.clock = func() int64 { return now }
	for _, want := range []int64{1_000, 1_700} {
		now = want
		tx, err := s.utxo(accounts[0], accounts[1].keyID, "1", "bench")
		if err != nil {
			t.Fatal(err)
		}
		parsed := &transaction.Transaction{}
		if err := parsed.UnmarshallClient(bytes.NewBuffer(bytes.Clone(tx.data)), true); err != nil {
			t.Fatal(err)
		}
		if got := parsed.SignedTime(); got != want {
			t.Fatalf("header time %d, want %d", got, want)
		}
	}
}

// TestChainClock: the clock of a chain starts at its time, goes speed seconds a second, and stops
// at the system clock
func TestChainClock(t *testing.T) {
	sys := time.Now().Unix()
	if got := chainClock(sys-3_600, 1)(); got < sys-3_600 || got > sys-3_599 {
		t.Fatalf("speed 1: %d, want %d", got, sys-3_600)
	}
	c := chainClock(sys-10, 1e6)
	time.Sleep(10 * time.Millisecond)
	if got := c(); got != time.Now().Unix() {
		t.Fatalf("fast clock past the system clock: %d, want %d", got, time.Now().Unix())
	}
}
