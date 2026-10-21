/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import "testing"

func testVote(t *testing.T, vs []testValidator, index uint32, mt MsgType, height uint64, round uint32, hash Hash) *Vote {
	t.Helper()
	v := &Vote{Type: mt, Height: height, Round: round, Validator: index, Hash: hash}
	if err := v.Sign(vs[index].private, testNetwork); err != nil {
		t.Fatal(err)
	}
	return v
}

func testProposal(t *testing.T, vs []testValidator, set Validators, height uint64, round uint32, hash Hash) *Proposal {
	t.Helper()
	index := set.Proposer(height, round)
	p := &Proposal{Height: height, Round: round, ValidRound: NoRound, Validator: index, Hash: hash, Block: []byte{1}}
	if err := p.Sign(vs[index].private, testNetwork); err != nil {
		t.Fatal(err)
	}
	return p
}

func requireAdmission(t *testing.T, want, got Admission, what string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: admission %d, want %d", what, got, want)
	}
}

// The pool admits the first message of a validator, height, round and type and a second one for
// another hash, and nothing more of them: one for the hash of a message it has is a duplicate
func TestPoolPerKey(t *testing.T) {
	vs, set := testSet(t, 4)
	p := NewPool(testNetwork, 10, 5, set)
	a := testVote(t, vs, 1, TypePrevote, 5, 0, testHash(1))
	requireAdmission(t, Admitted, p.AddVote(a), "the first prevote")
	requireAdmission(t, Duplicate, p.AddVote(a), "the same prevote")
	requireAdmission(t, Duplicate, p.AddVote(testVote(t, vs, 1, TypePrevote, 5, 0, testHash(1))), "the same hash signed again")
	requireAdmission(t, Conflicting, p.AddVote(testVote(t, vs, 1, TypePrevote, 5, 0, Nil)), "a prevote for nil")
	requireAdmission(t, Dropped, p.AddVote(testVote(t, vs, 1, TypePrevote, 5, 0, testHash(2))), "a third prevote")
	requireAdmission(t, Duplicate, p.AddVote(testVote(t, vs, 1, TypePrevote, 5, 0, Nil)), "the second prevote again")
	requireAdmission(t, Admitted, p.AddVote(testVote(t, vs, 1, TypePrecommit, 5, 0, testHash(2))), "a precommit of the round")
	requireAdmission(t, Admitted, p.AddVote(testVote(t, vs, 1, TypePrevote, 5, 1, testHash(2))), "a prevote of the next round")
	requireAdmission(t, Admitted, p.AddVote(testVote(t, vs, 2, TypePrevote, 5, 0, testHash(1))), "a prevote of another validator")

	pr := testProposal(t, vs, set, 5, 0, testHash(1))
	requireAdmission(t, Admitted, p.AddProposal(pr), "the proposal")
	requireAdmission(t, Duplicate, p.AddProposal(pr), "the same proposal")
	requireAdmission(t, Conflicting, p.AddProposal(testProposal(t, vs, set, 5, 0, testHash(3))), "another proposal")
	requireAdmission(t, Dropped, p.AddProposal(testProposal(t, vs, set, 5, 0, testHash(4))), "a third proposal")
	requireAdmission(t, Duplicate, p.AddProposal(testProposal(t, vs, set, 5, 0, testHash(3))), "the second proposal again")
	if got := len(p.Messages(5)); got != 7 {
		t.Fatalf("%d messages kept, want 7", got)
	}
}

// The pool drops what does not verify, what is not of a validator of the set and a proposal not
// of the proposer
func TestPoolSigners(t *testing.T) {
	vs, set := testSet(t, 4)
	others, _ := testSet(t, 1)
	p := NewPool(testNetwork, 10, 5, set)

	v := testVote(t, vs, 1, TypePrevote, 5, 0, testHash(1))
	v.Signature[3] ^= 1
	requireAdmission(t, Dropped, p.AddVote(v), "a tampered signature")
	v = testVote(t, vs, 1, TypePrevote, 5, 0, testHash(1))
	v.Validator = 2
	requireAdmission(t, Dropped, p.AddVote(v), "the vote of validator 1 as validator 2")
	v = testVote(t, vs, 1, TypePrevote, 5, 0, testHash(1))
	v.Validator = 4
	requireAdmission(t, Dropped, p.AddVote(v), "a validator past the set")
	v = &Vote{Type: TypePrevote, Height: 5, Round: 0, Validator: 1, Hash: testHash(1)}
	if err := v.Sign(others[0].private, testNetwork); err != nil {
		t.Fatal(err)
	}
	requireAdmission(t, Dropped, p.AddVote(v), "a node out of the set")
	v = testVote(t, vs, 1, TypePrevote, 5, 0, testHash(1))
	v.Height = 6
	requireAdmission(t, Dropped, p.AddVote(v), "a vote signed for another height")
	v = testVote(t, vs, 1, TypeProposal, 5, 0, testHash(1))
	requireAdmission(t, Dropped, p.AddVote(v), "a vote of the type of a proposal")

	pr := testProposal(t, vs, set, 5, 0, testHash(1))
	pr.Validator = (pr.Validator + 1) % 4
	if err := pr.Sign(vs[pr.Validator].private, testNetwork); err != nil {
		t.Fatal(err)
	}
	requireAdmission(t, Dropped, p.AddProposal(pr), "a proposal of a validator that is not the proposer")
	pr = testProposal(t, vs, set, 5, 0, testHash(1))
	pr.ValidRound = 0
	requireAdmission(t, Dropped, p.AddProposal(pr), "a proposal signed with another validRound")
	if got := len(p.Messages(5)); got != 0 {
		t.Fatalf("%d messages kept, want 0", got)
	}
	requireAdmission(t, Admitted, p.AddVote(testVote(t, vs, 1, TypePrevote, 5, 0, testHash(1))), "the vote")
}

