/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

const testNetwork = 7

type testValidator struct {
	private, public []byte
}

func testSet(t *testing.T, n int) ([]testValidator, Validators) {
	t.Helper()
	crypto.InitAsymAlgo("ECC_P256")
	vs := make([]testValidator, n)
	var set Validators
	for i := range vs {
		private, public, err := crypto.GenNodeKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		vs[i] = testValidator{private, public}
		set.Keys = append(set.Keys, public)
	}
	return vs, set
}

func testHash(b byte) (h Hash) {
	for i := range h {
		h[i] = b
	}
	return h
}

func TestQuorum(t *testing.T) {
	want := map[int][2]int{1: {1, 0}, 2: {2, 0}, 3: {3, 0}, 4: {3, 1}, 5: {4, 1}, 6: {5, 1}, 7: {5, 2}, 8: {6, 2}, 9: {7, 2}, 10: {7, 3}}
	for n, qf := range want {
		if Quorum(n) != qf[0] || Faults(n) != qf[1] {
			t.Errorf("N = %d: Q %d, f %d, want %d, %d", n, Quorum(n), Faults(n), qf[0], qf[1])
		}
	}
	for n := 1; n <= 100; n++ {
		q, f := Quorum(n), Faults(n)
		// two quorums share more than f validators, so an honest one, and a quorum is reachable
		// with f validators off
		if 2*q-n <= f || q > n-f {
			t.Errorf("N = %d: Q %d, f %d", n, q, f)
		}
		// stopping a validator never raises the quorum (design section 4.1)
		if n > 1 && Quorum(n-1) > q {
			t.Errorf("Q(%d) = %d > Q(%d) = %d", n-1, Quorum(n-1), n, q)
		}
	}
}

func TestProposer(t *testing.T) {
	set := Validators{Keys: [][]byte{{1}, {2}, {3}, {4}}}
	cases := []struct {
		h    uint64
		r    uint32
		want uint32
	}{{1, 0, 1}, {2, 0, 2}, {3, 0, 3}, {4, 0, 0}, {1, 1, 2}, {1, 3, 0}, {^uint64(0), ^uint32(0), uint32((^uint64(0)%4 + uint64(^uint32(0))%4) % 4)}}
	for _, c := range cases {
		if got := set.Proposer(c.h, c.r); got != c.want {
			t.Errorf("height %d, round %d: proposer %d, want %d", c.h, c.r, got, c.want)
		}
	}
}

func TestSignBytes(t *testing.T) {
	vote := SignBytes(testNetwork, 0x0102030405060708, 0x0a0b0c0d, TypePrecommit, testHash(0xee), NoRound)
	want := hex.EncodeToString([]byte(MsgPrefix)) + "0000000000000007" + "0102030405060708" + "0a0b0c0d" + "03" + hex.EncodeToString(bytes.Repeat([]byte{0xee}, 32))
	if hex.EncodeToString(vote) != want || len(vote) != 68 {
		t.Fatalf("the bytes of a precommit: %x", vote)
	}
	prop := SignBytes(testNetwork, 0x0102030405060708, 0x0a0b0c0d, TypeProposal, testHash(0xee), NoRound)
	// the type is the byte after the prefix, the network, the height and the round
	const typeAt = len(MsgPrefix) + 8 + 8 + 4
	if len(prop) != 72 || !bytes.Equal(prop[:typeAt], vote[:typeAt]) || prop[typeAt] != byte(TypeProposal) ||
		!bytes.Equal(prop[typeAt+1:68], vote[typeAt+1:]) || !bytes.Equal(prop[68:], []byte{0xff, 0xff, 0xff, 0xff}) {
		t.Fatalf("the bytes of a proposal: %x", prop)
	}
	if bytes.Equal(SignBytes(testNetwork, 1, 0, TypePrevote, Nil, NoRound), SignBytes(testNetwork, 1, 0, TypePrecommit, Nil, NoRound)) ||
		bytes.Equal(SignBytes(testNetwork, 1, 0, TypePrevote, Nil, NoRound), SignBytes(testNetwork+1, 1, 0, TypePrevote, Nil, NoRound)) {
		t.Fatal("the bytes of a vote do not tell its type and its network")
	}
}

