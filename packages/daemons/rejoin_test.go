package daemons

import (
	"os"
	"testing"

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
