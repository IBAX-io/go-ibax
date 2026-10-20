/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"errors"
	"testing"
)

// The corner cases of the paper (design section 8, item 13), each driving the machine of
// validator 0 of 4 at height 1 by signed messages of the others. The proposers of height 1 are
// validators 1, 2, 3 and 0 in rounds 0 to 3.

type cornerApp struct {
	invalid map[Hash]bool
	values  int
}

func (a *cornerApp) Value(height uint64, round uint32) ([]byte, Hash, error) {
	a.values++
	return []byte("own"), testHash(0xaa), nil
}

func (a *cornerApp) Valid(height uint64, hash Hash, block []byte) bool {
	return !a.invalid[hash]
}

type corner struct {
	t   *testing.T
	vs  []testValidator
	set Validators
	app *cornerApp
	m   *Machine
	out []Output
}

var hashA, hashB = testHash(0xa1), testHash(0xb2)

func newCorner(t *testing.T) *corner {
	vs, set := testSet(t, 4)
	c := &corner{t: t, vs: vs, set: set, app: &cornerApp{invalid: map[Hash]bool{}}}
	c.m = NewMachine(DefaultConfig, c.app, vs[0].private, vs[0].public, testNetwork, 1, set)
	return c
}

func (c *corner) take(out []Output) {
	c.out = append(c.out, out...)
}

func (c *corner) proposal(round uint32, validRound int32, hash Hash) {
	c.t.Helper()
	p := &Proposal{Height: 1, Round: round, ValidRound: validRound, Validator: c.set.Proposer(1, round), Hash: hash, Block: hash[:]}
	if err := p.Sign(c.vs[p.Validator].private, testNetwork); err != nil {
		c.t.Fatal(err)
	}
	c.take(c.m.OnProposal(p))
}

func (c *corner) vote(validator uint32, t MsgType, round uint32, hash Hash) {
	c.t.Helper()
	v := &Vote{Type: t, Height: 1, Round: round, Validator: validator, Hash: hash}
	if err := v.Sign(c.vs[validator].private, testNetwork); err != nil {
		c.t.Fatal(err)
	}
	c.take(c.m.OnVote(v))
}

func (c *corner) timeout(round uint32, s Step) {
	c.take(c.m.OnTimeout(Timeout{Height: 1, Round: round, Step: s}))
}

// voted is the hash of the last vote of the type the machine sent in the round, and whether it
// sent one
func (c *corner) voted(t MsgType, round uint32) (Hash, bool) {
	for i := len(c.out) - 1; i >= 0; i-- {
		if v := c.out[i].Vote; v != nil && v.Type == t && v.Round == round {
			return v.Hash, true
		}
	}
	return Nil, false
}

func (c *corner) wantVote(t MsgType, round uint32, want Hash) {
	c.t.Helper()
	got, ok := c.voted(t, round)
	if !ok || got != want {
		c.t.Fatalf("%s of round %d: %x (sent %v), want %x", t, round, got[:2], ok, want[:2])
	}
}

func (c *corner) wantNoVote(t MsgType, round uint32) {
	c.t.Helper()
	if got, ok := c.voted(t, round); ok {
		c.t.Fatalf("%s of round %d: %x, want none", t, round, got[:2])
	}
}

// lockA locks validator 0 on A in round 0: the proposal of A and the prevotes of 1 and 2
func (c *corner) lockA() {
	c.t.Helper()
	c.take(c.m.Start())
	c.proposal(0, NoRound, hashA)
	c.wantVote(TypePrevote, 0, hashA)
	c.vote(1, TypePrevote, 0, hashA)
	c.vote(2, TypePrevote, 0, hashA)
	c.wantVote(TypePrecommit, 0, hashA)
	if r, h := c.m.Locked(); r != 0 || h != hashA {
		c.t.Fatalf("locked %d %x, want 0 A", r, h[:2])
	}
}

// nextRound leaves round r by the precommits of 1 and 2 for nil and the timeout of L47
func (c *corner) nextRound(r uint32) {
	c.t.Helper()
	c.vote(1, TypePrecommit, r, Nil)
	c.vote(2, TypePrecommit, r, Nil)
	if _, ok := c.voted(TypePrecommit, r); !ok {
		c.vote(3, TypePrecommit, r, Nil)
	}
	c.timeout(r, StepPrecommit)
	if c.m.Round() != r+1 {
		c.t.Fatalf("round %d, want %d", c.m.Round(), r+1)
	}
}

