/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"bytes"
	"errors"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// Rate orders the transactions of the pool: a lower rate goes into a block first
type Rate int8

const (
	// RateOnBlock is a transaction the node makes for the next block
	RateOnBlock Rate = iota + 1
	// RateClient is a transaction a client or another node sent
	RateClient
	// RateStopNetwork is the transaction that stops the network
	RateStopNetwork
)

const (
	defaultPoolCount    = 100000
	defaultPoolBytes    = 256 << 20
	defaultPerKey       = 1024
	defaultReserveCount = 1024
	defaultReserveBytes = 16 << 20
	defaultBadMinutes   = 60
)

var (
	// ErrPoolFull refuses a transaction the pool has no room for: the client sends it again later
	ErrPoolFull = errors.New("the transaction pool of the node is full, send the transaction again later")
	// ErrPoolKeyFull refuses a transaction of an account that has MaxPerKey in the pool
	ErrPoolKeyFull = errors.New("the account has as many transactions as it may in the pool of the node, send the transaction again once a block takes some")
	// ErrKeyBanned refuses a transaction of an account the node banned for its bad transactions
	ErrKeyBanned = errors.New("the account is banned for its bad transactions")
)

// The status of a transaction the pool took out to make room
const (
	statusExpired = "the transaction is too old for a block"
	statusEvicted = "the pool of the node was full of transactions before it in the order of the blocks, send the transaction again with a higher expedite"
)

// Pending is a transaction the pool hands to a block
type Pending struct {
	Tx   *Transaction
	Data []byte
	Rate Rate
}

type poolEntry struct {
	hash     []byte
	data     []byte
	tx       *Transaction // parsed, until the pool hands it out
	rate     Rate
	expedite decimal.Decimal
	time     int64
	seq      uint64
	sent     bool
	key      int64
	signed   int64
	reserved bool   // a transaction of the node or an evidence of a validator, in the reserve
	funds    *funds // what it holds of a balance, nil if none
}

type badStatus struct {
	msg string
	at  time.Time
}

// Pool holds in memory the transactions of the blocks to come, verified and checked, in the order
// they go into a block: by rate, by expedite descending, by time. A transaction leaves it when the
// block of the transaction is committed, when it is found bad, or when it is taken out to make
// room, whose status the pool answers for conf TxPool.BadMinutes. The pool of a node that restarts
// is empty: the clients send again what no block took.
//
// The transactions of clients and other nodes fill conf TxPool.MaxCount and MaxBytes, MaxPerKey of
// an account at most, each with the funds a block takes of it beyond those the others of its
// account hold. The transactions the node makes and the evidences validators sign go into a
// reserve beyond, ReserveCount and ReserveBytes, which no transaction of a client takes.
type Pool struct {
	mu      sync.Mutex
	entries map[string]*poolEntry
	bytes   int64
	seq     uint64
	bad     map[string]badStatus
	pruned  time.Time
	// the transactions of the reserve and their bytes, of entries and bytes
	reserved      int
	reservedBytes int64
	perKey        map[int64]int // the transactions of clients and other nodes of each account
	worst         *poolEntry    // the last of them in the order of the blocks, nil if unknown
	expired       int64         // the second the pool last took out the transactions too old
	// what the transactions hold of each balance
	held map[fundsBucket]decimal.Decimal
}

// TxPool is the pool of the node
var TxPool = NewPool()

// NewPool is an empty pool
func NewPool() *Pool {
	return &Pool{entries: make(map[string]*poolEntry), bad: make(map[string]badStatus), perKey: make(map[int64]int), held: make(map[fundsBucket]decimal.Decimal)}
}

func maxPoolCount() int {
	if n := conf.Config.TxPool.MaxCount; n > 0 {
		return n
	}
	return defaultPoolCount
}

func maxPoolBytes() int64 {
	if n := conf.Config.TxPool.MaxBytes; n > 0 {
		return n
	}
	return defaultPoolBytes
}

