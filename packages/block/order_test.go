/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
)

// orderedBlock is an ordered block of the height without transactions, as a commit takes it
func orderedBlock(t *testing.T, height int64) *Block {
	t.Helper()
	raw := &types.BlockData{
		Header:     &types.BlockHeader{BlockId: height, Version: consts.BvOrdered, Timestamp: 1000 + height, BlockHash: crypto.Hash([]byte{byte(height)}), CandidateNodes: []byte("[]")},
		PrevHeader: &types.BlockHeader{BlockId: height - 1},
	}
	bin, err := proto.Marshal(raw)
	require.NoError(t, err)
	raw.BinData = bin
	return &Block{BlockData: raw}
}

// The commit of an ordered block writes the block and its certificate, the executed height stays
// where the executor left it; the execution hash of a block is known once it is executed, and a
// proposal of a height the node committed is stale. It needs a database server, IBAX_TEST_DSN,
// where it makes a database of its own.
func TestOrderCommitAndHeights(t *testing.T) {
	stateTestDB(t)
	require.NoError(t, sqldb.DBConn.AutoMigrate(&sqldb.BlockChain{}, &sqldb.InfoBlock{}, &sqldb.BftCertificate{},
		&sqldb.BftExecHash{}, &sqldb.BftValidatorSet{}, &sqldb.LogTransaction{}))
	sqldb.ResetTxWindow()
	t.Cleanup(sqldb.ResetTxWindow)

	heights := func() (int64, int64) {
		committed, err := sqldb.CommittedHeight()
		require.NoError(t, err)
		executed, err := sqldb.ExecutedHeight()
		require.NoError(t, err)
		return committed, executed
	}
	committed, executed := heights()
	require.Zero(t, committed)
	require.Zero(t, executed)

	// the execution hash: none below block 1, ErrNotExecuted before the execution
	hash, err := ExecHashAt(0)
	require.NoError(t, err)
	require.Nil(t, hash)
	_, err = ExecHashAt(3)
	require.ErrorIs(t, err, ErrNotExecuted)
	require.NoError(t, (&sqldb.BftExecHash{Height: 3, Hash: []byte{3}}).SaveIn(nil))
	hash, err = ExecHashAt(3)
	require.NoError(t, err)
	require.Equal(t, []byte{3}, hash)

	for _, h := range []int64{5, 6} {
		b := orderedBlock(t, h)
		require.NoError(t, b.commitOrdered(&sqldb.BftCertificate{Height: h, Hash: b.Header.BlockHash}), "block %d", h)
	}
	committed, executed = heights()
	require.Equal(t, int64(6), committed)
	require.Zero(t, executed, "a commit executes nothing")
	cert := &sqldb.BftCertificate{}
	found, err := cert.Get(nil, 6)
	require.NoError(t, err)
	require.True(t, found, "the certificate is committed with its block")

	// a block committed twice fails as a whole: no second certificate either
	b := orderedBlock(t, 6)
	require.Error(t, b.commitOrdered(&sqldb.BftCertificate{Height: 6, Hash: b.Header.BlockHash}))
	var certs int64
	require.NoError(t, sqldb.DBConn.Model(&sqldb.BftCertificate{}).Count(&certs).Error)
	require.Equal(t, int64(2), certs)

	for _, h := range []int64{5, 6} {
		require.ErrorIs(t, CheckProposal(orderedBlock(t, h).BinData, h, bft.Hash{}), ErrStale, "height %d", h)
	}
}

// The signer of an ordered block is the validator at the position its header names in the set of
// its height
func TestValidatorAt(t *testing.T) {
	stateTestDB(t)
	require.NoError(t, sqldb.DBConn.AutoMigrate(&sqldb.BftValidatorSet{}))
	keys := make([][]byte, 3)
	for i := range keys {
		_, pub, err := crypto.GenNodeKeyPair()
		require.NoError(t, err)
		keys[i] = pub
	}
	set := bft.Validators{Keys: keys}
	require.NoError(t, (&sqldb.BftValidatorSet{FromHeight: 2, Data: set.Marshal()}).Create(nil))
	for i, k := range keys {
		got, err := validatorAt(nil, 5, int64(i))
		require.NoError(t, err)
		require.Equal(t, k, got)
	}
	_, err := validatorAt(nil, 5, 3)
	require.ErrorContains(t, err, "no validator at position 3")
	_, err = validatorAt(nil, 5, -1)
	require.ErrorContains(t, err, "no validator at position -1")
	_, err = validatorAt(nil, 1, 0)
	require.ErrorContains(t, err, "no validators set of height 1")
}