func TestCornerNotStarted(t *testing.T) {
	c := newCorner(t)
	if c.m.Round() != 0 || c.m.Step() != StepPropose {
		t.Fatal("a new height")
	}
	// the machine starts on a message of the height
	c.vote(1, TypePrevote, 0, hashA)
	if !c.m.started {
		t.Fatal("a vote of the height does not start it")
	}
	c2 := newCorner(t)
	c2.take(c2.m.Start())
	if len(c2.out) != 1 || c2.out[0].Timeout == nil || c2.out[0].Timeout.Step != StepPropose {
		t.Fatalf("a validator that is not the proposer starts with %+v, want the timeout of L21", c2.out)
	}
}

func TestCornerLockedPrevotesNil(t *testing.T) {
	// L22-L26: a validator locked on A prevotes nil for B proposed without a validRound
	c := newCorner(t)
	c.lockA()
	c.nextRound(0)
	c.proposal(1, NoRound, hashB)
	c.wantVote(TypePrevote, 1, Nil)
	if r, h := c.m.Locked(); r != 0 || h != hashA {
		t.Fatalf("locked %d %x, want 0 A", r, h[:2])
	}
	// L23 lockedValue_p = v: and prevotes A proposed again without a validRound
	c = newCorner(t)
	c.lockA()
	c.nextRound(0)
	c.proposal(1, NoRound, hashA)
	c.wantVote(TypePrevote, 1, hashA)
}

// prevotes sends prevotes of a round for a hash from validators
func (c *corner) prevotes(round uint32, hash Hash, validators ...uint32) {
	for _, v := range validators {
		c.vote(v, TypePrevote, round, hash)
	}
}

func TestCornerProposalOfAHigherValidRound(t *testing.T) {
	// L28-L30: locked on A in round 0, the validator prevotes B proposed in round 2 with
	// validRound 1 when it has the prevotes of a quorum for B in round 1, as lockedRound 0 <= 1
	c := newCorner(t)
	c.lockA()
	c.nextRound(0)
	c.timeout(1, StepPropose) // the proposal of round 1 does not come: prevote nil
	c.wantVote(TypePrevote, 1, Nil)
	c.prevotes(1, hashB, 1, 2, 3)
	c.timeout(1, StepPrevote)
	c.wantVote(TypePrecommit, 1, Nil)
	c.nextRound(1)
	c.proposal(2, 1, hashB)
	c.wantVote(TypePrevote, 2, hashB)
}

func TestCornerProposalOfAValidRoundWithoutQuorum(t *testing.T) {
	// L28 waits for a quorum of prevotes in validRound: with 2 of 3 the validator does not
	// prevote, and prevotes nil at the timeout of L57
	c := newCorner(t)
	c.lockA()
	c.nextRound(0)
	c.timeout(1, StepPropose)
	c.prevotes(1, hashB, 1, 2)
	c.prevotes(1, Nil, 3)
	c.timeout(1, StepPrevote)
	c.nextRound(1)
	c.proposal(2, 1, hashB)
	c.wantNoVote(TypePrevote, 2)
	c.timeout(2, StepPropose)
	c.wantVote(TypePrevote, 2, Nil)
}