func maxPerKey() int {
	if n := conf.Config.TxPool.MaxPerKey; n > 0 {
		return n
	}
	return defaultPerKey
}

func reserveCount() int {
	if n := conf.Config.TxPool.ReserveCount; n > 0 {
		return n
	}
	return defaultReserveCount
}

func reserveBytes() int64 {
	if n := conf.Config.TxPool.ReserveBytes; n > 0 {
		return n
	}
	return defaultReserveBytes
}

func badRetention() time.Duration {
	if n := conf.Config.TxPool.BadMinutes; n > 0 {
		return time.Duration(n) * time.Minute
	}
	return defaultBadMinutes * time.Minute
}

// VerifyAll verifies the transactions in parallel, each error at the index of its transaction
func VerifyAll(txs []*Transaction) []error {
	errs := make([]error, len(txs))
	workers := min(runtime.GOMAXPROCS(0), len(txs))
	var wg sync.WaitGroup
	next := make(chan int)
	for range workers {
		wg.Go(func() {
			for i := range next {
				errs[i] = txs[i].Verify()
			}
		})
	}
	for i := range txs {
		next <- i
	}
	close(next)
	wg.Wait()
	return errs
}

// Stored is each transaction of the hashes in the form a block stores it, compressed, nil for one
// the pool does not have: a validator fills in the block of a compact proposal with them
func (p *Pool) Stored(hashes [][]byte) [][]byte {
	data := make([][]byte, len(hashes))
	p.mu.Lock()
	for i, h := range hashes {
		if e, ok := p.entries[string(h)]; ok {
			data[i] = e.data
		}
	}
	p.mu.Unlock()
	stored := make([][]byte, len(hashes))
	workers := min(runtime.GOMAXPROCS(0), len(hashes))
	var wg sync.WaitGroup
	next := make(chan int)
	for range workers {
		wg.Go(func() {
			for i := range next {
				stored[i] = types.DoZlibCompress(data[i])
			}
		})
	}
	for i, d := range data {
		if d != nil {
			next <- i
		}
	}
	close(next)
	wg.Wait()
	return stored
}

// Raw is the bodies of the transactions of the hashes the pool holds, as they were sent, nil for
// those it does not hold
func (p *Pool) Raw(hashes [][]byte) [][]byte {
	data := make([][]byte, len(hashes))
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, h := range hashes {
		if e, ok := p.entries[string(h)]; ok {
			data[i] = e.data
		}
	}
	return data
}

