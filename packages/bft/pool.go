/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import "math"

// The pool admits the consensus messages a node takes from the network (design 4.6), before
// any of them reaches the machine, and keeps those it admitted: the node relays a message the
// first time the pool admits it, resends the messages of its height until it commits, and gives
// the machine the messages of the next height when it moves there. The pool does no I/O.
//
// A message is admitted only if all of these hold, the cheap ones first and the signature last:
// its height is h−1, h or h+1, for the current height h; its round is at most the base round of
// its height plus the future rounds (h−1: the round it committed at; h: the round of the machine;
// h+1: 0); its validator is in the set of its height, and the proposer of the height and the
// round for a proposal; the pool keeps less than 2 messages of its validator, height, round and
// type, and none for the same hash; its signature verifies by the key of its validator. The set
// of h+1 is known only once h is committed: until then the pool takes it to be the set of h, and
// checks the messages of h+1 again by the set of h+1 when the node moves there.
//
// The memory of the pool is bounded: at most 2 × 3 × N × (round + future rounds + 1) votes and
// 2 × (round + future rounds + 1) proposals for each of its 3 heights.

// Admission is what the pool did with a message
type Admission uint8

const (
	// Dropped is a message the pool refused, or one it has already
	Dropped Admission = iota
	// Admitted is the first message of its validator, height, round and type
	Admitted
	// Conflicting is a second message of its validator, height, round and type for another
	// hash: the node relays it and gives it to the machine, which makes the evidence
	Conflicting
)

type poolKey struct {
	round     uint32
	t         MsgType
	validator uint32
}

type poolHeight struct {
	set       Validators
	base      uint32
	proposals map[poolKey][]*Proposal
	votes     map[poolKey][]*Vote
	order     []any // the admitted messages in the order the pool admitted them
}

func newPoolHeight(set Validators, base uint32) *poolHeight {
	return &poolHeight{set: set, base: base, proposals: map[poolKey][]*Proposal{}, votes: map[poolKey][]*Vote{}}
}

// Pool is the admitted messages of the heights h−1, h and h+1
type Pool struct {
	networkID       uint64
	maxFutureRounds uint32
	height          uint64
	heights         map[uint64]*poolHeight
}

// NewPool is the pool of a node at height h with the validators set of h, on the chain networkID;
// maxFutureRounds is bft_max_future_rounds
func NewPool(networkID uint64, maxFutureRounds uint32, height uint64, set Validators) *Pool {
	p := &Pool{networkID: networkID, maxFutureRounds: maxFutureRounds, height: height, heights: map[uint64]*poolHeight{}}
	p.heights[height] = newPoolHeight(set, 0)
	p.heights[height+1] = newPoolHeight(set, 0)
	return p
}

// Height is the current height of the pool
func (p *Pool) Height() uint64 { return p.height }

// SetRound moves the round window of the current height to the round of the machine
func (p *Pool) SetRound(round uint32) {
	if h := p.heights[p.height]; round > h.base {
		h.base = round
	}
}

// Advance moves the pool to height h+1 after the node committed h at the round committed;
// next is the set of h+1. The messages of h are kept as those of h−1, the ones of h−2 dropped,
// and the messages kept for h+1 are checked again by next: Advance returns those it keeps, in
// the order the pool admitted them, for the machine of h+1.
func (p *Pool) Advance(committed uint32, next Validators) []any {
	cur := p.heights[p.height]
	cur.base = committed
	delete(p.heights, p.height-1)
	held := p.heights[p.height+1]
	p.height++
	p.heights[p.height] = newPoolHeight(next, 0)
	p.heights[p.height+1] = newPoolHeight(next, 0)
	var kept []any
	for _, m := range held.order {
		switch m := m.(type) {
		case *Proposal:
			if p.AddProposal(m) != Dropped {
				kept = append(kept, m)
			}
		case *Vote:
			if p.AddVote(m) != Dropped {
				kept = append(kept, m)
			}
		}
	}
	return kept
}

// Messages is the messages the pool admitted for a height, in the order it admitted them: the
// node resends those of its height until it commits it
func (p *Pool) Messages(height uint64) []any {
	h := p.heights[height]
	if h == nil {
		return nil
	}
	return append([]any(nil), h.order...)
}

// window is the height of a message and its set, if the height and the round are in the windows
func (p *Pool) window(height uint64, round uint32) *poolHeight {
	h := p.heights[height]
	if h == nil || round > math.MaxInt32 {
		return nil
	}
	if uint64(round) > uint64(h.base)+uint64(p.maxFutureRounds) {
		return nil
	}
	return h
}

// AddProposal admits a proposal or drops it
func (p *Pool) AddProposal(m *Proposal) Admission {
	h := p.window(m.Height, m.Round)
	if h == nil || m.Validator != h.set.Proposer(m.Height, m.Round) {
		return Dropped
	}
	key, ok := h.set.Key(m.Validator)
	if !ok {
		return Dropped
	}
	k := poolKey{m.Round, TypeProposal, m.Validator}
	kept := h.proposals[k]
	if len(kept) >= 2 || len(kept) == 1 && kept[0].Hash == m.Hash {
		return Dropped
	}
	if m.Verify(key, p.networkID) != nil {
		return Dropped
	}
	h.proposals[k] = append(kept, m)
	h.order = append(h.order, m)
	if len(kept) == 0 {
		return Admitted
	}
	return Conflicting
}

// AddVote admits a prevote or a precommit or drops it
func (p *Pool) AddVote(m *Vote) Admission {
	if m.Type != TypePrevote && m.Type != TypePrecommit {
		return Dropped
	}
	h := p.window(m.Height, m.Round)
	if h == nil {
		return Dropped
	}
	key, ok := h.set.Key(m.Validator)
	if !ok {
		return Dropped
	}
	k := poolKey{m.Round, m.Type, m.Validator}
	kept := h.votes[k]
	if len(kept) >= 2 || len(kept) == 1 && kept[0].Hash == m.Hash {
		return Dropped
	}
	if m.Verify(key, p.networkID) != nil {
		return Dropped
	}
	h.votes[k] = append(kept, m)
	h.order = append(h.order, m)
	if len(kept) == 0 {
		return Admitted
	}
	return Conflicting
}