func TestVote(t *testing.T) {
	vs, set := testSet(t, 4)
	v := &Vote{Type: TypePrevote, Height: 12, Round: 3, Validator: 2, Hash: testHash(0x5a)}
	if err := v.Sign(vs[2].private, testNetwork); err != nil {
		t.Fatal(err)
	}
	if len(v.Signature) != crypto.GetAsymProvider().SignatureSize() {
		t.Fatalf("a signature of %d bytes", len(v.Signature))
	}
	wire := v.Marshal()
	got, err := UnmarshalVote(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Marshal(), wire) {
		t.Fatal("the vote does not decode to itself")
	}
	key, _ := set.Key(got.Validator)
	if err := got.Verify(key, testNetwork); err != nil {
		t.Fatal(err)
	}
	if err := got.Verify(key, testNetwork+1); !errors.Is(err, ErrSignature) {
		t.Fatalf("a vote of another network: %v", err)
	}
	// every byte of the wire form matters: a vote with any byte changed does not decode, names no
	// validator, or does not verify by the key of the validator it names
	for i := range wire {
		bad := append([]byte(nil), wire...)
		bad[i] ^= 0x01
		w, err := UnmarshalVote(bad)
		if err != nil {
			continue
		}
		key, ok := set.Key(w.Validator)
		if ok && w.Verify(key, testNetwork) == nil {
			t.Errorf("byte %d changed: the vote verifies", i)
		}
	}
	if _, err := UnmarshalVote(append(wire, 0)); !errors.Is(err, ErrFormat) {
		t.Fatalf("a byte left over: %v", err)
	}
	if _, err := UnmarshalVote(wire[:len(wire)-1]); !errors.Is(err, ErrFormat) {
		t.Fatalf("a byte short: %v", err)
	}
	p := append([]byte(nil), wire...)
	p[0] = byte(TypeProposal)
	if _, err := UnmarshalVote(p); !errors.Is(err, ErrFormat) {
		t.Fatalf("a vote of type proposal: %v", err)
	}
}

func TestProposal(t *testing.T) {
	vs, set := testSet(t, 4)
	p := &Proposal{Height: 5, Round: 2, ValidRound: 1, Validator: set.Proposer(5, 2), Hash: testHash(0x33), Block: []byte("block")}
	if err := p.Sign(vs[p.Validator].private, testNetwork); err != nil {
		t.Fatal(err)
	}
	wire := p.Marshal()
	got, err := UnmarshalProposal(wire, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Marshal(), wire) {
		t.Fatal("the proposal does not decode to itself")
	}
	key, _ := set.Key(got.Validator)
	if err := got.Verify(key, testNetwork); err != nil {
		t.Fatal(err)
	}
	// the block is not signed: its hash is, and the receiver checks the hash of the block. Every
	// other byte matters.
	blockAt := 8 + 4 + 4 + 4 + HashSize + 4
	for i := range wire {
		if i >= blockAt && i < blockAt+len(p.Block) {
			continue
		}
		bad := append([]byte(nil), wire...)
		bad[i] ^= 0x01
		w, err := UnmarshalProposal(bad, 1<<20)
		if err != nil {
			continue
		}
		key, ok := set.Key(w.Validator)
		if ok && w.Verify(key, testNetwork) == nil {
			t.Errorf("byte %d changed: the proposal verifies", i)
		}
	}
	if _, err := UnmarshalProposal(wire, int64(len(p.Block)-1)); !errors.Is(err, ErrFormat) {
		t.Fatalf("a block above the size: %v", err)
	}
	for _, vr := range []int32{-2, 2, 3} {
		q := *p
		q.ValidRound = vr
		if _, err := UnmarshalProposal(q.Marshal(), 1<<20); !errors.Is(err, ErrFormat) {
			t.Errorf("validRound %d at round 2: %v", vr, err)
		}
	}
	q := *p
	q.Hash = Nil
	if _, err := UnmarshalProposal(q.Marshal(), 1<<20); !errors.Is(err, ErrFormat) {
		t.Fatalf("a proposal of nil: %v", err)
	}
}

type testChain struct {
	height, maxAge uint64
	set            Validators
	seen           map[EvidenceKey]bool
}

func (c *testChain) Height() uint64                        { return c.height }
func (c *testChain) MaxAge() uint64                        { return c.maxAge }
func (c *testChain) Validators(uint64) (Validators, error) { return c.set, nil }
func (c *testChain) HasEvidence(k EvidenceKey) (bool, error) {
	return c.seen[k], nil
}