// Admit puts the transactions in the pool once Check takes them, each error at the index of its
// transaction. A transaction the pool holds is taken again as it is. A transaction Check refuses
// is bad; take refuses the others it has no room or no funds for.
func (p *Pool) Admit(txs []*Transaction, rate Rate) []error {
	errs := make([]error, len(txs))
	fresh := make([]*Transaction, 0, len(txs))
	index := make([]int, 0, len(txs))
	p.mu.Lock()
	for i, t := range txs {
		if _, ok := p.entries[string(t.Hash())]; !ok {
			fresh = append(fresh, t)
			index = append(index, i)
		}
	}
	p.mu.Unlock()
	now := time.Now().Unix()
	checked := CheckBatch(fresh, now)
	fund, l := make([]*funds, len(fresh)), newBatchLedger()
	for j, t := range fresh {
		if checked[j] == nil && !p.reservedTx(t, rate) {
			fund[j], checked[j] = fundsOf(t, l)
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for j, t := range fresh {
		i, h := index[j], string(t.Hash())
		if err := checked[j]; err != nil {
			errs[i] = err
			bad, ban := refusal(err)
			if ban {
				BadTxForBan(t.KeyID())
			}
			if bad {
				p.setBad(h, err.Error())
			}
			continue
		}
		if _, ok := p.entries[h]; ok {
			continue
		}
		errs[i] = p.take(t, rate, now, fund[j])
	}
	p.gauge()
	return errs
}

// take places a transaction. The status of one it refuses is the refusal, until the pool takes
// it, but for a full pool, which the client is answered with.
func (p *Pool) take(t *Transaction, rate Rate, now int64, f *funds) error {
	err := p.place(t, rate, now, f)
	if err != nil && !errors.Is(err, ErrPoolFull) {
		p.setBad(string(t.Hash()), err.Error())
	}
	return err
}

// place puts a transaction in the pool, or refuses it. A transaction of the node or an evidence of
// a validator goes into the reserve. One of a client or another node is refused when the node
// banned its account, the account has MaxPerKey in the pool, or the balance it draws on has not
// its funds beyond those the transactions in the pool hold. When the pool is full, the
// transactions too old for a block leave it, then the last in the order of the blocks, if the
// transaction comes before them: a flood of transactions takes out none that come before it, as
// a higher expedite does.
func (p *Pool) place(t *Transaction, rate Rate, now int64, f *funds) error {
	size := len(t.FullData)
	if p.reservedTx(t, rate) {
		if p.reserved >= reserveCount() || p.reservedBytes+int64(size) > reserveBytes() {
			return ErrPoolFull
		}
		p.put(t, rate, false, nil)
		return nil
	}
	key := t.KeyID()
	if IsKeyBanned(key) {
		return ErrKeyBanned
	}
	if p.perKey[key] >= maxPerKey() {
		return ErrPoolKeyFull
	}
	if !p.hold(f) {
		return ErrPoolNoFunds
	}
	if !p.fits(size) {
		p.expire(now)
	}
	if !p.fits(size) && !p.evict(t, size) {
		return ErrPoolFull
	}
	p.put(t, rate, false, f)
	return nil
}

// honorNodes are the nodes whose evidences go into the reserve
var honorNodes = syspar.GetNodes

// reservedTx reports whether the transaction goes into the reserve: the node made it, it stops the
// network, or it is an evidence a validator signed with the key of its node, as the evidences of
// other validators come
func (p *Pool) reservedTx(t *Transaction, rate Rate) bool {
	if rate != RateClient || t.Type() == types.StopNetworkTxType {
		return true
	}
	if !t.IsSmartContract() {
		return false
	}
	sc := t.SmartContract()
	if sc.TxContract == nil || sc.TxContract.Name != smart.BftEvidenceContract || sc.TxSmart.SignedBy == 0 {
		return false
	}
	for _, node := range honorNodes() {
		if crypto.Address(node.PublicKey) == sc.TxSmart.SignedBy {
			return bytes.Equal(crypto.NodeAccountKey(node.PublicKey).Bytes(), sc.TxSmart.PublicKey)
		}
	}
	return false
}

// expire takes out, once a second at most, the transactions of clients and other nodes too old
// for a block
func (p *Pool) expire(now int64) {
	if p.expired == now {
		return
	}
	p.expired = now
	for h, e := range p.entries {
		if !e.reserved && e.signed < now-consts.MaxTxBack {
			p.remove(h)
			p.setBad(h, statusExpired)
		}
	}
}

// evict takes out the last transactions of clients and other nodes in the order of the blocks, all
// after the transaction, to make room for size bytes of it, and reports whether it did. It takes
// out none when it cannot make the room.
func (p *Pool) evict(t *Transaction, size int) bool {
	next := &poolEntry{rate: RateClient, expedite: t.Expedite(), time: t.Timestamp(), seq: p.seq + 1}
	if w := p.worstEntry(); w == nil || !before(next, w) {
		return false
	}
	list := p.ordered(func(e *poolEntry) bool { return !e.reserved })
	count, room := len(p.entries)-p.reserved, maxPoolBytes()-(p.bytes-p.reservedBytes)
	n := 0
	for i := len(list) - 1; i >= 0 && before(next, list[i]); i-- {
		n++
		count--
		room += int64(len(list[i].data))
		if count < maxPoolCount() && int64(size) <= room {
			for _, e := range list[len(list)-n:] {
				h := string(e.hash)
				p.remove(h)
				p.setBad(h, statusEvicted)
			}
			return true
		}
	}
	return false
}

// worstEntry is the last transaction of a client or another node in the order of the blocks
func (p *Pool) worstEntry() *poolEntry {
	if p.worst != nil {
		return p.worst
	}
	for _, e := range p.entries {
		if !e.reserved && (p.worst == nil || before(p.worst, e)) {
			p.worst = e
		}
	}
	return p.worst
}

// Restore puts back the transactions of a block rolled back, beyond the bounds of the pool: the
// other nodes have them, and no client sends them again. The block ran them, so Take parses them
// again.
func (p *Pool) Restore(txs []*Transaction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, t := range txs {
		if _, ok := p.entries[string(t.Hash())]; !ok {
			p.put(t, RateClient, true, nil)
			p.entries[string(t.Hash())].tx = nil
		}
	}
	p.gauge()
}

// fits reports whether the pool has room for one more transaction of a client or another node of
// size bytes
func (p *Pool) fits(size int) bool {
	return len(p.entries)-p.reserved < maxPoolCount() && p.bytes-p.reservedBytes+int64(size) <= maxPoolBytes()
}

func (p *Pool) put(t *Transaction, rate Rate, sent bool, f *funds) {
	reserved := p.reservedTx(t, rate)
	if t.Type() == types.StopNetworkTxType {
		rate = RateStopNetwork
	}
	p.seq++
	h := string(t.Hash())
	e := &poolEntry{
		hash: t.Hash(), data: t.FullData, tx: t, rate: rate, expedite: t.Expedite(), time: t.Timestamp(), seq: p.seq, sent: sent,
		key: t.KeyID(), signed: t.SignedTime(), reserved: reserved,
	}
	if !reserved && f != nil {
		e.funds = f
		p.held[f.bucket] = p.held[f.bucket].Add(f.amount)
	}
	p.entries[h] = e
	p.bytes += int64(len(t.FullData))
	if reserved {
		p.reserved++
		p.reservedBytes += int64(len(t.FullData))
	} else {
		p.perKey[e.key]++
		if p.worst != nil && before(p.worst, e) {
			p.worst = e
		}
	}
	delete(p.bad, h)
}

func (p *Pool) remove(h string) {
	e, ok := p.entries[h]
	if !ok {
		return
	}
	p.bytes -= int64(len(e.data))
	delete(p.entries, h)
	if e.reserved {
		p.reserved--
		p.reservedBytes -= int64(len(e.data))
		return
	}
	if p.perKey[e.key]--; p.perKey[e.key] <= 0 {
		delete(p.perKey, e.key)
	}
	if f := e.funds; f != nil {
		if p.held[f.bucket] = p.held[f.bucket].Sub(f.amount); !p.held[f.bucket].IsPositive() {
			delete(p.held, f.bucket)
		}
	}
	if p.worst == e {
		p.worst = nil
	}
}

func (p *Pool) gauge() {
	metrics.TxPool.WithLabelValues("count").Set(float64(len(p.entries)))
	metrics.TxPool.WithLabelValues("bytes").Set(float64(p.bytes))
}

// ordered is the entries in the order of the blocks
func (p *Pool) ordered(keep func(*poolEntry) bool) []*poolEntry {
	list := make([]*poolEntry, 0, len(p.entries))
	for _, e := range p.entries {
		if keep == nil || keep(e) {
			list = append(list, e)
		}
	}
	sort.Slice(list, func(i, j int) bool { return before(list[i], list[j]) })
	return list
}

// before reports whether a goes into a block before b
func before(a, b *poolEntry) bool {
	if a.rate != b.rate {
		return a.rate < b.rate
	}
	if c := a.expedite.Cmp(b.expedite); c != 0 {
		return c > 0
	}
	if a.time != b.time {
		return a.time < b.time
	}
	return a.seq < b.seq
}

// Take is the first limit transactions in the order of the blocks, parsed. They stay in the pool
// until their block is committed; a transaction is handed out parsed once, and parsed again after,
// as a block that runs it changes it.
func (p *Pool) Take(limit int) []*Pending {
	p.mu.Lock()
	list := p.ordered(nil)
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	out := make([]*Pending, 0, len(list))
	for _, e := range list {
		out = append(out, &Pending{Tx: e.tx, Data: e.data, Rate: e.rate})
		e.tx = nil
	}
	p.mu.Unlock()
	taken := out[:0]
	for _, pd := range out {
		if pd.Tx == nil {
			tx, err := UnmarshallTransaction(bytes.NewBuffer(pd.Data), true)
			if err != nil {
				log.WithFields(log.Fields{"type": consts.ParseError, "error": err}).Error("parsing a transaction of the pool")
				continue
			}
			pd.Tx = tx
		}
		taken = append(taken, pd)
	}
	return taken
}

// Unsent is the data and the hashes of the first limit transactions not sent to the other nodes
func (p *Pool) Unsent(limit int) (data, hashes [][]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	list := p.ordered(func(e *poolEntry) bool { return !e.sent })
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	for _, e := range list {
		data = append(data, e.data)
		hashes = append(hashes, e.hash)
	}
	return data, hashes
}

// MarkSent records the transactions sent to the other nodes
func (p *Pool) MarkSent(hashes [][]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, h := range hashes {
		if e, ok := p.entries[string(h)]; ok {
			e.sent = true
		}
	}
}

// Remove takes out the transactions of a block committed
func (p *Pool) Remove(hashes [][]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, h := range hashes {
		p.remove(string(h))
	}
	p.gauge()
}

// Bad takes out a transaction found bad, whose status the pool answers for conf
// TxPool.BadMinutes
func (p *Pool) Bad(hash []byte, msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.remove(string(hash))
	p.setBad(string(hash), msg)
	p.gauge()
}

func (p *Pool) setBad(h, msg string) {
	now := time.Now()
	p.bad[h] = badStatus{msg: msg, at: now}
	if now.Sub(p.pruned) < time.Minute {
		return
	}
	p.pruned = now
	keep := badRetention()
	for k, b := range p.bad {
		if now.Sub(b.at) > keep {
			delete(p.bad, k)
		}
	}
}

// Has reports whether the pool holds the transaction
func (p *Pool) Has(hash []byte) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.entries[string(hash)]
	return ok
}

// Len is the transactions of the pool
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}

