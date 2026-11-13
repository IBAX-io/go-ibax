/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"cmp"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
)

// errNoValidator is a validator of no address in honor_nodes
var errNoValidator = errors.New("no address of the validator")

// bftPending gets a node without transactions those of a validator that has some (F18 design
// 4.7: a proposer has in its pool the transactions sent to the network). Each node sends its own
// transactions to the others once, and a full queue or a full pool loses some of them: a pool can
// be empty while another validator holds transactions, and its round of proposer would time out
// with no block. So a validator with transactions at its height says so to the others when it
// starts the height and every bftResend after; a node whose pool is empty then takes the first of
// them in the order of the blocks from that validator, and starts the height with them from when
// the validator said so. A proposer that finds no transaction for its block takes them at once
// from the validators that said so, within the time of its proposal.
type bftPending struct {
	mu     sync.Mutex
	height uint64
	set    bft.Validators
	self   uint32
	member bool // whether the node is a validator of set
	// hints is when each validator first said it had transactions, at the height and the next:
	// a validator may start the next height before the node commits the height
	hints map[uint64]map[uint32]time.Time
	// pulled is when the node last took transactions from each validator at the height
	pulled map[uint32]time.Time
	// busy is whether a pull of hint runs
	busy bool
	// said and saidAt are the height and the time the node last said it had transactions
	said   uint64
	saidAt time.Time

	poolLen func() int
	// fetch takes from the node of the key the first transactions of its pool within timeout
	fetch func(key []byte, timeout time.Duration) ([][]byte, error)
	// receive puts the transactions in the pool and is the count it put
	receive func(bodies [][]byte) int
	// work tells the driver that the height has transactions since at
	work func(height uint64, at time.Time)
	// say says to the others that the validator has transactions at the height
	say func(height uint64, validator uint32)
	// timeout is the most a pull takes
	timeout time.Duration
	now     func() time.Time
}

// moveTo moves to the height of the set; self is the node public key of the node. The hints of
// the height the node had before it moved are taken at once.
func (p *bftPending) moveTo(height uint64, set bft.Validators, self []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.height == height && p.hints != nil {
		return
	}
	p.height, p.set = height, set
	p.self, p.member = set.Index(self)
	kept := p.hints[height]
	p.hints = map[uint64]map[uint32]time.Time{}
	if kept != nil {
		p.hints[height] = kept
	}
	p.pulled = map[uint32]time.Time{}
	p.pull()
}

// hint takes the word of a validator that it has transactions at the height: the node pulls them
// if its pool is empty
func (p *bftPending) hint(height uint64, validator uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hints == nil || (height != p.height && height != p.height+1) {
		return
	}
	at := p.hints[height]
	if at == nil {
		at = map[uint32]time.Time{}
		p.hints[height] = at
	}
	if _, ok := at[validator]; !ok {
		at[validator] = p.now()
	}
	if height == p.height {
		p.pull()
	}
}

// sources is the validators of the set but the node that said they had transactions at the
// height, the first to say it first, the lower index first of those that said it at once
func (p *bftPending) sources() []uint32 {
	var list []uint32
	for v := range p.hints[p.height] {
		if _, ok := p.set.Key(v); ok && (!p.member || v != p.self) {
			list = append(list, v)
		}
	}
	at := p.hints[p.height]
	slices.SortFunc(list, func(a, b uint32) int {
		if c := at[a].Compare(at[b]); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	return list
}

// pull starts a pull of the transactions of a validator that said it had some at the height, if
// the pool of the node is empty, no pull runs, and the node did not pull from it in bftResend.
// It is called with mu held.
func (p *bftPending) pull() {
	if p.busy || p.poolLen() > 0 {
		return
	}
	now := p.now()
	for _, v := range p.sources() {
		if last, ok := p.pulled[v]; ok && now.Sub(last) < bftResend {
			continue
		}
		key, _ := p.set.Key(v)
		height, at := p.height, p.hints[p.height][v]
		p.pulled[v], p.busy = now, true
		go func() {
			bodies, err := p.fetch(key, p.timeout)
			taken := 0
			if err == nil {
				taken = p.receive(bodies)
			}
			p.mu.Lock()
			p.busy = false
			if taken == 0 && p.height == height {
				// another validator that said so may have them
				p.pull()
			}
			p.mu.Unlock()
			if taken > 0 {
				p.work(height, at)
			}
		}()
		return
	}
}

// pullNow pulls, before the deadline, the transactions of the validators that said they had some
// at the height, and reports whether the pool took any: the proposer that found none for its block
// tries again with them
func (p *bftPending) pullNow(height uint64, deadline time.Time) bool {
	p.mu.Lock()
	if height != p.height {
		p.mu.Unlock()
		return false
	}
	var keys [][]byte
	now := p.now()
	for _, v := range p.sources() {
		key, _ := p.set.Key(v)
		keys = append(keys, key)
		p.pulled[v] = now
	}
	p.mu.Unlock()
	for _, key := range keys {
		timeout := min(p.timeout, deadline.Sub(p.now()))
		if timeout <= 0 {
			return false
		}
		if bodies, err := p.fetch(key, timeout); err == nil && p.receive(bodies) > 0 {
			return true
		}
	}
	return false
}

// announce says to the others that the node has transactions at the height, once a bftResend:
// the driver calls it while the height is undecided and the pool of the node has transactions
func (p *bftPending) announce(height uint64) {
	p.mu.Lock()
	if !p.member || height != p.height {
		p.mu.Unlock()
		return
	}
	now := p.now()
	if p.said == height && now.Sub(p.saidAt) < bftResend {
		p.mu.Unlock()
		return
	}
	p.said, p.saidAt = height, now
	self := p.self
	p.mu.Unlock()
	p.say(height, self)
}
