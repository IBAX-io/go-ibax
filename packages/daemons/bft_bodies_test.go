/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/stretchr/testify/require"
)

// The node gives the bodies of the blocks it made or filled in, those of its queue for the rest,
// keeps the blocks of the heights it is at or above, and fills in a proposal once at a time
func TestBftBodies(t *testing.T) {
	s := newBftBodies()
	s.put(5, bft.Hash{1}, []byte("block"), []byte("compact"), [][]byte{[]byte("a"), []byte("b")},
		[][]byte{[]byte("A"), []byte("B")})
	queue := func(hashes [][]byte) [][]byte {
		bodies := make([][]byte, len(hashes))
		for i, h := range hashes {
			if string(h) == "q" {
				bodies[i] = []byte("Q")
			}
		}
		return bodies
	}
	require.Equal(t, [][]byte{[]byte("B"), []byte("Q"), nil, []byte("A")},
		s.transactions(bft.Hash{1}, [][]byte{[]byte("b"), []byte("q"), []byte("x"), []byte("a")}, queue))
	require.Equal(t, [][]byte{nil, []byte("Q")}, s.transactions(bft.Hash{2}, [][]byte{[]byte("a"), []byte("q")}, queue))
	compact, err := s.compact(bft.Hash{1}, nil)
	require.NoError(t, err)
	require.Equal(t, []byte("compact"), compact)

	k := bftProposalKey{height: 5, round: 1, validator: 2, hash: bft.Hash{1}}
	require.True(t, s.start(k))
	require.False(t, s.start(k))
	s.done(k)
	require.True(t, s.start(k))

	s.prune(5)
	require.NotNil(t, s.get(bft.Hash{1}))
	s.prune(6)
	require.Nil(t, s.get(bft.Hash{1}))
	require.True(t, s.start(k), "the proposals of the heights below are dropped")
}
