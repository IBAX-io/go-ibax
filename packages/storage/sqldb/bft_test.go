/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBftTables runs on the database of a node the migrations made, named by IBAX_TEST_DSN, with the
// rows of the tables of F18 deleted in the transaction testDB rolls back
func TestBftTables(t *testing.T) {
	tx := testDB(t)
	for _, table := range []string{"bft_certificates", "bft_validator_sets", "bft_sign_state", "1_bft_evidence", "1_bft_reviews"} {
		require.NoError(t, tx.Exec(`DELETE FROM "`+table+`"`).Error)
	}
	dbTx := NewDbTransaction(tx)

	cert := &BftCertificate{Height: 5, Round: 1, Hash: []byte{5}, Data: []byte{1, 2}}
	require.NoError(t, cert.Create(dbTx))
	require.NoError(t, tx.SavePoint("certificate").Error)
	require.ErrorContains(t, (&BftCertificate{Height: 5, Hash: []byte{6}, Data: []byte{6}}).Create(dbTx), "duplicate key")
	require.NoError(t, tx.RollbackTo("certificate").Error)
	got := &BftCertificate{}
	found, err := got.Get(dbTx, 5)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, cert, got)
	found, err = got.Get(dbTx, 6)
	require.NoError(t, err)
	require.False(t, found)

	for _, s := range []*BftValidatorSet{{FromHeight: 1, Data: []byte{1}}, {FromHeight: 10, Data: []byte{10}}} {
		require.NoError(t, s.Create(dbTx))
	}
	for h, want := range map[int64]byte{1: 1, 9: 1, 10: 10, 99: 10} {
		s := &BftValidatorSet{}
		found, err := s.GetAt(dbTx, h)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, []byte{want}, s.Data, "the set of height %d", h)
	}
	found, err = (&BftValidatorSet{}).GetAt(dbTx, 0)
	require.NoError(t, err)
	require.False(t, found)

	key := []byte{0xab, 0xcd}
	ev := &BftEvidence{ID: 1, PublicKey: key, Height: 7, Round: 2, Kind: 3, Data: []byte{9}, BlockID: 20}
	require.NoError(t, tx.Create(ev).Error)
	found, err = (&BftEvidence{}).Exists(dbTx, key, 7, 2, 3)
	require.NoError(t, err)
	require.True(t, found)
	found, err = (&BftEvidence{}).Exists(dbTx, key, 7, 2, 2)
	require.NoError(t, err)
	require.False(t, found)
	// the key of an evidence is unique
	require.NoError(t, tx.SavePoint("evidence").Error)
	require.ErrorContains(t, tx.Create(&BftEvidence{ID: 2, PublicKey: key, Height: 7, Round: 2, Kind: 3, Data: []byte{8}}).Error, "duplicate key")
	require.NoError(t, tx.RollbackTo("evidence").Error)

	reviews := []*BftReview{
		{ID: 1, EvidenceID: 1, PublicKey: key, StopHeight: 21, Deadline: 1000, Status: BftReviewRestored, DecidedBlock: 30, RestoreHeight: 31},
		{ID: 2, EvidenceID: 2, PublicKey: []byte{1}, StopHeight: 25, Deadline: 2000, Status: BftReviewOpen},
		{ID: 3, EvidenceID: 3, PublicKey: []byte{2}, StopHeight: 40, Deadline: 500, Status: BftReviewMaintained, DecidedBlock: 45},
	}
	for _, r := range reviews {
		require.NoError(t, tx.Create(r).Error)
	}
	for h, want := range map[int64][]int64{20: nil, 21: {1}, 25: {1, 2}, 30: {1, 2}, 31: {2}, 40: {2, 3}} {
		rs, err := (&BftReview{}).GetStopped(dbTx, h)
		require.NoError(t, err)
		var ids []int64
		for _, r := range rs {
			ids = append(ids, r.ID)
		}
		require.Equal(t, want, ids, "stopped at height %d", h)
	}
	rs, err := (&BftReview{}).GetOverdue(dbTx, 1500)
	require.NoError(t, err)
	require.Len(t, rs, 0)
	rs, err = (&BftReview{}).GetOverdue(dbTx, 2001)
	require.NoError(t, err)
	require.Len(t, rs, 1)
	require.Equal(t, int64(2), rs[0].ID)
	// one review of an evidence
	require.NoError(t, tx.SavePoint("review").Error)
	require.ErrorContains(t, tx.Create(&BftReview{ID: 4, EvidenceID: 1, PublicKey: key}).Error, "duplicate key")
	require.NoError(t, tx.RollbackTo("review").Error)

	// the sign state is written outside the transaction of a block, through DBConn, and only of the
	// last height
	for h := int64(1); h <= 3; h++ {
		require.NoError(t, (&BftSignState{Height: h, Data: []byte{byte(h)}}).Save())
	}
	require.NoError(t, (&BftSignState{Height: 3, Data: []byte{33}}).Save())
	last := &BftSignState{}
	found, err = last.GetLast()
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, &BftSignState{Height: 3, Data: []byte{33}}, last)
	var count int64
	require.NoError(t, tx.Model(&BftSignState{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