func TestCornerLockAboveValidRound(t *testing.T) {
	// L29-L32: locked on A in round 2, the validator prevotes nil for B proposed in round 3 with
	// validRound 1, though a quorum prevoted B in round 1, as lockedRound 2 > 1. The proposer of
	// round 3 is validator 0 itself, so the case runs at height 2, whose proposers are 2, 3, 0, 1.
	vs, set := testSet(t, 4)
	app := &cornerApp{invalid: map[Hash]bool{}}
	m := NewMachine(DefaultConfig, app, vs[0].private, vs[0].public, testNetwork, 2, set)
	var out []Output
	send := func(o []Output) { out = append(out, o...) }
	prop := func(round uint32, vr int32, hash Hash) {
		p := &Proposal{Height: 2, Round: round, ValidRound: vr, Validator: set.Proposer(2, round), Hash: hash, Block: hash[:]}
		if err := p.Sign(vs[p.Validator].private, testNetwork); err != nil {
			t.Fatal(err)
		}
		send(m.OnProposal(p))
	}
	vote := func(i uint32, mt MsgType, round uint32, hash Hash) {
		v := &Vote{Type: mt, Height: 2, Round: round, Validator: i, Hash: hash}
		if err := v.Sign(vs[i].private, testNetwork); err != nil {
			t.Fatal(err)
		}
		send(m.OnVote(v))
	}
	send(m.Start())
	// round 0 and 1 pass with prevotes of nil; 1, 2, 3 prevote B in round 1
	send(m.OnTimeout(Timeout{Height: 2, Round: 0, Step: StepPropose}))
	for _, i := range []uint32{1, 2, 3} {
		vote(i, TypePrecommit, 0, Nil)
	}
	send(m.OnTimeout(Timeout{Height: 2, Round: 0, Step: StepPrecommit}))
	send(m.OnTimeout(Timeout{Height: 2, Round: 1, Step: StepPropose}))
	for _, i := range []uint32{1, 2, 3} {
		vote(i, TypePrevote, 1, hashB)
	}
	for _, i := range []uint32{1, 2, 3} {
		vote(i, TypePrecommit, 1, Nil)
	}
	send(m.OnTimeout(Timeout{Height: 2, Round: 1, Step: StepPrecommit}))
	// round 2: validator 0 proposes its own value (L18) and locks it
	if m.Round() != 2 || app.values != 1 {
		t.Fatalf("round %d, %d values", m.Round(), app.values)
	}
	own := testHash(0xaa)
	for _, i := range []uint32{1, 2} {
		vote(i, TypePrevote, 2, own)
	}
	if r, h := m.Locked(); r != 2 || h != own {
		t.Fatalf("locked %d %x, want 2 and its own value", r, h[:2])
	}
	for _, i := range []uint32{1, 2, 3} {
		vote(i, TypePrecommit, 2, Nil)
	}
	send(m.OnTimeout(Timeout{Height: 2, Round: 2, Step: StepPrecommit}))
	prop(3, 1, hashB)
	var last *Vote
	for _, o := range out {
		if o.Vote != nil && o.Vote.Round == 3 && o.Vote.Type == TypePrevote {
			last = o.Vote
		}
	}
	if last == nil || last.Hash != Nil {
		t.Fatalf("the prevote of round 3: %+v, want nil", last)
	}
}

func TestCornerProposerProposesValidValue(t *testing.T) {
	// L15-L19: the proposer with a valid value proposes it with its validRound, not a new value
	c := newCorner(t)
	c.lockA()
	c.nextRound(0)
	c.nextRound(1)
	c.nextRound(2)
	var p *Proposal
	for _, o := range c.out {
		if o.Proposal != nil {
			p = o.Proposal
		}
	}
	if p == nil || p.Round != 3 || p.Hash != hashA || p.ValidRound != 0 || c.app.values != 0 {
		t.Fatalf("the proposal of round 3: %+v, %d new values; want A of validRound 0", p, c.app.values)
	}
	// and prevotes its proposal, as it is locked on it (L28 needs the quorum of round 0, which
	// it has)
	c.wantVote(TypePrevote, 3, hashA)
}

func TestCornerValidValueAtPrecommit(t *testing.T) {
	// L36-L43 at step precommit: the quorum of prevotes for A after the validator precommitted
	// nil updates validValue and validRound, not the lock
	c := newCorner(t)
	c.take(c.m.Start())
	c.timeout(0, StepPropose)
	c.wantVote(TypePrevote, 0, Nil)
	c.prevotes(0, hashA, 1, 2)
	c.timeout(0, StepPrevote)
	c.wantVote(TypePrecommit, 0, Nil)
	c.proposal(0, NoRound, hashA)
	c.prevotes(0, hashA, 3)
	if r, h := c.m.ValidValue(); r != 0 || h != hashA {
		t.Fatalf("valid value %d %x, want 0 A", r, h[:2])
	}
	if r, _ := c.m.Locked(); r != NoRound {
		t.Fatalf("locked in round %d at step precommit", r)
	}
}

func TestCornerRoundSkip(t *testing.T) {
	// L55-L56: f+1 = 2 messages of a higher round move the validator to that round; 1 does not
	c := newCorner(t)
	c.take(c.m.Start())
	c.vote(1, TypePrevote, 5, Nil)
	if c.m.Round() != 0 {
		t.Fatalf("one message of round 5 moves to round %d", c.m.Round())
	}
	c.vote(2, TypePrecommit, 5, Nil)
	if c.m.Round() != 5 || c.m.Step() != StepPropose {
		t.Fatalf("round %d %s, want 5 propose", c.m.Round(), c.m.Step())
	}
	// messages past bft_max_future_rounds are dropped
	c.vote(1, TypePrevote, 5+DefaultConfig.MaxFutureRounds+1, Nil)
	c.vote(2, TypePrevote, 5+DefaultConfig.MaxFutureRounds+1, Nil)
	if c.m.Round() != 5 {
		t.Fatalf("messages past the window move to round %d", c.m.Round())
	}
}