func testEvidence(t *testing.T, v testValidator, index uint32, mt MsgType, height uint64, round uint32, a, b Hash, vrA, vrB int32) *Evidence {
	t.Helper()
	sigA, err := Sign(v.private, testNetwork, height, round, mt, a, vrA)
	if err != nil {
		t.Fatal(err)
	}
	sigB, err := Sign(v.private, testNetwork, height, round, mt, b, vrB)
	if err != nil {
		t.Fatal(err)
	}
	e, err := NewEvidence(testNetwork, height, round, index, mt, a, b, vrA, vrB, sigA, sigB)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestEvidence(t *testing.T) {
	vs, set := testSet(t, 4)
	chain := &testChain{height: 200000, maxAge: 100000, set: set, seen: map[EvidenceKey]bool{}}

	for _, mt := range []MsgType{TypeProposal, TypePrevote, TypePrecommit} {
		vrA, vrB := NoRound, int32(0)
		if mt != TypeProposal {
			vrB = NoRound
		}
		e := testEvidence(t, vs[1], 1, mt, 150000, 1, testHash(1), Nil, vrA, vrB)
		got, key, err := CheckEvidence(e.Marshal(), testNetwork, chain)
		if err != nil {
			t.Fatalf("an evidence of %s: %v", mt, err)
		}
		if !bytes.Equal(got.Marshal(), e.Marshal()) || key != (EvidenceKey{string(vs[1].public), 150000, 1, uint8(mt)}) {
			t.Fatalf("an evidence of %s decodes to %+v, key %+v", mt, got, key)
		}
	}
	if _, err := NewEvidence(testNetwork, 1, 0, 0, TypePrevote, testHash(1), testHash(1), NoRound, NoRound, nil, nil); err == nil {
		t.Fatal("an evidence of one hash")
	}

	good := testEvidence(t, vs[2], 2, TypePrevote, 150000, 0, testHash(7), testHash(8), NoRound, NoRound)
	check := func(name string, body []byte, want error) {
		t.Helper()
		if _, _, err := CheckEvidence(body, testNetwork, chain); !errors.Is(err, want) {
			t.Errorf("%s: %v, want %v", name, err, want)
		}
	}
	mutate := func(f func(e *Evidence)) []byte {
		e := *good
		f(&e)
		return e.Marshal()
	}

	// check 1: the format
	check("a byte left over", append(good.Marshal(), 0), ErrEvidenceFormat)
	check("a byte short", good.Marshal()[:len(good.Marshal())-1], ErrEvidenceFormat)
	bad := good.Marshal()
	bad[0] ^= 0x01
	check("another prefix", bad, ErrEvidenceFormat)
	check("another network", mutate(func(e *Evidence) { e.NetworkID++ }), ErrEvidenceFormat)
	check("kind 0", mutate(func(e *Evidence) { e.Kind = 0 }), ErrEvidenceFormat)
	check("kind 4", mutate(func(e *Evidence) { e.Kind = 4 }), ErrEvidenceFormat)
	check("a validRound of a vote", mutate(func(e *Evidence) { e.ValidRoundA = 0 }), ErrEvidenceFormat)
	check("a validRound below -1", mutate(func(e *Evidence) { e.Kind = KindDoubleProposal; e.ValidRoundB = -2 }), ErrEvidenceFormat)

	// check 2: the height
	check("height 0", mutate(func(e *Evidence) { e.Height = 0 }), ErrEvidenceAge)
	check("a height above the chain", mutate(func(e *Evidence) { e.Height = chain.height + 1 }), ErrEvidenceAge)
	check("a height too old", mutate(func(e *Evidence) { e.Height = chain.height - chain.maxAge - 1 }), ErrEvidenceAge)
	old := testEvidence(t, vs[2], 2, TypePrevote, chain.height-chain.maxAge, 0, testHash(7), testHash(8), NoRound, NoRound)
	check("the oldest height kept", old.Marshal(), nil)

	// check 3: the validator
	check("no validator 4 of 4", mutate(func(e *Evidence) { e.Validator = 4 }), ErrEvidenceValidator)

	// check 4: the signatures
	check("another validator", mutate(func(e *Evidence) { e.Validator = 1 }), ErrEvidenceSignature)
	check("signature a changed", mutate(func(e *Evidence) { e.SigA = append([]byte(nil), e.SigA...); e.SigA[10] ^= 1 }), ErrEvidenceSignature)
	check("signature b empty", mutate(func(e *Evidence) { e.SigB = nil }), ErrEvidenceSignature)
	check("another round", mutate(func(e *Evidence) { e.Round = 1 }), ErrEvidenceSignature)
	check("another kind", mutate(func(e *Evidence) { e.Kind = KindDoublePrecommit }), ErrEvidenceSignature)
	check("hash b changed", mutate(func(e *Evidence) { e.HashB = testHash(9) }), ErrEvidenceSignature)

	// check 5: two hashes. Two proposals of one hash and two validRounds are no evidence.
	same := testEvidence(t, vs[0], 0, TypeProposal, 150000, 2, testHash(3), testHash(4), NoRound, 1)
	sigB, _ := Sign(vs[0].private, testNetwork, 150000, 2, TypeProposal, testHash(3), 1)
	same.HashB, same.SigB = testHash(3), sigB
	check("two proposals of one hash", same.Marshal(), ErrEvidenceSameHash)
	check("one vote twice", mutate(func(e *Evidence) { e.HashB, e.SigB = e.HashA, e.SigA }), ErrEvidenceSameHash)

	// check 6: a duplicate
	_, key, err := CheckEvidence(good.Marshal(), testNetwork, chain)
	if err != nil {
		t.Fatal(err)
	}
	chain.seen[key] = true
	check("the evidence again", good.Marshal(), ErrEvidenceDuplicate)
	again := testEvidence(t, vs[2], 2, TypePrevote, 150000, 0, testHash(7), Nil, NoRound, NoRound)
	check("another pair of the same key", again.Marshal(), ErrEvidenceDuplicate)
	other := testEvidence(t, vs[2], 2, TypePrecommit, 150000, 0, testHash(7), testHash(8), NoRound, NoRound)
	check("another kind of the same height and round", other.Marshal(), nil)
}
