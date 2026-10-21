/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/bft"
)

// The driver takes the evidence of a key once, and keeps it to submit: past bftEvidences keys it
// forgets the oldest, and past bftEvidences waiting it keeps no more
func TestBftEvidenceTake(t *testing.T) {
	var s bftEvidence
	key := func(h uint64) bft.EvidenceKey { return bft.EvidenceKey{PublicKey: "k", Height: h, Kind: 1} }
	if !s.take(key(1), []byte("a")) {
		t.Fatal("a new evidence is not new")
	}
	if s.take(key(1), []byte("b")) {
		t.Fatal("the evidence of a key taken twice")
	}
	if !s.take(bft.EvidenceKey{PublicKey: "k", Height: 1, Round: 1, Kind: 1}, []byte("c")) {
		t.Fatal("the evidence of another round is not new")
	}
	if len(s.pending) != 2 {
		t.Fatalf("%d evidences to submit, want 2", len(s.pending))
	}
	for h := uint64(2); len(s.order) < bftEvidences; h++ {
		s.take(key(h), []byte("d"))
	}
	if len(s.pending) != bftEvidences || !s.seen[key(1)] {
		t.Fatalf("%d to submit, the first key remembered %v", len(s.pending), s.seen[key(1)])
	}
	if !s.take(key(1<<40), []byte("e")) {
		t.Fatal("an evidence past the bound is not new")
	}
	if len(s.pending) != bftEvidences || len(s.seen) != bftEvidences || s.seen[key(1)] {
		t.Fatalf("past the bound: %d to submit, %d remembered, the oldest remembered %v", len(s.pending), len(s.seen), s.seen[key(1)])
	}
	if !s.take(key(1), []byte("a")) {
		t.Fatal("a forgotten evidence is not new again")
	}
}
