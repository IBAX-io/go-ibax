/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import "testing"

// alone is the machine of the only validator of a set, whose blocks are pending
func alone(t *testing.T) *corner {
	vs, set := testSet(t, 1)
	c := &corner{t: t, vs: vs, set: set, app: &cornerApp{invalid: map[Hash]bool{}, untimely: map[Hash]bool{}, pending: true}}
	c.m = NewMachine(DefaultConfig, c.app, vs[0].private, vs[0].public, testNetwork, 1, set)
	return c
}

func (c *corner) proposed(round uint32) bool {
	for _, o := range c.out {
		if o.Proposal != nil && o.Proposal.Round == round {
			return true
		}
	}
	return false
}

func (c *corner) scheduled(round uint32, s Step) bool {
	for _, o := range c.out {
		if o.Timeout != nil && o.Timeout.Round == round && o.Timeout.Step == s {
			return true
		}
	}
	return false
}

// TestPendingBlockWaitsForValidity checks a proposer sends its pending block at once and
// prevotes it only once the node tells it is valid: the only validator decides a valid block
func TestPendingBlockWaitsForValidity(t *testing.T) {
	c := alone(t)
	own := testHash(0xaa)
	c.take(c.m.Start())
	if !c.proposed(0) {
		t.Fatal("the proposal of a pending block waits")
	}
	c.wantNoVote(TypePrevote, 0)
	if !c.scheduled(0, StepPropose) {
		t.Fatal("a proposer of a pending block schedules no propose timeout")
	}
	if c.m.Decided() {
		t.Fatal("a pending block is decided")
	}
	c.take(c.m.OnValid(1, testHash(0xbb), ValidityValid))
	c.take(c.m.OnValid(2, own, ValidityValid))
	c.wantNoVote(TypePrevote, 0)
	c.take(c.m.OnValid(1, own, ValidityValid))
	c.wantVote(TypePrevote, 0, own)
	c.wantVote(TypePrecommit, 0, own)
	if !c.m.Decided() {
		t.Fatal("the only validator did not decide its valid block")
	}
}

// TestPendingBlockInvalidNotDecided checks the only validator whose block fails after its
// proposal, as its writes fail, prevotes nil and never decides the block
func TestPendingBlockInvalidNotDecided(t *testing.T) {
	c := alone(t)
	own := testHash(0xaa)
	c.take(c.m.Start())
	c.take(c.m.OnValid(1, own, ValidityInvalid))
	c.wantVote(TypePrevote, 0, Nil)
	c.wantVote(TypePrecommit, 0, Nil)
	if c.m.Decided() {
		t.Fatal("a block that failed is decided")
	}
	// the next round proposes again; the block of the same hash is still not valid
	c.timeout(0, StepPrecommit)
	if !c.proposed(1) {
		t.Fatal("no proposal in the next round")
	}
	c.take(c.m.OnValid(1, own, ValidityInvalid))
	c.wantVote(TypePrevote, 1, Nil)
	if c.m.Decided() {
		t.Fatal("a block that failed is decided")
	}
}

// TestPendingBlockTimesOut checks a proposer whose block is still pending at its propose timeout
// prevotes nil, as a validator without the proposal does, and its late validity changes nothing
func TestPendingBlockTimesOut(t *testing.T) {
	c := alone(t)
	own := testHash(0xaa)
	c.take(c.m.Start())
	c.timeout(0, StepPropose)
	c.wantVote(TypePrevote, 0, Nil)
	c.take(c.m.OnValid(1, own, ValidityValid))
	c.wantVote(TypePrevote, 0, Nil)
	if c.m.Decided() {
		t.Fatal("a block prevoted nil is decided")
	}
}

// TestPendingBlockUnknownAsksValid checks a block whose check the node gave up, as it dropped its
// trial, is checked by Valid once the machine needs to know
func TestPendingBlockUnknownAsksValid(t *testing.T) {
	c := alone(t)
	own := testHash(0xaa)
	c.take(c.m.Start())
	c.app.invalid[own] = true
	c.take(c.m.OnValid(1, own, ValidityUnknown))
	c.wantVote(TypePrevote, 0, Nil)
	if c.m.Decided() {
		t.Fatal("a block Valid refuses is decided")
	}

	c = alone(t)
	c.take(c.m.Start())
	c.take(c.m.OnValid(1, own, ValidityUnknown))
	c.wantVote(TypePrevote, 0, own)
	if !c.m.Decided() {
		t.Fatal("a block Valid takes is not decided")
	}
}

// TestPendingBlockOutlastsRounds checks the only validator whose writes outlast its propose
// timeout proposes the same block in the next round, and decides it there once its writes end,
// whether they end in that round or before it starts
func TestPendingBlockOutlastsRounds(t *testing.T) {
	own := testHash(0xaa)
	for _, before := range []bool{false, true} {
		c := alone(t)
		c.take(c.m.Start())
		c.timeout(0, StepPropose)
		c.wantVote(TypePrevote, 0, Nil)
		c.wantVote(TypePrecommit, 0, Nil)
		if before {
			// the writes end in round 0, after its nil prevote
			c.take(c.m.OnValid(1, own, ValidityValid))
		}
		c.timeout(0, StepPrecommit)
		if !c.proposed(1) {
			t.Fatal("no proposal in the next round")
		}
		if !before {
			c.wantNoVote(TypePrevote, 1)
			c.take(c.m.OnValid(1, own, ValidityValid))
		}
		c.wantVote(TypePrevote, 1, own)
		c.wantVote(TypePrecommit, 1, own)
		if !c.m.Decided() {
			t.Fatal("the block whose writes outlasted a round is not decided")
		}
	}
}
