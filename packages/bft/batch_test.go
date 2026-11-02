/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"bytes"
	"errors"
	"testing"
)

func testBatchHashes(n int, b byte) [][]byte {
	hashes := make([][]byte, n)
	for i := range hashes {
		h := testHash(b + byte(i))
		hashes[i] = h[:]
	}
	return hashes
}

func TestBatch(t *testing.T) {
	vs, set := testSet(t, 4)
	first := &Batch{Height: 5, Round: 2, Validator: set.Proposer(5, 2), Seq: 0, Header: []byte("header"), Hashes: testBatchHashes(2, 0x10)}
	later := &Batch{Height: 5, Round: 2, Validator: set.Proposer(5, 2), Seq: 1, Hashes: testBatchHashes(3, 0x20)}
	empty := &Batch{Height: 5, Round: 2, Validator: set.Proposer(5, 2), Seq: 0, Header: []byte("header")}
	for _, b := range []*Batch{first, later, empty} {
		if err := b.Sign(vs[b.Validator].private, testNetwork); err != nil {
			t.Fatal(err)
		}
		wire := b.Marshal()
		got, err := UnmarshalBatch(wire, 100)
		if err != nil {
			t.Fatalf("batch %d: %v", b.Seq, err)
		}
		if !bytes.Equal(got.Marshal(), wire) {
			t.Fatalf("batch %d does not decode to itself", b.Seq)
		}
		key, _ := set.Key(got.Validator)
		if err := got.Verify(key, testNetwork); err != nil {
			t.Fatal(err)
		}
		if err := got.Verify(key, testNetwork+1); !errors.Is(err, ErrSignature) {
			t.Fatalf("a batch of another network verifies: %v", err)
		}
		// every byte is signed
		for i := range wire {
			bad := append([]byte(nil), wire...)
			bad[i] ^= 0x01
			w, err := UnmarshalBatch(bad, 100)
			if err != nil {
				continue
			}
			key, ok := set.Key(w.Validator)
			if ok && w.Verify(key, testNetwork) == nil {
				t.Errorf("batch %d, byte %d changed: the batch verifies", b.Seq, i)
			}
		}
	}
	if _, err := UnmarshalBatch(first.Marshal(), 1); !errors.Is(err, ErrFormat) {
		t.Fatalf("a batch above the count: %v", err)
	}
	for name, b := range map[string]*Batch{
		"a first batch without a header": {Height: 5, Seq: 0, Hashes: testBatchHashes(1, 1)},
		"a later batch with a header":    {Height: 5, Seq: 1, Header: []byte("h"), Hashes: testBatchHashes(1, 1)},
		"a later batch without a hash":   {Height: 5, Seq: 1},
		"a header above the bound":       {Height: 5, Seq: 0, Header: make([]byte, MaxBatchHeader+1)},
	} {
		b.Signature = []byte{1}
		if _, err := UnmarshalBatch(b.Marshal(), 100); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := UnmarshalBatch(append(first.Marshal(), 0), 100); !errors.Is(err, ErrFormat) {
		t.Fatalf("bytes left over: %v", err)
	}
}

func TestPoolAdmissibleBatch(t *testing.T) {
	vs, set := testSet(t, 4)
	others, _ := testSet(t, 1)
	p := NewPool(testNetwork, 2, 5, set)
	p.SetRound(1)
	signed := func(b *Batch, private []byte) *Batch {
		if err := b.Sign(private, testNetwork); err != nil {
			t.Fatal(err)
		}
		return b
	}
	batch := func(height uint64, round uint32) *Batch {
		return &Batch{Height: height, Round: round, Validator: set.Proposer(height, round), Seq: 1, Hashes: testBatchHashes(1, 1)}
	}
	ok := signed(batch(5, 2), vs[set.Proposer(5, 2)].private)
	if key, admitted := p.AdmissibleBatch(ok); !admitted || !bytes.Equal(key, vs[ok.Validator].public) {
		t.Fatal("a batch of the proposer of the round is refused")
	}
	notProposer := batch(5, 2)
	notProposer.Validator = (notProposer.Validator + 1) % 4
	for name, b := range map[string]*Batch{
		"of the next height":     signed(batch(6, 0), vs[set.Proposer(6, 0)].private),
		"of a round below":       signed(batch(5, 0), vs[set.Proposer(5, 0)].private),
		"of a round past":        signed(batch(5, 4), vs[set.Proposer(5, 4)].private),
		"of another validator":   signed(notProposer, vs[notProposer.Validator].private),
		"signed by another node": signed(batch(5, 2), others[0].private),
	} {
		if _, admitted := p.AdmissibleBatch(b); admitted {
			t.Errorf("a batch %s is admitted", name)
		}
	}
}
