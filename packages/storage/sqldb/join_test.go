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
	"github.com/IBAX-io/go-ibax/packages/migration"
	"github.com/stretchr/testify/require"
)

// setRows makes the sets from the heights 2, of block 1, pb, of block pb-1 in range 0, and 2pb,
// of block 2pb-1 in range 1, and the certificates of the blocks of chainRows
func setRows(t *testing.T, db *DbTransaction) {
	for _, h := range []int64{2, pb, 2 * pb} {
		require.NoError(t, (&BftValidatorSet{FromHeight: h, Data: []byte(fmt.Sprintf("set %d", h))}).Create(db))
	}
	for _, h := range []int64{pb - 1, pb, 2*pb - 1} {
		require.NoError(t, (&BftCertificate{Height: h, Hash: []byte{byte(h)}, Data: []byte(fmt.Sprintf("cert %d", h))}).Create(db))
	}
}

// retainTo2pb makes the rows of chainRows and setRows and takes range 0 out at the boundary 2pb
func retainTo2pb(t *testing.T, db *DbTransaction) {
	conf.Config.Storage.Archive = false
	T := int64(1) << 32
	chainRows(t, db, T)
	setRows(t, db)
	require.NoError(t, Retain(db, pb, testWindow))
	addBlocks(t, db, T, 2*pb, 3*pb-1)
	require.NoError(t, Retain(db, 2*pb, testWindow))
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
	require.Equal(t, []int64{pb, 2 * pb}, heightsOf(ps))
	require.Equal(t, fmt.Sprintf("cert %d", pb-1), string(ps[0].Cert))
	require.Equal(t, fmt.Sprintf("set %d", pb), string(ps[0].Set))
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "bft_set_proofs"`))
}

// TestSetProofsRetired: the proof of a set whose block the boundary retired is of bft_set_proofs
func TestSetProofsRetired(t *testing.T) {
	db := retentionDB(t)
	retainTo2pb(t, db)
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "block_chain" WHERE "id" = ?`, pb-1))
	require.EqualValues(t, 1, countRows(t, db, `SELECT count(*) FROM "bft_set_proofs" WHERE "height" = ?`, pb-1))
	ps, err := SetProofs(0, 16)
	require.NoError(t, err)
	require.Equal(t, []int64{pb, 2 * pb}, heightsOf(ps))
	require.Equal(t, fmt.Sprintf("cert %d", pb-1), string(ps[0].Cert))
	require.Equal(t, []byte{0}, ps[0].Block)
	ps, err = SetProofs(0, 1)
	require.NoError(t, err)
	require.Equal(t, []int64{pb}, heightsOf(ps))
	ps, err = SetProofs(pb, 16)
	require.NoError(t, err)
	require.Equal(t, []int64{2 * pb}, heightsOf(ps))
	ps, err = SetProofs(2*pb, 16)
	require.NoError(t, err)
	require.Empty(t, ps)

	// the rollback of the boundary puts the block back, and the proof is one still
	require.NoError(t, RestoreRetired(db, 2*pb))
	ps, err = SetProofs(0, 16)
	require.NoError(t, err)
	require.Equal(t, []int64{pb, 2 * pb}, heightsOf(ps))
}

// TestPrepareJoined: after the import of a state of the boundary 2pb, the node tables have the
// ranges of rollback_tx, the partitions the boundary retired are tables of the retired schema, and
// the window starts at range 1
func TestPrepareJoined(t *testing.T) {
	db := retentionDB(t)
	retainTo2pb(t, db)
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
	require.EqualValues(t, pb, first)
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
	require.NoError(t, Retain(db, 3*pb, testWindow))
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
	require.NoError(t, (&SnapshotJoin{Height: 2 * pb, Hash: []byte{1}}).Create(db))
	h, err = JoinedHeight(db)
	require.NoError(t, err)
	require.EqualValues(t, 2*pb, h)
	require.NoError(t, CheckRollbackTo(db, 2*pb))
	require.True(t, errors.Is(CheckRollbackTo(db, 2*pb-1), ErrBeyondRetention))
}

// TestInsertJoined: the blocks of the window go in with their certificates
func TestInsertJoined(t *testing.T) {
	db := retentionDB(t)
	require.NoError(t, InsertJoined(db, nil, nil))
	var blocks []BlockChain
	var certs []BftCertificate
	for h := int64(pb); h < pb+100; h++ {
		blocks = append(blocks, BlockChain{ID: h, Hash: []byte{byte(h)}, RollbacksHash: []byte{0}, Data: []byte{0},
			CandidateNodes: []byte{0}})
		certs = append(certs, BftCertificate{Height: h, Hash: []byte{byte(h)}, Data: []byte{1}})
	}
	require.NoError(t, InsertJoined(db, blocks, certs))
	require.EqualValues(t, 100, countRows(t, db, `SELECT count(*) FROM "block_chain"`))
	require.EqualValues(t, 100, countRows(t, db, `SELECT count(*) FROM "bft_certificates"`))
}
