/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"sync"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
)

// bftBlock is a block of a proposal the node made or filled in: its compact form, the form the
// node sends, and the bodies of its transactions by their hashes, which the node gives the
// validators that do not have them
type bftBlock struct {
	height  uint64
	block   []byte
	compact []byte
	bodies  map[string][]byte
}

// bftBodies is the blocks of the proposals of the heights the node is at or above, by their hashes
type bftBodies struct {
	mu       sync.Mutex
	blocks   map[bft.Hash]*bftBlock
	inflight map[bftProposalKey]bool // the proposals whose blocks the node is filling in
}

// bftProposalKey is a proposal the pool tells apart from the others
type bftProposalKey struct {
	height    uint64
	round     uint32
	validator uint32
	hash      bft.Hash
}

func newBftBodies() *bftBodies {
	return &bftBodies{blocks: map[bft.Hash]*bftBlock{}, inflight: map[bftProposalKey]bool{}}
}

// put keeps a block whose transactions have the hashes and the bodies, in their order
func (s *bftBodies) put(height uint64, hash bft.Hash, bin, compact []byte, hashes, bodies [][]byte) {
	b := &bftBlock{height: height, block: bin, compact: compact, bodies: make(map[string][]byte, len(hashes))}
	for i, h := range hashes {
		b.bodies[string(h)] = bodies[i]
	}
	s.mu.Lock()
	s.blocks[hash] = b
	s.mu.Unlock()
}

func (s *bftBodies) get(hash bft.Hash) *bftBlock {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blocks[hash]
}

// prune drops the blocks below the height
func (s *bftBodies) prune(height uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, b := range s.blocks {
		if b.height < height {
			delete(s.blocks, h)
		}
	}
	for k := range s.inflight {
		if k.height < height {
			delete(s.inflight, k)
		}
	}
}

// start reports whether the node starts filling in the block of the proposal: false if it is
// filling it in already
func (s *bftBodies) start(k bftProposalKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight[k] {
		return false
	}
	s.inflight[k] = true
	return true
}

func (s *bftBodies) done(k bftProposalKey) {
	s.mu.Lock()
	delete(s.inflight, k)
	s.mu.Unlock()
}

// compact is the compact form of the block of the hash
func (s *bftBodies) compact(hash bft.Hash, bin []byte) ([]byte, error) {
	if b := s.get(hash); b != nil {
		return b.compact, nil
	}
	return block.CompactBlock(bin)
}

// transactions is the bodies of the hashes in the block of the hash, those it does not have
// taken from stored
func (s *bftBodies) transactions(hash bft.Hash, hashes [][]byte, stored func([][]byte) [][]byte) [][]byte {
	b := s.get(hash)
	if b == nil {
		return stored(hashes)
	}
	bodies := make([][]byte, len(hashes))
	var missing []int
	for i, h := range hashes {
		if bodies[i] = b.bodies[string(h)]; bodies[i] == nil {
			missing = append(missing, i)
		}
	}
	if len(missing) > 0 {
		rest := make([][]byte, len(missing))
		for j, i := range missing {
			rest[j] = hashes[i]
		}
		for j, body := range stored(rest) {
			bodies[missing[j]] = body
		}
	}
	return bodies
}
