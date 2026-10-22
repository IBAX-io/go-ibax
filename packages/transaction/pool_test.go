/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/types"
)

type poolCaller struct {
	TransactionCaller
	hash     []byte
	typ      byte
	time     int64
	expedite int64
}

func (c *poolCaller) txType() byte                { return c.typ }
func (c *poolCaller) txHash() []byte              { return c.hash }
func (c *poolCaller) txTime() int64               { return c.time }
func (c *poolCaller) txExpedite() decimal.Decimal { return decimal.NewFromInt(c.expedite) }

func poolTx(hash string, typ byte, time, expedite int64) *Transaction {
	return &Transaction{
		FullData: []byte("data of " + hash),
		Inner:    &poolCaller{hash: []byte(hash), typ: typ, time: time, expedite: expedite},
	}
}

func hashesOf(list []*Pending) []string {
	out := make([]string, len(list))
	for i, pd := range list {
		out[i] = string(pd.Tx.Hash())
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPoolOrder(t *testing.T) {
	p := NewPool()
	p.put(poolTx("late", types.SmartContractTxType, 20, 0), RateClient, false)
	p.put(poolTx("early", types.SmartContractTxType, 10, 0), RateClient, false)
	p.put(poolTx("same-time", types.SmartContractTxType, 10, 0), RateClient, false)
	p.put(poolTx("expedite", types.SmartContractTxType, 30, 5), RateClient, false)
	p.put(poolTx("on-block", types.SmartContractTxType, 40, 0), RateOnBlock, false)
	p.put(poolTx("stop", types.StopNetworkTxType, 1, 9), RateClient, false)

	got := hashesOf(p.Take(0))
	want := []string{"on-block", "expedite", "early", "same-time", "late", "stop"}
	if !equalStrings(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
	if !p.HasStopNetwork() {
		t.Fatal("the stop network transaction takes the rate of its type")
	}
}

func TestPoolTakeHandsOutParsedOnce(t *testing.T) {
	p := NewPool()
	tx := poolTx("a", types.SmartContractTxType, 1, 0)
	p.put(tx, RateClient, false)
	first := p.Take(1)
	if len(first) != 1 || first[0].Tx != tx {
		t.Fatalf("first take %v", first)
	}
	if p.entries["a"].tx != nil {
		t.Fatal("the pool keeps the parsed transaction it handed out")
	}
	if p.Len() != 1 {
		t.Fatal("a taken transaction stays in the pool until its block is committed")
	}
}

func TestPoolSentRemoveBad(t *testing.T) {
	p := NewPool()
	p.put(poolTx("a", types.SmartContractTxType, 1, 0), RateClient, false)
	p.put(poolTx("b", types.SmartContractTxType, 2, 0), RateClient, false)
	p.put(poolTx("c", types.SmartContractTxType, 3, 0), RateClient, true)

	data, hashes := p.Unsent(0)
	if len(data) != 2 || string(hashes[0]) != "a" || string(hashes[1]) != "b" {
		t.Fatalf("unsent %q", hashes)
	}
	p.MarkSent(hashes[:1])
	if _, hashes = p.Unsent(0); len(hashes) != 1 || string(hashes[0]) != "b" {
		t.Fatalf("unsent after mark %q", hashes)
	}

	size := p.bytes
	p.Remove([][]byte{[]byte("c")})
	if p.Has([]byte("c")) || p.bytes != size-int64(len("data of c")) {
		t.Fatalf("remove: has %v bytes %d", p.Has([]byte("c")), p.bytes)
	}
	if _, _, found := p.Status([]byte("c")); found {
		t.Fatal("a committed transaction has the status of its block, not the pool's")
	}

	p.Bad([]byte("b"), "bad b")
	if pending, msg, found := p.Status([]byte("b")); pending || msg != "bad b" || !found {
		t.Fatalf("bad status %v %q %v", pending, msg, found)
	}
	if pending, msg, found := p.Status([]byte("a")); !pending || msg != "" || !found {
		t.Fatalf("pending status %v %q %v", pending, msg, found)
	}
	if p.Len() != 1 {
		t.Fatalf("len %d", p.Len())
	}

	p.put(poolTx("b", types.SmartContractTxType, 2, 0), RateClient, false)
	if pending, _, _ := p.Status([]byte("b")); !pending {
		t.Fatal("a transaction put again is pending, not bad")
	}
}

func TestPoolBadRetention(t *testing.T) {
	p := NewPool()
	p.bad["old"] = badStatus{msg: "old", at: time.Now().Add(-badRetention() - time.Minute)}
	if _, _, found := p.Status([]byte("old")); found {
		t.Fatal("a bad status past retention is answered")
	}
	p.setBad("new", "new")
	if _, ok := p.bad["old"]; ok {
		t.Fatal("a bad status past retention is kept")
	}
}

func TestPoolBounds(t *testing.T) {
	saved := conf.Config.TxPool
	defer func() { conf.Config.TxPool = saved }()
	conf.Config.TxPool.MaxCount = 2
	conf.Config.TxPool.MaxBytes = 100

	p := NewPool()
	if !p.fits(100) || p.fits(101) {
		t.Fatal("bytes bound")
	}
	p.put(poolTx("a", types.SmartContractTxType, 1, 0), RateClient, false)
	p.put(poolTx("b", types.SmartContractTxType, 2, 0), RateClient, false)
	if p.fits(1) {
		t.Fatal("count bound")
	}
	p.Restore([]*Transaction{poolTx("c", types.SmartContractTxType, 3, 0)})
	if !p.Has([]byte("c")) || p.entries["c"].tx != nil || !p.entries["c"].sent {
		t.Fatal("a rolled back transaction goes back beyond the bounds, sent, to be parsed again")
	}
}
