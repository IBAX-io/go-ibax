/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"errors"
	"reflect"
	"testing"
)

// restart is the machine of validator 0 after a restart: a new machine of its keys that restores
// the stored form of the state
func (c *corner) restart(s *SignState) {
	c.t.Helper()
	s, err := UnmarshalSignState(s.Marshal(), 1<<20)
	if err != nil {
		c.t.Fatal(err)
	}
	c.m = NewMachine(DefaultConfig, c.app, c.vs[0].private, c.vs[0].public, testNetwork, 1, c.set)
	c.out = nil
	c.take(c.m.Restore(s))
}

func (c *corner) wantNoVoteOf(round uint32) {
	c.t.Helper()
	for _, o := range c.out {
		if o.Vote != nil && o.Vote.Round == round || o.Proposal != nil && o.Proposal.Round == round {
			c.t.Fatalf("a message of round %d after the restart", round)
		}
	}
}

// The machine takes a state when it signed, locked or took a valid value, and only then; a node
// that is no validator has none
func TestTakeSignState(t *testing.T) {
	c := newCorner(t)
	if c.m.TakeSignState() != nil {
		t.Fatal("a state of a machine that signed nothing")
	}
	c.lockA()
	s := c.m.TakeSignState()
	if s == nil || s.Height != 1 || s.Round != 0 || s.LockedRound != 0 || s.Locked.Hash != hashA || s.ValidRound != 0 {
		t.Fatalf("the state %+v", s)
	}
	if len(s.Votes) != 2 || s.Votes[0].Type != TypePrevote || s.Votes[1].Type != TypePrecommit || len(s.Proposals) != 0 {
		t.Fatalf("%d votes and %d proposals", len(s.Votes), len(s.Proposals))
	}
	if c.m.TakeSignState() != nil {
		t.Fatal("a second state with no change")
	}
	c.vote(3, TypePrevote, 0, hashA)
	if c.m.TakeSignState() != nil {
		t.Fatal("a state on the vote of another validator")
	}

	vs, set := testSet(t, 4)
	others, _ := testSet(t, 1)
	m := NewMachine(DefaultConfig, &cornerApp{}, others[0].private, others[0].public, testNetwork, 1, set)
	m.Start()
	v := &Vote{Type: TypePrevote, Height: 1, Round: 0, Validator: 1, Hash: hashA}
	if err := v.Sign(vs[1].private, testNetwork); err != nil {
		t.Fatal(err)
	}
	m.OnVote(v)
	if m.TakeSignState() != nil {
		t.Fatal("a state of a node that is no validator")
	}
}

// A machine restored after it locked on A in round 0 signs nothing of round 0 again, starts round
// 1, and keeps its lock: it prevotes nil for B there
func TestRestoreKeepsTheLock(t *testing.T) {
	c := newCorner(t)
	c.lockA()
	c.restart(c.m.TakeSignState())
	if c.m.Round() != 1 {
		t.Fatalf("restored at round %d, want 1", c.m.Round())
	}
	if r, h := c.m.Locked(); r != 0 || h != hashA {
		t.Fatalf("locked %d %x, want 0 A", r, h[:2])
	}
	// the messages of round 0 again, and prevotes for B: the machine signs none
	c.proposal(0, NoRound, hashA)
	c.vote(1, TypePrevote, 0, hashB)
	c.vote(2, TypePrevote, 0, hashB)
	c.vote(3, TypePrevote, 0, hashB)
	c.timeout(0, StepPropose)
	c.timeout(0, StepPrevote)
	c.wantNoVoteOf(0)
	c.proposal(1, NoRound, hashB)
	c.wantVote(TypePrevote, 1, Nil)
	// its own votes of round 0 count: with those of 1 and 2 they are a quorum of precommits for A
	c.vote(1, TypePrecommit, 0, hashA)
	c.vote(2, TypePrecommit, 0, hashA)
	if !c.m.Decided() {
		t.Fatal("the restored precommit of round 0 does not count")
	}
}

// A proposer restored with a valid value proposes it with its valid round (L15, L16, L19)
func TestRestoreProposesTheValidValue(t *testing.T) {
	c := newCorner(t)
	c.lockA()
	s := c.m.TakeSignState()
	// the proposer of round 3 is validator 0: a state of round 2 restarts there
	s.Round = 2
	c.restart(s)
	if c.app.values != 0 {
		t.Fatal("the proposer asks for a new block though it has a valid value")
	}
	var p *Proposal
	for _, o := range c.out {
		if o.Proposal != nil {
			p = o.Proposal
		}
	}
	if p == nil || p.Round != 3 || p.Hash != hashA || p.ValidRound != 0 {
		t.Fatalf("the proposal %+v", p)
	}
	if c.m.TakeSignState() == nil {
		t.Fatal("no state after the proposal")
	}
}

// The stored form decodes to the state; a state of another height restores nothing; a malformed
// form is refused
func TestSignStateForm(t *testing.T) {
	c := newCorner(t)
	c.lockA()
	s := c.m.TakeSignState()
	s.Proposals = []*Proposal{s.Locked}
	got, err := UnmarshalSignState(s.Marshal(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Fatalf("decoded %+v, want %+v", got, s)
	}
	empty := &SignState{Height: 7, LockedRound: NoRound, ValidRound: NoRound}
	if got, err := UnmarshalSignState(empty.Marshal(), 1<<20); err != nil || !reflect.DeepEqual(got, empty) {
		t.Fatalf("the empty state: %+v %v", got, err)
	}

	m := NewMachine(DefaultConfig, c.app, c.vs[0].private, c.vs[0].public, testNetwork, 2, c.set)
	if out := m.Restore(s); out != nil || m.Round() != 0 {
		t.Fatal("a state of height 1 restored at height 2")
	}

	b := s.Marshal()
	if _, err := UnmarshalSignState(append(b, 0), 1<<20); !errors.Is(err, ErrFormat) {
		t.Fatalf("a byte left over: %v", err)
	}
	if _, err := UnmarshalSignState(b[:len(b)-1], 1<<20); !errors.Is(err, ErrFormat) {
		t.Fatalf("a byte short: %v", err)
	}
	if _, err := UnmarshalSignState(b, 1); !errors.Is(err, ErrFormat) {
		t.Fatalf("a block past the size: %v", err)
	}
	bad := &SignState{Height: 7, LockedRound: 0, ValidRound: NoRound}
	if _, err := UnmarshalSignState(bad.Marshal(), 1<<20); !errors.Is(err, ErrFormat) {
		t.Fatalf("a locked round of nil: %v", err)
	}
}