func TestCornerLateCommit(t *testing.T) {
	// L49: a validator in round 2 commits by the precommits of round 0
	c := newCorner(t)
	c.take(c.m.Start())
	c.vote(1, TypePrevote, 2, Nil)
	c.vote(2, TypePrevote, 2, Nil)
	if c.m.Round() != 2 {
		t.Fatalf("round %d", c.m.Round())
	}
	c.proposal(0, NoRound, hashA)
	for _, i := range []uint32{1, 2, 3} {
		c.vote(i, TypePrecommit, 0, hashA)
	}
	var commit *Commit
	for _, o := range c.out {
		if o.Commit != nil {
			commit = o.Commit
		}
	}
	if commit == nil || commit.Round != 0 || commit.Hash != hashA || len(commit.Precommits) != 3 {
		t.Fatalf("commit %+v, want A of round 0 with 3 precommits", commit)
	}
	for _, v := range commit.Precommits {
		if v.Verify(c.set.Keys[v.Validator], testNetwork) != nil {
			t.Fatal("a precommit of the certificate does not verify")
		}
	}
	// a decided machine takes nothing more
	if out := c.m.OnVote(&Vote{Type: TypePrevote, Height: 1, Round: 3, Validator: 1}); out != nil || !c.m.Decided() {
		t.Fatal("a decided machine takes a vote")
	}
}

func TestCornerInvalidBlock(t *testing.T) {
	// L23, L26: a block that is not valid (its time, its transactions) gets a prevote of nil, and
	// no quorum of precommits for it commits it (L50)
	c := newCorner(t)
	c.app.invalid[hashA] = true
	c.take(c.m.Start())
	c.proposal(0, NoRound, hashA)
	c.wantVote(TypePrevote, 0, Nil)
	for _, i := range []uint32{1, 2, 3} {
		c.vote(i, TypePrecommit, 0, hashA)
	}
	if c.m.Decided() {
		t.Fatal("an invalid block committed")
	}
}

func TestCornerEvidence(t *testing.T) {
	c := newCorner(t)
	c.take(c.m.Start())
	// a proposal of a validator that is not the proposer of the round is dropped
	p := &Proposal{Height: 1, Round: 0, ValidRound: NoRound, Validator: 2, Hash: hashB, Block: hashB[:]}
	if err := p.Sign(c.vs[2].private, testNetwork); err != nil {
		t.Fatal(err)
	}
	if out := c.m.OnProposal(p); out != nil || c.m.proposals[0] != nil {
		t.Fatal("a proposal out of turn is taken")
	}
	c.proposal(0, NoRound, hashA)
	c.proposal(0, NoRound, hashB)
	c.vote(2, TypePrevote, 0, hashA)
	c.vote(2, TypePrevote, 0, hashB)
	c.vote(2, TypePrevote, 0, Nil) // the third is dropped and reported no more
	var evs []*Evidence
	for _, o := range c.out {
		if o.Evidence != nil {
			evs = append(evs, o.Evidence)
		}
	}
	if len(evs) != 2 || evs[0].Kind != KindDoubleProposal || evs[1].Kind != KindDoublePrevote {
		t.Fatalf("evidence %+v, want a double proposal and a double prevote", evs)
	}
	chain := &testChain{height: 1, maxAge: 100000, set: c.set, seen: map[EvidenceKey]bool{}}
	for _, e := range evs {
		if _, _, err := CheckEvidence(e.Marshal(), testNetwork, chain); err != nil {
			t.Fatalf("the evidence of a %d: %v", e.Kind, err)
		}
	}
	// the second vote counts not: 1 vote of A from validator 2
	if c.m.count(0, TypePrevote, hashA) != 2 || c.m.count(0, TypePrevote, hashB) != 0 {
		t.Fatalf("prevotes A %d, B %d", c.m.count(0, TypePrevote, hashA), c.m.count(0, TypePrevote, hashB))
	}
	if _, _, err := CheckEvidence(evs[0].Marshal(), testNetwork+1, chain); !errors.Is(err, ErrEvidenceFormat) {
		t.Fatal("evidence of another network")
	}
}
