/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network/snapnet"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/gogo/protobuf/proto"
	log "github.com/sirupsen/logrus"
)

const joinNetwork = 7

// joinSet is a validators set and the private keys of its validators
type joinSet struct {
	privates [][]byte
	set      bft.Validators
}

func newJoinSet(t *testing.T, n int) joinSet {
	var s joinSet
	for range n {
		private, public, err := crypto.GenNodeKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		s.privates = append(s.privates, private)
		s.set.Keys = append(s.set.Keys, public)
	}
	return s
}

// certify is the certificate of a quorum of the set for the block hash at the height
func (s joinSet) certify(t *testing.T, height int64, blockHash []byte) []byte {
	hash, err := bft.HashOf(blockHash)
	if err != nil {
		t.Fatal(err)
	}
	cert := &bft.Certificate{Height: uint64(height), Hash: hash}
	for i := range s.set.Quorum() {
		sig, err := bft.Sign(s.privates[i], joinNetwork, uint64(height), 0, bft.TypePrecommit, hash, bft.NoRound)
		if err != nil {
			t.Fatal(err)
		}
		cert.Precommits = append(cert.Precommits, bft.Signed{Validator: uint32(i), Signature: sig})
	}
	return cert.Marshal()
}

// joinChain is a chain of blocks without transactions whose set changes at the heights of from
type joinChain struct {
	sets      []joinSet
	from      []int64 // the first height of each set, from 2 on
	raws      map[int64][]byte
	certs     map[int64][]byte
	headers   map[int64]*types.BlockHeader
	stateHash map[int64][]byte // the state hash a block carries
}

func (c *joinChain) setIndex(height int64) int {
	i := 0
	for i+1 < len(c.from) && c.from[i+1] <= height {
		i++
	}
	return i
}

// newJoinChain makes the blocks 1..last, the set changing every each blocks from the height each+1
func newJoinChain(t *testing.T, last, each int64, stateHash map[int64][]byte) *joinChain {
	c := &joinChain{raws: map[int64][]byte{}, certs: map[int64][]byte{}, headers: map[int64]*types.BlockHeader{},
		stateHash: stateHash}
	c.sets, c.from = []joinSet{newJoinSet(t, 4)}, []int64{2}
	for h := each + 1; h <= last+1; h += each {
		c.sets, c.from = append(c.sets, newJoinSet(t, 4)), append(c.from, h)
	}
	prev := &types.BlockHeader{BlockHash: []byte("genesis"), RollbacksHash: []byte{0}}
	for h := int64(1); h <= last; h++ {
		c.add(t, h, prev, nil)
		prev = c.headers[h]
	}
	return c
}

// add makes the block of a height after prev, of the header changed by change
func (c *joinChain) add(t *testing.T, height int64, prev *types.BlockHeader, change func(*types.BlockData)) {
	header := &types.BlockHeader{BlockId: height, Timestamp: 1000 + height, NetworkId: joinNetwork,
		Version: consts.BvValidatorsHash, RollbacksHash: []byte{byte(height)}, StateHash: c.stateHash[height],
		NextValidatorsHash: crypto.Hash(c.sets[c.setIndex(height+1)].set.Marshal())}
	b := &types.BlockData{Header: header, PrevHeader: prev}
	b.MerkleRoot = b.GenMerkleRoot()
	if change != nil {
		change(b)
	}
	header.BlockHash = header.GenHash(prev, b.MerkleRoot)
	raw, err := proto.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	c.raws[height], c.headers[height] = raw, header
	if height > 1 {
		c.certs[height] = c.sets[c.setIndex(height)].certify(t, height, header.BlockHash)
	}
}

// proofs are the proofs of the changes of the sets
func (c *joinChain) proofs() []snapnet.Proof {
	var ps []snapnet.Proof
	for i := 1; i < len(c.from); i++ {
		h := c.from[i]
		ps = append(ps, snapnet.Proof{FromHeight: h, Block: c.raws[h-1], Cert: c.certs[h-1], Set: c.sets[i].set.Marshal()})
	}
	return ps
}

// serveProofs makes the host serve the proofs, after edit changes them
func serveProofs(t *testing.T, ps []snapnet.Proof) {
	saved := fetchSetProofs
	t.Cleanup(func() { fetchSetProofs = saved })
	fetchSetProofs = func(host string, from int64, count int, timeout time.Duration) ([]snapnet.Proof, error) {
		var out []snapnet.Proof
		for _, p := range ps {
			if p.FromHeight > from && len(out) < min(count, snapnet.MaxProofs) {
				out = append(out, p)
			}
		}
		return out, nil
	}
}

