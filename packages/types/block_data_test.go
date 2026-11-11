/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package types

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/stretchr/testify/require"
)

// The hash of a block of version 4 takes its validators hash and its state hash; the hash of a
// block of an earlier version is as it was
func TestForShaVersions(t *testing.T) {
	prev := &BlockHeader{BlockHash: []byte{0xaa}, RollbacksHash: []byte{0xbb}}
	header := func(version int32) *BlockHeader {
		return &BlockHeader{BlockId: 2, Timestamp: 3, KeyId: 4, NodePosition: 1, NetworkId: 9, Version: version,
			NextValidatorsHash: []byte{0xcc}, StateHash: []byte{0xdd}}
	}
	require.Equal(t, "2,aa,mr,3,0,4,1,9,bb", header(consts.BvIncludeRollbackHash).ForSha(prev, []byte("mr")))
	require.Equal(t, "2,aa,mr,3,0,4,1,9,bb,cc,dd", header(consts.BvValidatorsHash).ForSha(prev, []byte("mr")))
	require.Equal(t, "0,2,aa,3,0,4,1,mr,9,bb,cc,dd", header(consts.BvValidatorsHash).ForSign(prev, []byte("mr")))

	h := header(consts.BvValidatorsHash)
	hash := h.ForSha(prev, nil)
	h.NextValidatorsHash = []byte{0xce}
	require.NotEqual(t, hash, h.ForSha(prev, nil))
	h.NextValidatorsHash, h.StateHash = []byte{0xcc}, nil
	require.NotEqual(t, hash, h.ForSha(prev, nil))
}

// HeaderOf reads from the first bytes of a marshalled block the header that the whole block has,
// tells the length of data a header longer than the bytes needs, and refuses data that does not
// lead with a header
func TestHeaderOf(t *testing.T) {
	header := &BlockHeader{BlockId: 7, Timestamp: 3, KeyId: 4, NodePosition: 1, NetworkId: 9, Version: consts.BvValidatorsHash,
		Sign: make([]byte, 3309), BlockHash: []byte{0xaa}, RollbacksHash: []byte{0xbb}, CandidateNodes: []byte("[]"),
		NextValidatorsHash: []byte{0xcc}, StateHash: []byte{0xdd}}
	block := &BlockData{Header: header, PrevHeader: &BlockHeader{BlockId: 6, BlockHash: []byte{0xee}}, MerkleRoot: []byte("mr"),
		TxFullData: [][]byte{make([]byte, 5000), make([]byte, 5000)}}
	data, err := block.Marshal()
	require.NoError(t, err)
	end := header.Size() + 3 // the key and a size of two bytes

	full := &BlockData{}
	require.NoError(t, full.UnmarshallBlock(data))
	for _, n := range []int{len(data), end} {
		got, need, err := HeaderOf(data[:n])
		require.NoError(t, err)
		require.Zero(t, need)
		require.Equal(t, full.Header, got)
	}
	for _, n := range []int{3, end - 1} {
		got, need, err := HeaderOf(data[:n])
		require.NoError(t, err)
		require.Equal(t, end, need, n)
		require.Nil(t, got)
	}
	// a size cut short needs more than the bytes there are
	for _, n := range []int{1, 2} {
		got, need, err := HeaderOf(data[:n])
		require.NoError(t, err)
		require.Greater(t, need, n)
		require.Nil(t, got)
	}

	noHeader, err := (&BlockData{MerkleRoot: []byte("mr"), TxFullData: [][]byte{{1}}}).Marshal()
	require.NoError(t, err)
	for _, d := range [][]byte{nil, noHeader} {
		_, _, err = HeaderOf(d)
		require.Error(t, err)
	}
}