// The pool keeps the heights h−1, h and h+1, and the rounds up to the base round of each plus the
// future rounds
func TestPoolWindows(t *testing.T) {
	vs, set := testSet(t, 4)
	p := NewPool(testNetwork, 3, 5, set)
	requireAdmission(t, Dropped, p.AddVote(testVote(t, vs, 0, TypePrevote, 4, 0, testHash(1))), "h−1 before the pool committed it")
	requireAdmission(t, Dropped, p.AddVote(testVote(t, vs, 0, TypePrevote, 7, 0, testHash(1))), "h+2")
	requireAdmission(t, Admitted, p.AddVote(testVote(t, vs, 0, TypePrevote, 5, 3, testHash(1))), "round 3 at round 0")
	requireAdmission(t, Dropped, p.AddVote(testVote(t, vs, 0, TypePrevote, 5, 4, testHash(1))), "round 4 at round 0")
	requireAdmission(t, Dropped, p.AddVote(testVote(t, vs, 0, TypePrevote, 5, 1<<31, testHash(1))), "a round past int32")
	p.SetRound(2)
	requireAdmission(t, Admitted, p.AddVote(testVote(t, vs, 0, TypePrevote, 5, 5, testHash(1))), "round 5 at round 2")
	requireAdmission(t, Dropped, p.AddVote(testVote(t, vs, 0, TypePrevote, 5, 6, testHash(1))), "round 6 at round 2")
	p.SetRound(1)
	requireAdmission(t, Admitted, p.AddVote(testVote(t, vs, 1, TypePrevote, 5, 5, testHash(1))), "the window does not move back")

	requireAdmission(t, Admitted, p.AddVote(testVote(t, vs, 0, TypePrevote, 6, 3, testHash(1))), "h+1, round 3")
	requireAdmission(t, Dropped, p.AddVote(testVote(t, vs, 0, TypePrevote, 6, 4, testHash(1))), "h+1, round 4")
	if got := len(p.Advance(2, set)); got != 1 {
		t.Fatalf("%d messages of h+1 kept, want 1", got)
	}
	if p.Height() != 6 {
		t.Fatalf("height %d", p.Height())
	}
	// h−1 is height 5, committed at round 2: its rounds up to 5 come in
	requireAdmission(t, Admitted, p.AddVote(testVote(t, vs, 2, TypePrecommit, 5, 5, testHash(1))), "h−1, round 5")
	requireAdmission(t, Dropped, p.AddVote(testVote(t, vs, 2, TypePrecommit, 5, 6, testHash(1))), "h−1, round 6")
	requireAdmission(t, Admitted, p.AddVote(testVote(t, vs, 0, TypePrevote, 7, 0, testHash(1))), "the new h+1")
	p.Advance(0, set)
	requireAdmission(t, Dropped, p.AddVote(testVote(t, vs, 3, TypePrecommit, 5, 0, testHash(1))), "h−2")
	if p.Messages(5) != nil {
		t.Fatal("the messages of h−2 are kept")
	}
}

// The messages of h+1 are checked by the set of h until h is committed and then by the set of
// h+1: those of a validator that left, or whose index is another key in the new set, are dropped
func TestPoolNextSet(t *testing.T) {
	vs, set := testSet(t, 5)
	p := NewPool(testNetwork, 10, 5, set)
	for i := range uint32(5) {
		requireAdmission(t, Admitted, p.AddVote(testVote(t, vs, i, TypePrevote, 6, 0, testHash(1))), "a prevote of h+1")
	}
	requireAdmission(t, Admitted, p.AddProposal(testProposal(t, vs, set, 6, 0, testHash(1))), "the proposal of h+1")
	// validator 1 leaves: 2, 3, 4 move to 1, 2, 3
	next := Validators{Keys: [][]byte{set.Keys[0], set.Keys[2], set.Keys[3], set.Keys[4]}}
	kept := p.Advance(0, next)
	var votes, proposals int
	for _, m := range kept {
		switch m := m.(type) {
		case *Vote:
			votes++
			if m.Validator != 0 {
				t.Fatalf("the prevote of validator %d is kept", m.Validator)
			}
		case *Proposal:
			proposals++
		}
	}
	// the proposer of height 6, round 0 is 6 mod 5 = 1 in the old set and 6 mod 4 = 2 in the new
	if votes != 1 || proposals != 0 {
		t.Fatalf("%d votes and %d proposals kept, want 1 and 0", votes, proposals)
	}
	if got := len(p.Messages(6)); got != 1 {
		t.Fatalf("%d messages of the new height, want 1", got)
	}
}