// serveChain makes the host serve the blocks and the certificates of the chain
func serveChain(t *testing.T, c *joinChain) {
	savedBlocks, savedCerts := fetchBlocks, fetchCertificates
	t.Cleanup(func() { fetchBlocks, fetchCertificates = savedBlocks, savedCerts })
	fetchBlocks = func(ctx context.Context, host string, from int64, reverse bool) (<-chan []byte, int64, error) {
		raw, ok := c.raws[from]
		if !ok {
			return nil, 0, errors.New("no blocks")
		}
		ch := make(chan []byte, 1)
		ch <- raw
		close(ch)
		return ch, 1, nil
	}
	fetchCertificates = func(host string, from uint64, count int, timeout time.Duration) ([][]byte, error) {
		if cert, ok := c.certs[int64(from)]; ok {
			return [][]byte{cert}, nil
		}
		return nil, nil
	}
}

func testJoiner() *joiner {
	return &joiner{ctx: context.Background(), logger: log.NewEntry(log.StandardLogger()), hosts: []string{"host"},
		networkID: joinNetwork}
}

// TestJoinWalk: the walk takes the sets from block 1 up to the height asked by the proofs, more
// than a request of them, and refuses a proof of a change omitted or of another set
func TestJoinWalk(t *testing.T) {
	c := newJoinChain(t, 400, 20, nil)
	ps := c.proofs()
	if len(ps) <= snapnet.MaxProofs {
		t.Fatalf("%d proofs, within a request", len(ps))
	}
	serveProofs(t, ps)
	j := testJoiner()
	// the set of 401, that block 400 leaves, is the last
	sets, proofs, err := j.walk("host", c.sets[0].set, 401)
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != len(c.sets) || len(proofs) != len(ps) {
		t.Fatalf("%d sets and %d proofs, not %d and %d", len(sets), len(proofs), len(c.sets), len(ps))
	}
	for _, h := range []int64{2, 20, 21, 40, 41, 399, 400, 401} {
		if got, want := setAt(sets, h), c.sets[c.setIndex(h)].set; !equalSets(got, want) {
			t.Fatalf("the set of height %d", h)
		}
	}
	if proofs[0].Height != 20 {
		t.Fatalf("the first proof is of block %d", proofs[0].Height)
	}

	// up to a height, the changes after it are not taken
	sets, _, err = j.walk("host", c.sets[0].set, 45)
	if err != nil || len(sets) != 3 || sets[2].from != 41 {
		t.Fatalf("the walk up to 45: %d sets, %v", len(sets), err)
	}

	// a host that omits a change: the proof after it is of a set the walk does not have
	serveProofs(t, append(append([]snapnet.Proof{}, ps[:3]...), ps[4:]...))
	if _, _, err := j.walk("host", c.sets[0].set, 400); !errors.Is(err, bft.ErrCertificate) {
		t.Fatalf("a change omitted: %v", err)
	}

	// a proof of another set than the block has the hash of
	other := append([]snapnet.Proof{}, ps...)
	other[2].Set = newJoinSet(t, 4).set.Marshal()
	serveProofs(t, other)
	if _, _, err := j.walk("host", c.sets[0].set, 400); err == nil || !strings.Contains(err.Error(), "another set") {
		t.Fatalf("another set: %v", err)
	}

	// a proof of another block than the one before the change
	moved := append([]snapnet.Proof{}, ps...)
	moved[1].Block, moved[1].Cert = c.raws[39], c.certs[39]
	serveProofs(t, moved)
	if _, _, err := j.walk("host", c.sets[0].set, 400); err == nil {
		t.Fatal("a proof of another block")
	}

	// the walk from another set than that of block 1
	serveProofs(t, ps)
	if _, _, err := j.walk("host", newJoinSet(t, 4).set, 400); !errors.Is(err, bft.ErrCertificate) {
		t.Fatalf("another set of block 1: %v", err)
	}
}

func equalSets(a, b bft.Validators) bool {
	return string(a.Marshal()) == string(b.Marshal())
}

// TestJoinStateHash: the block after the delay gives the state hash by its certificate of the set
// of its height alone
func TestJoinStateHash(t *testing.T) {
	want := crypto.Hash([]byte("state of 100"))
	c := newJoinChain(t, 120, 50, map[int64][]byte{110: want})
	serveChain(t, c)
	j := testJoiner()
	hash, err := j.certifiedStateHash(110, c.sets[c.setIndex(110)].set)
	if err != nil || string(hash) != string(want) {
		t.Fatalf("the state hash of block 110: %x, %v", hash, err)
	}
	if _, err := j.certifiedStateHash(110, c.sets[0].set); !errors.Is(err, bft.ErrCertificate) {
		t.Fatalf("of another set: %v", err)
	}
	if _, err := j.certifiedStateHash(121, c.sets[2].set); err == nil {
		t.Fatal("a block the host has not")
	}
	// a block without a state hash
	hash, err = j.certifiedStateHash(109, c.sets[c.setIndex(109)].set)
	if err != nil || hash != nil {
		t.Fatalf("block 109: %x, %v", hash, err)
	}
}

