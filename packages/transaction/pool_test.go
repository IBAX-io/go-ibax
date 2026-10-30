/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"bytes"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/types"
)

type poolCaller struct {
	TransactionCaller
	hash     []byte
	typ      byte
	time     int64
	expedite int64
	key      int64
}

func (c *poolCaller) txType() byte                { return c.typ }
func (c *poolCaller) txHash() []byte              { return c.hash }
func (c *poolCaller) txTime() int64               { return c.time }
func (c *poolCaller) txSignedTime() int64         { return c.time / 1000 }
func (c *poolCaller) txExpedite() decimal.Decimal { return decimal.NewFromInt(c.expedite) }
func (c *poolCaller) txKeyID() int64              { return c.key }

func poolTx(hash string, typ byte, time, expedite int64) *Transaction {
	t := keyTx(hash, 1, time, expedite)
	t.Inner.(*poolCaller).typ = typ
	return t
}

// keyTx is a contract transaction of the account key, of the time in milliseconds
func keyTx(hash string, key, time, expedite int64) *Transaction {
	return &Transaction{
		FullData: []byte("data of " + hash),
		Inner:    &poolCaller{hash: []byte(hash), typ: types.SmartContractTxType, time: time, expedite: expedite, key: key},
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
	p.put(poolTx("late", types.SmartContractTxType, 20, 0), RateClient, false, nil)
	p.put(poolTx("early", types.SmartContractTxType, 10, 0), RateClient, false, nil)
	p.put(poolTx("same-time", types.SmartContractTxType, 10, 0), RateClient, false, nil)
	p.put(poolTx("expedite", types.SmartContractTxType, 30, 5), RateClient, false, nil)
	p.put(poolTx("on-block", types.SmartContractTxType, 40, 0), RateOnBlock, false, nil)
	p.put(poolTx("stop", types.StopNetworkTxType, 1, 9), RateClient, false, nil)

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
	p.put(tx, RateClient, false, nil)
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
	p.put(poolTx("a", types.SmartContractTxType, 1, 0), RateClient, false, nil)
	p.put(poolTx("b", types.SmartContractTxType, 2, 0), RateClient, false, nil)
	p.put(poolTx("c", types.SmartContractTxType, 3, 0), RateClient, true, nil)

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

	p.put(poolTx("b", types.SmartContractTxType, 2, 0), RateClient, false, nil)
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
	p.put(poolTx("a", types.SmartContractTxType, 1, 0), RateClient, false, nil)
	p.put(poolTx("b", types.SmartContractTxType, 2, 0), RateClient, false, nil)
	if p.fits(1) {
		t.Fatal("count bound")
	}
	p.Restore([]*Transaction{poolTx("c", types.SmartContractTxType, 3, 0)})
	if !p.Has([]byte("c")) || p.entries["c"].tx != nil || !p.entries["c"].sent {
		t.Fatal("a rolled back transaction goes back beyond the bounds, sent, to be parsed again")
	}
}

func poolLimits(t *testing.T, count int, bytes int64, perKey, reserve int) {
	saved := conf.Config.TxPool
	t.Cleanup(func() { conf.Config.TxPool = saved })
	conf.Config.TxPool.MaxCount = count
	conf.Config.TxPool.MaxBytes = bytes
	conf.Config.TxPool.MaxPerKey = perKey
	conf.Config.TxPool.ReserveCount = reserve
	conf.Config.TxPool.ReserveBytes = 1 << 20
}

func requirePlaced(t *testing.T, p *Pool, tx *Transaction, rate Rate, now int64, want error) {
	t.Helper()
	if err := p.place(tx, rate, now, nil); err != want {
		t.Fatalf("%s: %v, want %v", tx.Hash(), err, want)
	}
}

// A pool full of transactions of clients takes the transactions of the node and the stop of the
// network in the reserve, which the clients do not take
func TestPoolReserve(t *testing.T) {
	poolLimits(t, 2, 1<<20, 10, 2)
	now := time.Now().Unix()
	p := NewPool()
	requirePlaced(t, p, keyTx("a", 1, now*1000, 0), RateClient, now, nil)
	requirePlaced(t, p, keyTx("b", 2, now*1000, 0), RateClient, now, nil)
	requirePlaced(t, p, keyTx("c", 3, now*1000, 0), RateClient, now, ErrPoolFull)
	requirePlaced(t, p, keyTx("node", 4, now*1000, 0), RateOnBlock, now, nil)
	requirePlaced(t, p, poolTx("stop", types.StopNetworkTxType, now*1000, 0), RateClient, now, nil)
	requirePlaced(t, p, keyTx("node2", 4, now*1000, 0), RateOnBlock, now, ErrPoolFull)
	if p.reserved != 2 || p.Len() != 4 {
		t.Fatalf("reserve %d of %d", p.reserved, p.Len())
	}
	p.Remove([][]byte{[]byte("node")})
	requirePlaced(t, p, keyTx("c", 3, now*1000, 0), RateClient, now, ErrPoolFull)
	requirePlaced(t, p, keyTx("node2", 4, now*1000, 0), RateOnBlock, now, nil)
	p.Remove([][]byte{[]byte("a")})
	requirePlaced(t, p, keyTx("c", 3, now*1000, 0), RateClient, now, nil)
}

// An account has MaxPerKey transactions of clients and other nodes in the pool at most; those of
// the node are not counted
func TestPoolPerKeyLimit(t *testing.T) {
	poolLimits(t, 100, 1<<20, 2, 10)
	now := time.Now().Unix()
	p := NewPool()
	requirePlaced(t, p, keyTx("a", 1, now*1000, 0), RateClient, now, nil)
	requirePlaced(t, p, keyTx("b", 1, now*1000, 0), RateClient, now, nil)
	requirePlaced(t, p, keyTx("c", 1, now*1000, 9), RateClient, now, ErrPoolKeyFull)
	requirePlaced(t, p, keyTx("d", 2, now*1000, 0), RateClient, now, nil)
	requirePlaced(t, p, keyTx("node", 1, now*1000, 0), RateOnBlock, now, nil)
	p.Remove([][]byte{[]byte("a")})
	requirePlaced(t, p, keyTx("c", 1, now*1000, 9), RateClient, now, nil)
	if p.perKey[1] != 2 || p.perKey[2] != 1 {
		t.Fatalf("per key %v", p.perKey)
	}
	p.Bad([]byte("b"), "bad")
	p.Remove([][]byte{[]byte("c"), []byte("d")})
	if len(p.perKey) != 0 {
		t.Fatalf("per key after removal %v", p.perKey)
	}
}

// A banned account sends nothing into the pool
func TestPoolBannedKey(t *testing.T) {
	poolLimits(t, 100, 1<<20, 10, 10)
	saved := conf.Config.BanKey
	t.Cleanup(func() { conf.Config.BanKey = saved })
	conf.Config.BanKey.BadTx = 1
	conf.Config.BanKey.BanTime = 60000
	conf.Config.BanKey.BadTime = 1
	const key = 987654321
	t.Cleanup(func() { mutex.Lock(); delete(banList, key); mutex.Unlock() })
	BadTxForBan(key)
	BadTxForBan(key)
	if !IsKeyBanned(key) {
		t.Fatal("the key is not banned")
	}
	now := time.Now().Unix()
	p := NewPool()
	requirePlaced(t, p, keyTx("a", key, now*1000, 0), RateClient, now, ErrKeyBanned)
	requirePlaced(t, p, keyTx("b", key+1, now*1000, 0), RateClient, now, nil)
}

// A full pool takes out the transactions too old for a block, then the last in the order of the
// blocks for one that comes before them, with a higher expedite; a flood of the same expedite
// takes out nothing
func TestPoolEviction(t *testing.T) {
	poolLimits(t, 3, 1<<20, 10, 10)
	now := time.Now().Unix()
	p := NewPool()
	old := (now - consts.MaxTxBack - 1) * 1000
	requirePlaced(t, p, keyTx("old", 1, old, 0), RateClient, now, nil)
	requirePlaced(t, p, keyTx("a", 2, now*1000, 1), RateClient, now, nil)
	requirePlaced(t, p, keyTx("b", 3, now*1000, 0), RateClient, now, nil)
	requirePlaced(t, p, keyTx("c", 4, now*1000, 0), RateClient, now, nil)
	if p.Has([]byte("old")) {
		t.Fatal("a transaction too old for a block stays in a full pool")
	}
	if _, msg, _ := p.Status([]byte("old")); msg != statusExpired {
		t.Fatalf("the status of the old transaction: %q", msg)
	}
	requirePlaced(t, p, keyTx("flood", 5, now*1000, 0), RateClient, now, ErrPoolFull)
	requirePlaced(t, p, keyTx("earlier", 5, (now-1)*1000, 0), RateClient, now, nil)
	if p.Has([]byte("c")) || !p.Has([]byte("b")) {
		t.Fatal("an earlier transaction takes out the last one")
	}
	requirePlaced(t, p, keyTx("high", 6, now*1000, 2), RateClient, now, nil)
	if p.Has([]byte("b")) || !p.Has([]byte("a")) || !p.Has([]byte("earlier")) {
		t.Fatalf("a higher expedite takes out the last: %v", hashesOf(p.Take(0)))
	}
	if _, msg, _ := p.Status([]byte("b")); msg != statusEvicted {
		t.Fatalf("the status of the evicted transaction: %q", msg)
	}
	requirePlaced(t, p, keyTx("node", 7, now*1000, 0), RateOnBlock, now, nil)
	requirePlaced(t, p, keyTx("higher", 8, now*1000, 3), RateClient, now, nil)
	if !p.Has([]byte("node")) || p.Has([]byte("earlier")) {
		t.Fatal("a transaction of a client takes out one of the reserve")
	}
	if got := hashesOf(p.Take(0)); !equalStrings(got, []string{"node", "higher", "high", "a"}) {
		t.Fatalf("pool %v", got)
	}
}

// A transaction larger than the last one takes out as many as its room needs, or none
func TestPoolEvictionBytes(t *testing.T) {
	poolLimits(t, 10, 40, 10, 10)
	now := time.Now().Unix()
	p := NewPool()
	requirePlaced(t, p, keyTx("a", 1, now*1000, 0), RateClient, now, nil)
	requirePlaced(t, p, keyTx("b", 2, now*1000, 0), RateClient, now, nil)
	requirePlaced(t, p, keyTx("c", 3, now*1000, 0), RateClient, now, nil)
	big := keyTx("big", 4, now*1000, 1)
	big.FullData = make([]byte, 41)
	requirePlaced(t, p, big, RateClient, now, ErrPoolFull)
	if p.Len() != 3 {
		t.Fatal("a transaction that cannot fit took out others")
	}
	big.FullData = make([]byte, 30)
	requirePlaced(t, p, big, RateClient, now, nil)
	if got := hashesOf(p.Take(0)); !equalStrings(got, []string{"big", "a"}) {
		t.Fatalf("pool %v", got)
	}
}

// A transaction whose room needs one that comes before it in the order of the blocks takes out
// none
func TestPoolEvictionStopsAtEarlier(t *testing.T) {
	poolLimits(t, 10, 40, 10, 10)
	now := time.Now().Unix()
	p := NewPool()
	requirePlaced(t, p, keyTx("high", 1, now*1000, 5), RateClient, now, nil)
	requirePlaced(t, p, keyTx("b", 2, now*1000, 0), RateClient, now, nil)
	requirePlaced(t, p, keyTx("c", 3, now*1000, 0), RateClient, now, nil)
	big := keyTx("big", 4, now*1000, 1)
	big.FullData = make([]byte, 35)
	requirePlaced(t, p, big, RateClient, now, ErrPoolFull)
	if p.Len() != 3 {
		t.Fatal("a transaction that cannot fit took out others")
	}
}

func evidenceTx(hash, contract string, signedBy int64, pub []byte, now int64) *Transaction {
	return &Transaction{
		FullData: []byte("data of " + hash),
		Inner: &SmartTransactionParser{SmartContract: &smart.SmartContract{
			Hash:       []byte(hash),
			Timestamp:  now * 1000,
			TxContract: &smart.Contract{Name: contract},
			TxSmart:    &types.SmartTransaction{Header: &types.Header{KeyID: 1, Time: now, PublicKey: pub}, SignedBy: signedBy},
		}},
	}
}

// An evidence a validator signs with the key of its node, as another node sends it on, goes into
// the reserve; one signed with another key, and another contract the node signs, do not
func TestPoolReserveEvidence(t *testing.T) {
	poolLimits(t, 1, 1<<20, 10, 10)
	node := bytes.Repeat([]byte{7}, 64)
	saved := honorNodes
	t.Cleanup(func() { honorNodes = saved })
	honorNodes = func() []syspar.HonorNode { return []syspar.HonorNode{{PublicKey: node}} }
	addr, key := crypto.Address(node), crypto.NodeAccountKey(node).Bytes()
	now := time.Now().Unix()
	p := NewPool()
	requirePlaced(t, p, keyTx("a", 1, now*1000, 0), RateClient, now, nil)
	requirePlaced(t, p, evidenceTx("other key", smart.BftEvidenceContract, addr, []byte("other"), now), RateClient, now, ErrPoolFull)
	requirePlaced(t, p, evidenceTx("other contract", "NewParameter", addr, key, now), RateClient, now, ErrPoolFull)
	requirePlaced(t, p, evidenceTx("other node", smart.BftEvidenceContract, addr+1, key, now), RateClient, now, ErrPoolFull)
	requirePlaced(t, p, evidenceTx("evidence", smart.BftEvidenceContract, addr, key, now), RateClient, now, nil)
	if p.reserved != 1 {
		t.Fatalf("reserve %d", p.reserved)
	}
}