// HasStopNetwork reports whether the pool holds the transaction that stops the network
func (p *Pool) HasStopNetwork() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.entries {
		if e.rate == RateStopNetwork {
			return true
		}
	}
	return false
}

// Status is whether the pool holds the transaction, or the error it was found bad with
func (p *Pool) Status(hash []byte) (pending bool, badMsg string, found bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.entries[string(hash)]; ok {
		return true, "", true
	}
	if b, ok := p.bad[string(hash)]; ok && time.Since(b.at) <= badRetention() {
		return false, b.msg, true
	}
	return false, "", false
}

// Receive parses the transactions another node sent, verifies them in parallel and puts them in
// the pool. It refuses the packet at the first transaction that does not parse or verify; one the
// pool refuses is left out.
func (p *Pool) Receive(datas [][]byte) error {
	txs := make([]*Transaction, 0, len(datas))
	for _, data := range datas {
		tx := &Transaction{}
		if err := tx.Unmarshall(bytes.NewBuffer(data), true); err != nil {
			log.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err}).Error("unmarshalling transaction")
			return err
		}
		txs = append(txs, tx)
	}
	for _, err := range VerifyAll(txs) {
		if err != nil {
			log.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err}).Error("verifying transaction")
			return err
		}
	}
	for i, err := range p.Admit(txs, RateClient) {
		if err != nil {
			log.WithFields(log.Fields{"tx_hash": txs[i].Hash(), "error": err}).Debug("a transaction of another node left out")
		}
	}
	return nil
}