// TestVerifyCertified: a block is of its certificate only whole: its transactions, its header and
// the block before it
func TestVerifyCertified(t *testing.T) {
	c := newJoinChain(t, 10, 100, nil)
	set := c.sets[0].set
	cert := func(h int64) *bft.Certificate {
		cert, err := bft.UnmarshalCertificate(c.certs[h])
		if err != nil {
			t.Fatal(err)
		}
		return cert
	}
	b, err := block.VerifyCertified(c.raws[5], cert(5), set, joinNetwork)
	if err != nil || b.Header.BlockId != 5 || string(b.BinData) != string(c.raws[5]) {
		t.Fatalf("block 5: %v", err)
	}
	for _, f := range []struct {
		what   string
		change func(*types.BlockData)
	}{
		{"a transaction", func(b *types.BlockData) { b.TxFullData = [][]byte{[]byte("tx")} }},
		{"a version without the validators hash", func(b *types.BlockData) { b.Header.Version = consts.BvValidatorsHash - 1 }},
		{"the block before", func(b *types.BlockData) { b.PrevHeader = &types.BlockHeader{BlockId: 5, BlockHash: []byte("x")} }},
		{"another height", func(b *types.BlockData) { b.Header.BlockId = 7 }},
	} {
		// the block changed after its hash: the hash and the certificate are those of block 6
		good := c.headers[6].BlockHash
		c.add(t, 6, c.headers[5], nil)
		var d types.BlockData
		if err := proto.Unmarshal(c.raws[6], &d); err != nil {
			t.Fatal(err)
		}
		f.change(&d)
		d.Header.BlockHash = good
		raw, err := proto.Marshal(&d)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := block.VerifyCertified(raw, cert(6), set, joinNetwork); err == nil {
			t.Fatalf("%s: verified", f.what)
		}
	}
	if _, err := block.VerifyCertified(c.raws[5], cert(6), set, joinNetwork); err == nil {
		t.Fatal("the certificate of another height")
	}
	if _, err := block.VerifyCertified(c.raws[5], cert(5), set, joinNetwork+1); err == nil {
		t.Fatal("another network")
	}
	if _, err := block.VerifyCertified([]byte("not a block"), cert(5), set, joinNetwork); err == nil {
		t.Fatal("not a block")
	}
}

func TestSetAt(t *testing.T) {
	sets := []walkedSet{{from: 2}, {from: 101}, {from: 201}}
	for i := range sets {
		sets[i].set.Keys = [][]byte{[]byte(fmt.Sprint(i))}
	}
	for h, want := range map[int64]string{2: "0", 100: "0", 101: "1", 200: "1", 201: "2", 1 << 40: "2"} {
		if got := string(setAt(sets, h).Keys[0]); got != want {
			t.Fatalf("height %d: set %s, not %s", h, got, want)
		}
	}
}

func TestJoinWaitTip(t *testing.T) {
	defer func(f func(context.Context, []string) (string, int64, error), poll, stall time.Duration) {
		hostWithMaxBlock, joinPoll, joinStall = f, poll, stall
	}(hostWithMaxBlock, joinPoll, joinStall)
	joinPoll, joinStall = time.Millisecond, 50*time.Millisecond
	j := &joiner{ctx: context.Background(), logger: log.NewEntry(log.StandardLogger()), hosts: []string{"a"}}

	// the hosts move on to the block of the state hash
	tip := int64(100195)
	hostWithMaxBlock = func(context.Context, []string) (string, int64, error) { tip += 3; return "a", tip, nil }
	got, err := j.waitTip(100200, 100195)
	if err != nil || got < 100200 {
		t.Fatalf("waitTip = %d, %v; want at least 100200", got, err)
	}

	// the hosts stop short of it: the wait gives up with the tip it reached
	hostWithMaxBlock = func(context.Context, []string) (string, int64, error) { return "a", 100197, nil }
	start := time.Now()
	got, err = j.waitTip(100200, 100195)
	if err != nil || got != 100197 {
		t.Fatalf("waitTip = %d, %v; want 100197", got, err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("waitTip waited %v after the hosts stopped", d)
	}

	// a cancelled join stops waiting
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	j.ctx = ctx
	if _, err = j.waitTip(100200, 100195); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitTip of a cancelled join = %v", err)
	}
}

func TestJoinSnapshotHeights(t *testing.T) {
	defer func(f func(string, time.Duration) ([]int64, error)) { fetchSnapshotHeights = f }(fetchSnapshotHeights)
	fetchSnapshotHeights = func(h string, _ time.Duration) ([]int64, error) {
		if h == "b" {
			return nil, errors.New("down")
		}
		return []int64{90000, 95000, 100000}, nil
	}
	j := &joiner{logger: log.NewEntry(log.StandardLogger()), hosts: []string{"a", "b"}, heights: map[int64][]string{}}
	// the latest snapshot is listed even before the block of its state hash, for the join to wait
	if got := j.snapshotHeights(10000); !slices.Equal(got, []int64{100000, 90000}) {
		t.Fatalf("snapshotHeights = %v", got)
	}
}
