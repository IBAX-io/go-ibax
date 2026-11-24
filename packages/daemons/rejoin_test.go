package daemons

import (
	"fmt"
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestRejoin: a node with block 1 alone tries the join again on a restart; with no snapshot to
// join from, its state must be that of block 1, and a node with more blocks does not join
func TestRejoin(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	saved := sqldb.DBConn
	t.Cleanup(func() { sqldb.DBConn = saved })
	logger := log.WithField("test", "rejoin")

	for _, c := range []struct {
		name   string
		blocks []int64
		state  int64
		err    string
	}{
		{name: "block 1 alone, its state", blocks: []int64{1}, state: 1},
		{name: "block 1 alone, the state of a failed join", blocks: []int64{1}, state: 70000, err: "of a join that failed"},
		{name: "blocks after block 1", blocks: []int64{1, 2, 3}, state: 3},
		{name: "the blocks of a join", blocks: []int64{1, 69001, 70000}, state: 70000},
	} {
		t.Run(c.name, func(t *testing.T) {
			tx := conn.Begin()
			require.NoError(t, tx.Error)
			t.Cleanup(func() { tx.Rollback() })
			require.NoError(t, tx.Exec(`CREATE SCHEMA rejoin_test; SET LOCAL search_path TO rejoin_test`).Error)
			require.NoError(t, tx.AutoMigrate(&sqldb.BlockChain{}, &sqldb.InfoBlock{}))
			for _, id := range c.blocks {
				// data no block parses from: the node has no first block data to join with
				require.NoError(t, tx.Create(&sqldb.BlockChain{ID: id, Hash: []byte{1}, Data: []byte{0}, RollbacksHash: []byte{1}, CandidateNodes: []byte{}}).Error)
			}
			require.NoError(t, tx.Create(&sqldb.InfoBlock{BlockID: c.state, Hash: []byte{1}, RollbacksHash: []byte{1}, CandidateNodes: []byte{}}).Error)
			sqldb.DBConn = tx

			err := rejoin(logger)
			if c.err == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, c.err)
		})
	}
}

// TestKeepProofs: a join keeps the proofs out of the window, and the preimages of the execution
// hashes of the changes in it up to the snapshot height, over the hashes the last blocks carry; a
// change of the state up to the set the snapshot height leaves without its proof fails the join
func TestKeepProofs(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	saved := sqldb.DBConn
	t.Cleanup(func() { sqldb.DBConn = saved })
	const first, height = 100, 150
	proof := func(h int64) sqldb.BftSetProof {
		return sqldb.BftSetProof{Height: h, Block: []byte{byte(h)}, Cert: []byte{1}, Exec: []byte(fmt.Sprintf("exec %d", h-2))}
	}
	for _, c := range []struct {
		name   string
		proofs []sqldb.BftSetProof
		err    string
	}{
		{name: "every change proved", proofs: []sqldb.BftSetProof{proof(79), proof(119), proof(152), proof(160)}},
		{name: "the change of the set the snapshot height leaves", proofs: []sqldb.BftSetProof{proof(79), proof(119)},
			err: "no proof of the change of the validators set at height 153"},
	} {
		t.Run(c.name, func(t *testing.T) {
			tx := conn.Begin()
			require.NoError(t, tx.Error)
			t.Cleanup(func() { tx.Rollback() })
			require.NoError(t, tx.Exec(`CREATE SCHEMA keep_proofs_test; SET LOCAL search_path TO keep_proofs_test`).Error)
			require.NoError(t, tx.AutoMigrate(&sqldb.BftValidatorSet{}, &sqldb.BftSetProof{}, &sqldb.BftExecHash{}))
			for _, f := range []int64{2, 80, 120, 153, 161} {
				require.NoError(t, tx.Create(&sqldb.BftValidatorSet{FromHeight: f, Data: []byte{byte(f)}}).Error)
			}
			sqldb.DBConn = tx
			execs := []sqldb.BftExecHash{{Height: height - 1, Hash: []byte{1}, Data: []byte{}}, {Height: height, Hash: crypto.Hash([]byte("exec 150"))}}
			err := keepProofs(nil, first, height, c.proofs, execs)
			if c.err != "" {
				require.ErrorContains(t, err, c.err)
				return
			}
			require.NoError(t, err)
			var proofs []sqldb.BftSetProof
			require.NoError(t, tx.Order("height").Find(&proofs).Error)
			require.Equal(t, []sqldb.BftSetProof{proof(79)}, proofs, "the proofs out of the window")
			var hashes []sqldb.BftExecHash
			require.NoError(t, tx.Order("height").Find(&hashes).Error)
			require.Equal(t, []sqldb.BftExecHash{
				{Height: 117, Hash: crypto.Hash([]byte("exec 117")), Data: []byte("exec 117")},
				{Height: height - 1, Hash: []byte{1}, Data: []byte{}},
				{Height: height, Hash: crypto.Hash([]byte("exec 150")), Data: []byte("exec 150")},
			}, hashes)
		})
	}
}
