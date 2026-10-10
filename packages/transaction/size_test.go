/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/vmihailenco/msgpack/v5"
)

type transferCase struct {
	Cryptoer, Hasher, Data string
	NetworkID              int64
}

// The signed client transactions of the cross-implementation vectors, on every suite
func loadTransfers(t *testing.T) []transferCase {
	t.Helper()
	raw, err := os.ReadFile("../../tools/cryptovectors/testdata/go-ibax-transfers.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []struct {
			transferCase
			Tampered bool
			Node     struct{ Error string }
		}
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	var cases []transferCase
	for _, c := range f.Cases {
		if !c.Tampered && c.Node.Error == "" {
			cases = append(cases, c.transferCase)
		}
	}
	if len(cases) == 0 {
		t.Fatal("no valid transfers in the vectors")
	}
	return cases
}

// A transaction's size is the length of the bytes a block stores for it, whether it arrives from a
// client or is read back from a block, and it is not part of those bytes. max_tx_size applies to it.
func TestTxSizeIsStoredLength(t *testing.T) {
	var every syspar.AccountAlgorithmSet
	for _, a := range []crypto.AsymAlgo{crypto.AsymAlgo_ECC_P256, crypto.AsymAlgo_ECC_Secp256k1, crypto.AsymAlgo_SM2, crypto.AsymAlgo_MLDSA65, crypto.AsymAlgo_MLDSA87} {
		every = append(every, syspar.AccountAlgorithm{Algo: a})
	}
	syspar.SetAccountAlgorithms(every)
	defer syspar.SetAccountAlgorithms(nil)
	for _, c := range loadTransfers(t) {
		crypto.InitAsymAlgo(c.Cryptoer)
		crypto.InitHashAlgo(c.Hasher)
		conf.Config.LocalConf.NetworkID = c.NetworkID
		data, err := hex.DecodeString(c.Data)
		if err != nil {
			t.Fatal(err)
		}
		client := &Transaction{}
		if err := client.Unmarshall(bytes.NewBuffer(data), true); err != nil {
			t.Fatalf("%s/%s: %v", c.Cryptoer, c.Hasher, err)
		}
		if len(client.FullData) <= len(data) {
			t.Fatalf("%s/%s: stored transaction is %d bytes, the client sent %d", c.Cryptoer, c.Hasher, len(client.FullData), len(data))
		}
		if got := client.Inner.txSize(); got != int64(len(client.FullData)) {
			t.Fatalf("%s/%s: size %d, stored %d bytes", c.Cryptoer, c.Hasher, got, len(client.FullData))
		}

		stored := &Transaction{}
		if err := stored.Unmarshall(bytes.NewBuffer(bytes.Clone(client.FullData)), true); err != nil {
			t.Fatalf("%s/%s: reading the stored transaction: %v", c.Cryptoer, c.Hasher, err)
		}
		if got := stored.Inner.txSize(); got != int64(len(client.FullData)) {
			t.Fatalf("%s/%s: stored transaction size %d, want %d", c.Cryptoer, c.Hasher, got, len(client.FullData))
		}
		before, err := msgpack.Marshal(stored.SmartContract())
		if err != nil {
			t.Fatal(err)
		}
		stored.Inner.setTxSize(1 << 40)
		after, err := msgpack.Marshal(stored.SmartContract())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("%s/%s: the size is part of the stored encoding", c.Cryptoer, c.Hasher)
		}

		size := int64(len(client.FullData))
		if err := (&txMaxSize{LimitTx: size}).check(client.Inner, letPreprocess); err != nil {
			t.Fatalf("%s/%s: refused at max_tx_size %d: %v", c.Cryptoer, c.Hasher, size, err)
		}
		if err := (&txMaxSize{LimitTx: size - 1}).check(client.Inner, letGenBlock); err == nil {
			t.Fatalf("%s/%s: accepted at max_tx_size %d", c.Cryptoer, c.Hasher, size-1)
		}
	}
}

// A transaction signed with a key of an algorithm the network does not take is refused before the
// queue: it could never run
func TestQueueRefusesAccountAlgorithm(t *testing.T) {
	defer syspar.SetAccountAlgorithms(nil)
	for _, c := range loadTransfers(t) {
		crypto.InitAsymAlgo(c.Cryptoer)
		crypto.InitHashAlgo(c.Hasher)
		conf.Config.LocalConf.NetworkID = c.NetworkID
		data, err := hex.DecodeString(c.Data)
		if err != nil {
			t.Fatal(err)
		}
		algo := crypto.AsymAlgo(crypto.AsymAlgo_value[c.Cryptoer])
		other := crypto.AsymAlgo_MLDSA87
		if algo == other {
			other = crypto.AsymAlgo_ECC_P256
		}
		for _, set := range []syspar.AccountAlgorithmSet{
			{{Algo: other}},
			{{Algo: algo, SignUntil: time.Unix(0, 0).UTC()}},
		} {
			syspar.SetAccountAlgorithms(set)
			if err := (&Transaction{}).Unmarshall(bytes.NewBuffer(bytes.Clone(data)), true); !errors.Is(err, syspar.ErrAccountAlgorithm) {
				t.Errorf("%s/%s with %s: %v", c.Cryptoer, c.Hasher, set, err)
			}
		}
	}
}
