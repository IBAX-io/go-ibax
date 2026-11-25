/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/consts"
)

// The pace of the proposals (direction A, a proposer policy, not a rule of validity). Every
// validator executes every block, and the proposer of h and its validators wait for their own
// executors to have executed h−BftExecDepth. A proposer that sized its block to its own executor
// alone gave the slower validators more than they execute in a block interval: they fell behind
// block after block, and when one of them proposed, it waited for its executor past the propose
// timeout. So each validator says, after each block it executed, its executed height and the
// rate of its executor (bft.ExecStatus), and a proposer sizes its block to the slowest rate of the
// validators, and to half of it while a validator is behind, until it caught up.
//
// A validator slower than 1/bftPaceSpread of the proposer is not counted: a Byzantine validator
// that says a low rate makes the blocks at most bftPaceSpread times smaller, and an honest one that
// slow is degraded and falls behind as it would on any chain whose load it cannot execute.

const (
	// bftPaceFresh is how long the execution status of a validator counts: a validator silent
	// longer, stopped or cut off, does not hold the blocks back
	bftPaceFresh = 10 * time.Second
	// bftPaceSpread is how many times slower than the proposer a validator may be and still count
	bftPaceSpread = 2
	// bftPaceDrain is the part of the slowest rate a proposer sizes its block to while a validator
	// is behind
	bftPaceDrain = 0.5
)

// bftPaceReport is the last execution status of a validator
type bftPaceReport struct {
	height int64
	rate   float64
	at     time.Time
}

// bftPace is the execution statuses the node took, by the node public keys of the validators
type bftPace struct {
	mu      sync.Mutex
	reports map[string]bftPaceReport
}

// newer reports whether an execution status of the key at the height would be newer than the one
// the node has, before the node verifies its signature
func (p *bftPace) newer(key []byte, height int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.reports[string(key)]
	return !ok || height > r.height
}

// take keeps the execution status of the key, unless the node has a newer one
func (p *bftPace) take(key []byte, height int64, rate float64, at time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reports == nil {
		p.reports = make(map[string]bftPaceReport)
	}
	if r, ok := p.reports[string(key)]; ok && height <= r.height {
		return
	}
	p.reports[string(key)] = bftPaceReport{height: height, rate: rate, at: at}
}

// rate is the transactions a second a proposer of the key whose executor runs at own sizes its
// block to, after the committed height: the slowest of own and of the validators of the set that
// said their execution within bftPaceFresh and run at own/bftPaceSpread at least, times
// bftPaceDrain if one of them is more than BftExecDepth blocks behind the committed height. It
// reports whether one was behind.
func (p *bftPace) rate(set bft.Validators, self []byte, own float64, committed int64, now time.Time) (float64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	rate, behind := own, false
	for i := range uint32(set.N()) {
		key, _ := set.Key(i)
		r, ok := p.reports[string(key)]
		if !ok || string(key) == string(self) || now.Sub(r.at) > bftPaceFresh || r.rate*bftPaceSpread < own {
			continue
		}
		rate = min(rate, r.rate)
		if committed-r.height > consts.BftExecDepth {
			behind = true
		}
	}
	if behind {
		rate *= bftPaceDrain
	}
	return rate, behind
}
