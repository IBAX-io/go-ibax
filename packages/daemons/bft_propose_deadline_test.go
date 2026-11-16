/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
)

// The watch takes the time the precommits of a quorum for one block of its height reached the node:
// precommits of nil, of other blocks or rounds, and repeated ones do not add up; decided tells it
// for the height the watch moved on from, only as the watch moves to the next height
func TestBftWatchDecided(t *testing.T) {
	d, _ := alertDriver(t)
	set := alertSet(4)
	t0 := time.Now()
	d.watch.moveTo(30, set, t0, true)
	block := bft.Hash{1}
	other := bft.Hash{2}
	vote := func(at time.Duration, round uint32, hash bft.Hash, v uint32) {
		d.watch.precommit(&bft.Vote{Type: bft.TypePrecommit, Height: 30, Round: round, Hash: hash, Validator: v}, t0.Add(at))
	}
	vote(100*time.Millisecond, 0, block, 0)
	vote(200*time.Millisecond, 0, block, 0)
	vote(300*time.Millisecond, 0, bft.Nil, 1)
	vote(400*time.Millisecond, 0, other, 2)
	vote(500*time.Millisecond, 1, block, 3)
	vote(600*time.Millisecond, 0, block, 1)
	d.watch.precommit(&bft.Vote{Type: bft.TypePrevote, Height: 30, Hash: block, Validator: 2}, t0.Add(700*time.Millisecond))
	d.watch.precommit(&bft.Vote{Type: bft.TypePrecommit, Height: 31, Hash: block, Validator: 2}, t0.Add(700*time.Millisecond))
	if got := d.watch.decided(30); !got.IsZero() {
		t.Fatalf("decided %v before the watch moved", got)
	}
	vote(800*time.Millisecond, 0, block, 3)
	vote(900*time.Millisecond, 0, block, 2)
	d.watch.moveTo(31, set, t0.Add(time.Second), true)
	if got, want := d.watch.decided(30), t0.Add(800*time.Millisecond); !got.Equal(want) {
		t.Fatalf("decided %v, want the third distinct precommit for the block at %v", got, want)
	}
	if got := d.watch.decided(31); !got.IsZero() {
		t.Fatalf("decided %v of the height the watch is at", got)
	}

	// no quorum seen: the watch moves on with no time
	d.watch.moveTo(32, set, t0.Add(2*time.Second), true)
	if got := d.watch.decided(31); !got.IsZero() {
		t.Fatalf("decided %v of a height with no quorum seen", got)
	}
	// a quorum, then a jump of heights: the time is of no height before the next
	vote2 := func(v uint32) {
		d.watch.precommit(&bft.Vote{Type: bft.TypePrecommit, Height: 32, Hash: block, Validator: v}, t0.Add(3*time.Second))
	}
	vote2(0)
	vote2(1)
	vote2(2)
	d.watch.moveTo(40, set, t0.Add(4*time.Second), false)
	if got := d.watch.decided(39); !got.IsZero() {
		t.Fatalf("decided %v after a jump of heights", got)
	}
}

// The block of a proposal has max_block_generation_time, cut to the deadline, and at least
// 1/proposeFloor of it, late below that
func TestProposeLimit(t *testing.T) {
	const block = 2 * time.Second
	t0 := time.Now()
	for _, c := range []struct {
		name            string
		collect, budget time.Duration
		limit           time.Duration
		late            bool
	}{
		{"on time", 0, 3 * time.Second, block, false},
		{"waited for the clock", time.Second, 3 * time.Second, block, false},
		{"lagged", 1500 * time.Millisecond, 3 * time.Second, 1500 * time.Millisecond, false},
		// h270 r0 of the 4000 tx/s run on 3 cores a validator: the validators started at the
		// quorum of 269, the proposer collected 2.06 s later
		{"h270", 2060 * time.Millisecond, 3 * time.Second, 940 * time.Millisecond, false},
		{"at the floor", 2800 * time.Millisecond, 3 * time.Second, block / proposeFloor, false},
		{"below the floor", 2900 * time.Millisecond, 3 * time.Second, block / proposeFloor, true},
		{"past the deadline", 4 * time.Second, 3 * time.Second, block / proposeFloor, true},
	} {
		limit, late := proposeLimit(t0.Add(c.collect), t0.Add(c.budget), block)
		if limit != c.limit || late != c.late {
			t.Errorf("%s: limit %v late %v, want %v %v", c.name, limit, late, c.limit, c.late)
		}
	}
}

// The validators start a round as their precommit timeout of the round before expires: the
// driver takes the time it was due as the start of the next round of its height, and of no other
func TestBftRoundStartOfPrecommitTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := newSignDriver(t, ctx, false)
	d.apply(d.m.Start())
	height, round := d.m.Height(), d.m.Round()
	due := time.Now().Add(-300 * time.Millisecond)

	d.expired(bftExpired{Timeout: bft.Timeout{Height: height, Round: round + 3, Step: bft.StepPrecommit}, due: due})
	d.expired(bftExpired{Timeout: bft.Timeout{Height: height, Round: round, Step: bft.StepPrevote}, due: due})
	if got := d.roundStartOf(height, round+1); !got.IsZero() {
		t.Fatalf("the start %v of a round of a timeout of another round or step", got)
	}
	d.expired(bftExpired{Timeout: bft.Timeout{Height: height, Round: round, Step: bft.StepPrecommit}, due: due})
	if got := d.roundStartOf(height, round+1); !got.Equal(due) {
		t.Fatalf("the start of round %d is %v, want %v", round+1, got, due)
	}
	if d.m.Round() != round+1 {
		t.Fatalf("the machine is at round %d after the precommit timeout of round %d", d.m.Round(), round)
	}
	if got := d.roundStartOf(height, round+2); !got.IsZero() {
		t.Fatalf("the start %v of a round no timeout started", got)
	}
	if got := d.roundStartOf(height+1, round+1); !got.IsZero() {
		t.Fatalf("the start %v of a round of another height", got)
	}
}
