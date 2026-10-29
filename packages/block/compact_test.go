/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"errors"
	mrand "math/rand/v2"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/gogo/protobuf/proto"
)

// compactTestBlock is a block of n transactions, the hashes the queue keys them by and their
// bodies as the block holds them
func compactTestBlock(t *testing.T, n int) ([]byte, [][]byte, map[string][]byte) {
	t.Helper()
	crypto.InitAsymAlgo("MLDSA65")
	crypto.InitHashAlgo("SHA3_256")
	key, _, err := crypto.GenNodeKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	r := mrand.New(mrand.NewPCG(9, 10))
	cur, prev := testHeaders(r)
	var txs []testTx
	for range n {
		txs = append(txs, newTestTx(r))
	}
	bin := marshalTestBlock(t, key, cur, prev, txs, false)
	raw := &types.BlockData{}
	if err := proto.Unmarshal(bin, raw); err != nil {
		t.Fatal(err)
	}
	hashes := make([][]byte, n)
	bodies := make(map[string][]byte, n)
	for i, body := range raw.TxFullData {
		hashes[i] = crypto.Hash(txs[i].fullData)
		bodies[string(hashes[i])] = body
	}
	return bin, hashes, bodies
}

// storedOf is the queue of a node that has the bodies of keep
func storedOf(bodies map[string][]byte, keep func(i int) bool) func([][]byte) [][]byte {
	return func(hashes [][]byte) [][]byte {
		got := make([][]byte, len(hashes))
		for i, h := range hashes {
			if keep(i) {
				got[i] = bodies[string(h)]
			}
		}
		return got
	}
}

// TestCompactRoundTrip: the block a validator expands from the compact form is the block of the
// proposer byte for byte, whichever bodies it has and however many it fetches
func TestCompactRoundTrip(t *testing.T) {
	bin, hashes, bodies := compactTestBlock(t, 30)
	compact, err := compactOf(bin, hashes)
	if err != nil {
		t.Fatal(err)
	}
	if len(compact) >= len(bin)/10 {
		t.Fatalf("the compact form is %d bytes of a block of %d", len(compact), len(bin))
	}
	got, err := CompactHashes(compact)
	if err != nil || len(got) != len(hashes) || !bytes.Equal(got[29], hashes[29]) {
		t.Fatalf("hashes %d, %v", len(got), err)
	}
	for _, c := range []struct {
		name    string
		keep    func(i int) bool
		fetched int // hashes asked of the proposer
	}{
		{"all stored", func(int) bool { return true }, 0},
		{"none stored", func(int) bool { return false }, 30},
		{"some stored", func(i int) bool { return i%3 != 0 }, 10},
	} {
		t.Run(c.name, func(t *testing.T) {
			asked := 0
			fetch := func(hs [][]byte) ([][]byte, error) {
				asked += len(hs)
				return storedOf(bodies, func(int) bool { return true })(hs), nil
			}
			out, _, err := Expand(compact, 1<<30, storedOf(bodies, c.keep), fetch)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out, bin) {
				t.Fatal("the expanded block differs from the block")
			}
			if asked != c.fetched {
				t.Fatalf("fetched %d, want %d", asked, c.fetched)
			}
		})
	}

	// a body the validator stores apart from the proposer's: it fetches them all
	other := storedOf(bodies, func(int) bool { return true })
	stored := func(hs [][]byte) [][]byte {
		got := other(hs)
		got[4] = types.DoZlibCompress([]byte("another body"))
		return got
	}
	asked := 0
	fetch := func(hs [][]byte) ([][]byte, error) {
		asked += len(hs)
		return other(hs), nil
	}
	out, _, err := Expand(compact, 1<<30, stored, fetch)
	if err != nil || !bytes.Equal(out, bin) || asked != 30 {
		t.Fatalf("a body stored apart: fetched %d, %v", asked, err)
	}

	// the proposer's bodies that do not make the merkle root, or that it does not have
	if _, _, err := Expand(compact, 1<<30, stored, func(hs [][]byte) ([][]byte, error) { return stored(hs), nil }); !errors.Is(err, ErrCompact) {
		t.Fatalf("bodies of another merkle root: %v", err)
	}
	if _, _, err := Expand(compact, 1<<30, storedOf(bodies, func(i int) bool { return i != 7 }),
		func(hs [][]byte) ([][]byte, error) { return [][]byte{nil}, nil }); err == nil {
		t.Fatal("a body the proposer does not have")
	}
	// a block over the limit
	if _, _, err := Expand(compact, int64(len(bin)-1), storedOf(bodies, func(int) bool { return true }), nil); err == nil {
		t.Fatal("a block over max_block_size")
	}
}

// TestCompactMalformed: a compact form that does not decode is refused, not read past its end
func TestCompactMalformed(t *testing.T) {
	bin, hashes, _ := compactTestBlock(t, 3)
	compact, err := compactOf(bin, hashes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compactOf(bin, hashes[:2]); !errors.Is(err, ErrCompact) {
		t.Fatalf("fewer hashes than transactions: %v", err)
	}
	for i := range len(compact) {
		if _, _, err := parseCompact(compact[:i]); err == nil {
			t.Fatalf("a compact form cut at %d decodes", i)
		}
	}
	if _, _, err := parseCompact(append(bytes.Clone(compact), 0)); err == nil {
		t.Fatal("a compact form with a trailing byte decodes")
	}
	// the block itself with its transactions in the place of the shell
	whole := append([]byte{byte(len(bin) >> 24), byte(len(bin) >> 16), byte(len(bin) >> 8), byte(len(bin))}, bin...)
	whole = append(whole, 0, 0, 0, 0)
	if _, _, err := parseCompact(whole); !errors.Is(err, ErrCompact) {
		t.Fatalf("a shell with transactions: %v", err)
	}
}
