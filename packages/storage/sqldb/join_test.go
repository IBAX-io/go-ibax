/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"errors"
	"fmt"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/migration"
	"github.com/stretchr/testify/require"
)

// setRows makes the sets from the heights 2, of block 1, 10000, of block 9999 in range 0, and
// 20000, of block 19999 in range 1, and the certificates of the blocks of chainRows
func setRows(t *testing.T, db *DbTransaction) {
	for _, h := range []int64{2, 10000, 20000} {
		require.NoError(t, (&BftValidatorSet{FromHeight: h, Data: []byte(fmt.Sprintf("set %d", h))}).Create(db))
	}
	for _, h := range []int64{9999, 10000, 19999} {
		require.NoError(t, (&BftCertificate{Height: h, Hash: []byte{byte(h)}, Data: []byte(fmt.Sprintf("cert %d", h))}).Create(db))
	}
}

// retainTo20000 makes the rows of chainRows and setRows and takes range 0 out at the boundary 20000
func retainTo20000(t *testing.T, db *DbTransaction) {
	conf.Config.Storage.Archive = false
	T := int64(1) << 32
	chainRows(t, db, T)
	setRows(t, db)
	require.NoError(t, Retain(db, 10000, T, testWindow))
	addBlocks(t, db, T, 20000, 29999)
	require.NoError(t, Retain(db, 20000, T+consts.MaxTxForw+consts.MaxTxBack+1, testWindow))
}

func heightsOf(ps []SetProof) []int64 {
	hs := make([]int64, len(ps))
	for i, p := range ps {
		hs[i] = p.FromHeight
	}
	return hs
}

// TestSetProofs: the proofs of the sets are of the blocks of the window; block 1 has none
func TestSetProofs(t *testing.T) {
	db := retentionDB(t)
	conf.Config.Storage.Archive = false
	chainRows(t, db, 1<<32)
	setRows(t, db)
	ps, err := SetProofs(0, 16)
	require.NoError(t, err)
	require.Equal(t, []int64{10000, 20000}, heightsOf(ps))
	require.Equal(t, "cert 9999", string(ps[0].Cert))
	require.Equal(t, "set 10000", string(ps[0].Set))
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "bft_set_proofs"`))
}

// TestSetProofsRetired: the proof of a set whose block the boundary retired is of bft_set_proofs
func TestSetProofsRetired(t *testing.T) {
	db := retentionDB(t)
	retainTo20000(t, db)
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "block_chain" WHERE "id" = 9999`))
	require.EqualValues(t, 1, countRows(t, db, `SELECT count(*) FROM "bft_set_proofs" WHERE "height" = 9999`))
	ps, err := SetProofs(0, 16)
	require.NoError(t, err)
	require.Equal(t, []int64{10000, 20000}, heightsOf(ps))
	require.Equal(t, "cert 9999", string(ps[0].Cert))
	require.Equal(t, []byte{0}, ps[0].Block)
	ps, err = SetProofs(0, 1)
	require.NoError(t, err)
	require.Equal(t, []int64{10000}, heightsOf(ps))
	ps, err = SetProofs(10000, 16)
	require.NoError(t, err)
	require.Equal(t, []int64{20000}, heightsOf(ps))
	ps, err = SetProofs(20000, 16)
	require.NoError(t, err)
	require.Empty(t, ps)

	// the rollback of the boundary puts the block back, and the proof is one still
	require.NoError(t, RestoreRetired(db, 20000))
	ps, err = SetProofs(0, 16)
	require.NoError(t, err)
	require.Equal(t, []int64{10000, 20000}, heightsOf(ps))
}

// TestPrepareJoined: after the import of a state of the boundary 20000, the node tables have the
// ranges of rollback_tx, the partitions the boundary retired are tables of the retired schema, and
// the window starts at range 1
func TestPrepareJoined(t *testing.T) {
	db := retentionDB(t)
	retainTo20000(t, db)
	tx := GetDB(db)
	retired, err := retiredBefore(db, "<=", 1<<62)
	require.NoError(t, err)
	require.NotEmpty(t, retired)
	// what the snapshot does not carry, and a node of other partitions before the join
	for _, r := range retired {
		require.NoError(t, tx.Exec(fmt.Sprintf(`DROP TABLE "%s"."%s"`, migration.RetiredSchema,
			migration.PartitionName(r.TableName, r.K))).Error)
	}
	require.NoError(t, tx.Exec(fmt.Sprintf(`DROP TABLE "%s"."%s"`, migration.PartsSchema,
		migration.PartitionName("confirmations", 1))).Error)
	require.NoError(t, tx.Exec(migration.CreatePartitionSQL("block_chain", 7)).Error)

	first, err := PrepareJoined(db)
	require.NoError(t, err)
	require.EqualValues(t, 10000, first)
	want, err := attachedRanges(db, "rollback_tx")
	require.NoError(t, err)
	require.Equal(t, int64(1), want[0])
	for _, name := range []string{"block_chain", "confirmations", "bft_certificates"} {
		got, err := attachedRanges(db, name)
		require.NoError(t, err)
		require.Equal(t, want, got, name)
	}
	require.EqualValues(t, len(retired), countRows(t, db, `SELECT count(*) FROM pg_tables WHERE schemaname = ?`,
		migration.RetiredSchema))
	// the next boundary drops the stubs as it drops what it retired
	require.NoError(t, Retain(db, 30000, int64(1)<<32+consts.MaxTxForw+consts.MaxTxBack+2, testWindow))
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM pg_tables WHERE schemaname = ? AND tablename = ?`,
		migration.RetiredSchema, "rollback_tx_p0"))
}

// TestJoinedRollback: a node that joined at a height is not rolled back below it
func TestJoinedRollback(t *testing.T) {
	db := retentionDB(t)
	h, err := JoinedHeight(db)
	require.NoError(t, err)
	require.EqualValues(t, 0, h)
	require.NoError(t, CheckRollbackTo(db, 1))
	require.NoError(t, (&SnapshotJoin{Height: 20000, Hash: []byte{1}}).Create(db))
	h, err = JoinedHeight(db)
	require.NoError(t, err)
	require.EqualValues(t, 20000, h)
	require.NoError(t, CheckRollbackTo(db, 20000))
	require.True(t, errors.Is(CheckRollbackTo(db, 19999), ErrBeyondRetention))
}

// TestInsertJoined: the blocks of the window go in with their certificates
func TestInsertJoined(t *testing.T) {
	db := retentionDB(t)
	require.NoError(t, InsertJoined(db, nil, nil))
	var blocks []BlockChain
	var certs []BftCertificate
	for h := int64(10000); h < 10100; h++ {
		blocks = append(blocks, BlockChain{ID: h, Hash: []byte{byte(h)}, RollbacksHash: []byte{0}, Data: []byte{0},
			CandidateNodes: []byte{0}})
		certs = append(certs, BftCertificate{Height: h, Hash: []byte{byte(h)}, Data: []byte{1}})
	}
	require.NoError(t, InsertJoined(db, blocks, certs))
	require.EqualValues(t, 100, countRows(t, db, `SELECT count(*) FROM "block_chain"`))
	require.EqualValues(t, 100, countRows(t, db, `SELECT count(*) FROM "bft_certificates"`))
}
