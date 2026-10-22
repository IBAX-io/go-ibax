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

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"
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
	defaultPoolCount  = 100000
	defaultPoolBytes  = 256 << 20
	defaultBadMinutes = 60
)

// ErrPoolFull refuses a transaction the pool has no room for: the client sends it again later
var ErrPoolFull = errors.New("the transaction pool of the node is full, send the transaction again later")

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
}

type badStatus struct {
	msg string
	at  time.Time
}

// Pool holds in memory the transactions of the blocks to come, verified and checked, in the order
// they go into a block: by rate, by expedite descending, by time. A transaction leaves it when the
// block of the transaction is committed, or when it is found bad, whose status the pool answers for
// conf TxPool.BadMinutes. The pool of a node that restarts is empty: the clients send again what
// no block took.
type Pool struct {
	mu      sync.Mutex
	entries map[string]*poolEntry
	bytes   int64
	seq     uint64
	bad     map[string]badStatus
	pruned  time.Time
}

// TxPool is the pool of the node
var TxPool = NewPool()

// NewPool is an empty pool
func NewPool() *Pool {
	return &Pool{entries: make(map[string]*poolEntry), bad: make(map[string]badStatus)}
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

// Admit puts the transactions in the pool once Check takes them, each error at the index of its
// transaction. A transaction the pool holds is taken again as it is. A transaction Check refuses
// is bad; one the pool has no room for is refused with ErrPoolFull.
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
	checked := CheckBatch(fresh, time.Now().Unix())

	p.mu.Lock()
	defer p.mu.Unlock()
	for j, t := range fresh {
		i, h := index[j], string(t.Hash())
		if err := checked[j]; err != nil {
			errs[i] = err
			BadTxForBan(t.KeyID())
			p.setBad(h, err.Error())
			continue
		}
		if _, ok := p.entries[h]; ok {
			continue
		}
		if !p.fits(len(t.FullData)) {
			errs[i] = ErrPoolFull
			continue
		}
		p.put(t, rate, false)
	}
	p.gauge()
	return errs
}

// Restore puts back the transactions of a block rolled back, beyond the bounds of the pool: the
// other nodes have them, and no client sends them again. The block ran them, so Take parses them
// again.
func (p *Pool) Restore(txs []*Transaction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, t := range txs {
		if _, ok := p.entries[string(t.Hash())]; !ok {
			p.put(t, RateClient, true)
			p.entries[string(t.Hash())].tx = nil
		}
	}
	p.gauge()
}

// fits reports whether the pool has room for one more transaction of size bytes
func (p *Pool) fits(size int) bool {
	return len(p.entries) < maxPoolCount() && p.bytes+int64(size) <= maxPoolBytes()
}

func (p *Pool) put(t *Transaction, rate Rate, sent bool) {
	if t.Type() == types.StopNetworkTxType {
		rate = RateStopNetwork
	}
	p.seq++
	h := string(t.Hash())
	p.entries[h] = &poolEntry{
		hash: t.Hash(), data: t.FullData, tx: t, rate: rate, expedite: t.Expedite(), time: t.Timestamp(), seq: p.seq, sent: sent,
	}
	p.bytes += int64(len(t.FullData))
	delete(p.bad, h)
}

func (p *Pool) remove(h string) {
	if e, ok := p.entries[h]; ok {
		p.bytes -= int64(len(e.data))
		delete(p.entries, h)
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
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
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
	})
	return list
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
