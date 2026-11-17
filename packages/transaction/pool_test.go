/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"bytes"
	"math/rand"
	"sort"
	"strconv"
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
	if pending, bad, found := p.Status([]byte("b")); pending || bad != (Refusal{Msg: "bad b"}) || !found {
		t.Fatalf("bad status %v %+v %v", pending, bad, found)
	}
	if pending, bad, found := p.Status([]byte("a")); !pending || bad != (Refusal{}) || !found {
		t.Fatalf("pending status %v %+v %v", pending, bad, found)
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
	p.bad["old"] = badStatus{Refusal: Refusal{Msg: "old"}, at: time.Now().Add(-badRetention() - time.Minute)}
	if _, _, found := p.Status([]byte("old")); found {
		t.Fatal("a bad status past retention is answered")
	}
	p.setBad("new", "new", false)
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
	if err := p.take(tx, rate, now, nil); err != want {
		t.Fatalf("%s: %v, want %v", tx.Hash(), err, want)
	}
	requireStatus(t, p, tx, want)
}

// requireStatus checks the status of a transaction take placed or refused with want: pending, or
// the refusal but for a full pool, which is to retry for an account with MaxPerKey in the pool
func requireStatus(t *testing.T, p *Pool, tx *Transaction, want error) {
	t.Helper()
	pending, bad, _ := p.Status(tx.Hash())
	msg := bad.Msg
	switch {
	case bad.Retry != (want == ErrPoolKeyFull):
		t.Fatalf("%s: retry %v, refused with %v", tx.Hash(), bad.Retry, want)
	case want == nil && !pending:
		t.Fatalf("%s: not pending", tx.Hash())
	case want != nil && pending:
		t.Fatalf("%s: pending, refused with %v", tx.Hash(), want)
	case want != nil && want != ErrPoolFull && msg != want.Error():
		t.Fatalf("%s: status %q, want %q", tx.Hash(), msg, want)
	case want == ErrPoolFull && msg != "":
		t.Fatalf("%s: status %q of a full pool", tx.Hash(), msg)
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
	if _, bad, _ := p.Status([]byte("old")); bad != (Refusal{Msg: statusExpired}) {
		t.Fatalf("the status of the old transaction: %+v", bad)
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
	if _, bad, _ := p.Status([]byte("b")); bad != (Refusal{Msg: statusEvicted, Retry: true}) {
		t.Fatalf("the status of the evicted transaction: %+v", bad)
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

// TestPoolOrderIndex: through random puts, evictions, sends and removals, Take and Unsent are the
// entries, and those not sent, sorted in the order of the blocks
func TestPoolOrderIndex(t *testing.T) {
	poolLimits(t, 200, 1<<20, 1000, 50)
	now := time.Now().Unix()
	rnd := rand.New(rand.NewSource(1))
	p := NewPool()
	sorted := func(keep func(*poolEntry) bool) []string {
		var list []*poolEntry
		for _, e := range p.entries {
			if keep(e) {
				list = append(list, e)
			}
		}
		sort.Slice(list, func(i, j int) bool { return before(list[i], list[j]) })
		out := make([]string, len(list))
		for i, e := range list {
			out[i] = string(e.hash)
		}
		return out
	}
	hashes := func(list []*poolEntry) []string {
		out := make([]string, len(list))
		for i, e := range list {
			out[i] = string(e.hash)
		}
		return out
	}
	for i := range 5000 {
		h := strconv.Itoa(i)
		switch r := rnd.Intn(10); {
		case r < 6:
			rate := RateClient
			if rnd.Intn(20) == 0 {
				rate = RateOnBlock
			}
			p.take(keyTx(h, int64(rnd.Intn(300)), (now-int64(rnd.Intn(30)))*1000, int64(rnd.Intn(3))-1), rate, now, nil)
		case r < 8:
			var sent [][]byte
			for _, e := range first(p.unsent, rnd.Intn(5)) {
				sent = append(sent, e.hash)
			}
			p.MarkSent(sent)
		default:
			var gone [][]byte
			for _, e := range first(p.order, rnd.Intn(8)) {
				gone = append(gone, e.hash)
			}
			p.Remove(gone)
		}
		if p.order.Len() != len(p.entries) {
			t.Fatalf("step %d: %d ordered of %d", i, p.order.Len(), len(p.entries))
		}
		if i%97 == 0 {
			if got, want := hashes(first(p.order, 0)), sorted(func(*poolEntry) bool { return true }); !equalStrings(got, want) {
				t.Fatalf("step %d: order %v, want %v", i, got, want)
			}
			if got, want := hashes(first(p.unsent, 0)), sorted(func(e *poolEntry) bool { return !e.sent }); !equalStrings(got, want) {
				t.Fatalf("step %d: unsent %v, want %v", i, got, want)
			}
		}
	}
}

// BenchmarkPoolFullAdmit: a transaction that comes before the last of a full pool of 100,000
func BenchmarkPoolFullAdmit(b *testing.B) {
	saved := conf.Config.TxPool
	defer func() { conf.Config.TxPool = saved }()
	conf.Config.TxPool.MaxCount = 100000
	conf.Config.TxPool.MaxBytes = 1 << 30
	conf.Config.TxPool.MaxPerKey = 1 << 30
	now := time.Now().Unix()
	p := NewPool()
	for i := range 100000 {
		p.take(keyTx("f"+strconv.Itoa(i), int64(i), now*1000, 0), RateClient, now, nil)
	}
	b.ResetTimer()
	for i := range b.N {
		if err := p.take(keyTx("e"+strconv.Itoa(i), int64(i), (now-1)*1000-int64(i), 0), RateClient, now, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPoolTake: a block takes 20,000 of a pool of 100,000
func BenchmarkPoolTake(b *testing.B) {
	saved := conf.Config.TxPool
	defer func() { conf.Config.TxPool = saved }()
	conf.Config.TxPool.MaxCount = 100000
	conf.Config.TxPool.MaxBytes = 1 << 30
	conf.Config.TxPool.MaxPerKey = 1 << 30
	now := time.Now().Unix()
	p := NewPool()
	for i := range 100000 {
		p.take(keyTx("f"+strconv.Itoa(i), int64(i), now*1000+int64(i%1000), 0), RateClient, now, nil)
	}
	txs := make(map[*poolEntry]*Transaction, len(p.entries))
	for _, e := range p.entries {
		txs[e] = e.tx
	}
	b.ResetTimer()
	for range b.N {
		p.Take(20000)
		b.StopTimer()
		for e, tx := range txs {
			e.tx = tx
		}
		b.StartTimer()
	}
}

// stubKnown makes knownInBlocks call during with the hashes before it reports none in blocks
func stubKnown(t *testing.T, during func(hashes [][]byte)) {
	saved := knownInBlocks
	t.Cleanup(func() { knownInBlocks = saved })
	knownInBlocks = func(hashes [][]byte, _ int64) (map[string]bool, error) {
		during(hashes)
		return map[string]bool{}, nil
	}
}

// A block committed while an admission checks a transaction takes it out of the pool before the
// admission puts it in: the admission must not put it in after, for no block can take it again and
// the pool would hold work no proposer can fill
func TestPoolAdmitRacesCommit(t *testing.T) {
	poolLimits(t, 100, 1<<20, 100, 10)
	now := time.Now().Unix()
	p := NewPool()
	stubKnown(t, func(hashes [][]byte) { p.Remove(hashes) })
	late, other := keyTx("late", 1, now*1000, 0), keyTx("other", 2, now*1000, 0)
	errs := p.Admit([]*Transaction{late}, RateClient)
	if errs[0] != ErrDuplicatedTx {
		t.Fatalf("admitted a transaction committed while it was checked: %v", errs[0])
	}
	if p.Len() != 0 {
		t.Fatalf("the pool holds %d transactions committed", p.Len())
	}
	if _, bad, _ := p.Status(late.Hash()); bad.Msg != "" {
		t.Fatalf("a committed transaction is bad: %q", bad.Msg)
	}
	if len(p.recent) != 0 || len(p.admitting) != 0 {
		t.Fatalf("an admission ended keeps %d removals, %d admissions", len(p.recent), len(p.admitting))
	}

	// a block that commits other transactions keeps none from the pool
	stubKnown(t, func([][]byte) { p.Remove([][]byte{[]byte("third")}) })
	if errs := p.Admit([]*Transaction{other}, RateClient); errs[0] != nil || p.Len() != 1 {
		t.Fatalf("a transaction no block took is refused: %v, pool %d", errs[0], p.Len())
	}

	// without an admission a removal keeps no record
	p.Remove([][]byte{[]byte("other")})
	if len(p.recent) != 0 || p.commits != 3 {
		t.Fatalf("records %d, commits %d", len(p.recent), p.commits)
	}
}

// A removal a running admission needs stays until the earliest admission that started before it
// ends, and a rolled back transaction is not refused for the block rolled back
func TestPoolAdmitRemovals(t *testing.T) {
	p := NewPool()
	p.mu.Lock()
	p.admitting[0]++
	p.mu.Unlock()
	p.Remove([][]byte{[]byte("a")})
	p.mu.Lock()
	p.admitting[1]++
	p.mu.Unlock()
	p.Remove([][]byte{[]byte("b")})
	p.mu.Lock()
	if !p.removedSince(0, "a") || !p.removedSince(1, "b") || p.removedSince(1, "a") {
		t.Fatal("removals since a start")
	}
	p.admitted(1)
	if len(p.recent) != 2 {
		t.Fatalf("the admission of 0 runs, %d removals kept", len(p.recent))
	}
	p.mu.Unlock()
	p.Restore([]*Transaction{keyTx("b", 1, 1000, 0)})
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.removedSince(0, "b") {
		t.Fatal("a rolled back transaction is refused as committed")
	}
	p.admitted(0)
	if len(p.recent) != 0 || len(p.admitting) != 0 {
		t.Fatalf("no admission runs: %d removals, %d admissions", len(p.recent), len(p.admitting))
	}
}

// Live finds no transaction in a pool of those too old for a block, and takes out those of clients
func TestPoolLive(t *testing.T) {
	poolLimits(t, 100, 1<<20, 100, 10)
	now := time.Now().Unix()
	p := NewPool()
	p.put(keyTx("old", 1, (now-consts.MaxTxBack-1)*1000, 0), RateClient, false, nil)
	p.put(keyTx("own", 2, (now-consts.MaxTxBack-1)*1000, 0), RateOnBlock, false, nil)
	if p.Len() != 2 || p.Live(now) {
		t.Fatalf("live with %d too old", p.Len())
	}
	if p.Has([]byte("old")) || !p.Has([]byte("own")) {
		t.Fatal("an old transaction of a client stays or a reserved one is taken out")
	}
	p.put(keyTx("new", 3, now*1000, 0), RateClient, false, nil)
	if !p.Live(now) {
		t.Fatal("a pool with a new transaction is not live")
	}
}
