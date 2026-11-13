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

// Head is the first transactions in the order of the blocks, at most the count and the bytes
func TestPoolHead(t *testing.T) {
	p := NewPool()
	if got := p.Head(10, 1000); len(got) != 0 {
		t.Fatalf("an empty pool gives %d", len(got))
	}
	p.put(poolTx("late", types.SmartContractTxType, 20, 0), RateClient, false, nil)
	p.put(poolTx("early", types.SmartContractTxType, 10, 0), RateClient, false, nil)
	p.put(poolTx("expedite", types.SmartContractTxType, 30, 5), RateClient, true, nil)
	p.put(poolTx("on-block", types.SmartContractTxType, 40, 0), RateOnBlock, false, nil)
	want := []string{"data of on-block", "data of expedite", "data of early", "data of late"}
	got := p.Head(10, 1000)
	if len(got) != len(want) {
		t.Fatalf("head %q", got)
	}
	for i := range want {
		if string(got[i]) != want[i] {
			t.Fatalf("head %q, want %q", got, want)
		}
	}
	if got := p.Head(2, 1000); len(got) != 2 || string(got[1]) != want[1] {
		t.Fatalf("2 of the head: %q", got)
	}
	// the first two take 16 + 16 bytes: the third of 13 is past 40
	if got := p.Head(10, 40); len(got) != 2 {
		t.Fatalf("40 bytes of the head: %q", got)
	}
	if got := p.Head(0, 1000); len(got) != 0 {
		t.Fatalf("0 of the head: %q", got)
	}
}

// Receive verifies no transaction the pool holds, nor one twice in a packet: a packet of a
// transaction whose signature does not hold is refused, but not if the pool holds its hash
func TestPoolReceiveSkipsHeld(t *testing.T) {
	c := loadTransfers(t)[0]
	syspar.SetAccountAlgorithms(syspar.AccountAlgorithmSet{{Algo: crypto.AsymAlgo(crypto.AsymAlgo_value[c.Cryptoer])}})
	defer syspar.SetAccountAlgorithms(nil)
	crypto.InitAsymAlgo(c.Cryptoer)
	crypto.InitHashAlgo(c.Hasher)
	conf.Config.LocalConf.NetworkID = c.NetworkID
	data, err := hex.DecodeString(c.Data)
	if err != nil {
		t.Fatal(err)
	}
	client := &Transaction{}
	if err := client.UnmarshallClient(bytes.NewBuffer(data), true); err != nil {
		t.Fatal(err)
	}
	stored := client.FullData
	var st storedTx
	if err := msgpack.Unmarshal(stored[1:], &st); err != nil {
		t.Fatal(err)
	}
	st.TxSignature = bytes.Clone(st.TxSignature)
	st.TxSignature[len(st.TxSignature)-1] ^= 0xff
	enc, err := msgpack.Marshal(&st)
	if err != nil {
		t.Fatal(err)
	}
	damaged := append([]byte{stored[0]}, enc...)
	tx := &Transaction{}
	if err := tx.Unmarshall(bytes.NewBuffer(bytes.Clone(damaged)), true); err != nil {
		t.Fatal(err)
	}

	p := NewPool()
	if _, _, err := p.Receive([][]byte{damaged}, true); err == nil {
		t.Fatal("a transaction whose signature does not hold is taken")
	}
	p.put(keyTx(string(tx.Hash()), 1, 1, 0), RateClient, true, nil)
	taken, full, err := p.Receive([][]byte{damaged, damaged}, true)
	if err != nil || taken != 0 || full != 0 {
		t.Fatalf("a transaction the pool holds: taken %d full %d: %v", taken, full, err)
	}
}
