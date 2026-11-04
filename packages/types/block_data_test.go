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
