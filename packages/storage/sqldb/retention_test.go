/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/migration"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const testWindow = 10000

func TestOutOfWindow(t *testing.T) {
	window, _ := migration.PartitionedTableOf("rollback_tx")
	for _, c := range []struct {
		r, blockID int64
		out        bool
	}{
		{0, 10000, false},
		{0, 20000, true},
		{1, 20000, false},
		{1, 30000, true},
	} {
		out, err := outOfWindow(nil, window, c.r, c.blockID, 0, testWindow)
		require.NoError(t, err)
		require.Equal(t, c.out, out, "range %d at %d", c.r, c.blockID)
	}
	// a range of transactions is in until it is complete
	byTime, _ := migration.PartitionedTableOf("log_transactions")
	out, err := outOfWindow(nil, byTime, 1, 10000, 1<<40, testWindow)
	require.NoError(t, err)
	require.False(t, out)
	require.True(t, IsRetentionBoundary(20000))
	require.False(t, IsRetentionBoundary(20001))
	require.False(t, IsRetentionBoundary(0))
}

// testDB is a transaction it rolls back on the database of a node the migrations made, named by
// IBAX_TEST_DSN; DBConn is the transaction meanwhile
func testDB(t *testing.T) *gorm.DB {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	tx := conn.Begin()
	require.NoError(t, tx.Error)
	saved := DBConn
	DBConn = tx
	t.Cleanup(func() {
		DBConn = saved
		tx.Rollback()
	})
	return tx
}

// retentionDB is testDB with the rows of the tables the retention reads deleted
func retentionDB(t *testing.T) *DbTransaction {
	tx := testDB(t)
	for _, table := range []string{"block_chain", "confirmations", "bft_certificates", "rollback_tx", "1_history",
		"log_transactions", "spent_info", "retired_partitions", "log_transaction_ranges", "partition_id_floors",
		"retention_state", "log_transaction_counts", "stop_network_txs"} {
		require.NoError(t, tx.Exec(`DELETE FROM "`+table+`"`).Error)
	}
	return NewDbTransaction(tx)
}

// addBlocks makes the blocks of the time
func addBlocks(t *testing.T, db *DbTransaction, time int64, ids ...int64) {
	for _, b := range ids {
		require.NoError(t, (&BlockChain{ID: b, Hash: []byte{byte(b)}, RollbacksHash: []byte{0},
			Data: []byte{0}, CandidateNodes: []byte{0}, Time: time}).Create(db))
	}
}

// chainRows makes the blocks 1 and 5, the rollback rows of ids 1 to 3, a transaction and a spent
// output of block 5, of time 1000, and the blocks of the ranges 0 and 1 of time T; the blocks of
// range 2 come after the boundary 10000 makes its partitions
func chainRows(t *testing.T, db *DbTransaction, T int64) {
	tx := GetDB(db)
	addBlocks(t, db, 1000, 1, 5)
	addBlocks(t, db, T, 9999, 10000, 19999)
	for id := int64(1); id <= 3; id++ {
		require.NoError(t, tx.Create(&RollbackTx{ID: id, BlockID: 5, TxHash: []byte{5}, NameTable: "1_keys", TableID: "1",
			DataHash: []byte{0}}).Error)
	}
	require.NoError(t, tx.Create(&LogTransaction{Hash: []byte{5}, Block: 5, Timestamp: 1000}).Error)
	require.NoError(t, tx.Create(&SpentInfo{OutputTxHash: []byte{5}, OutputKeyId: 1, OutputValue: "1", Ecosystem: 1,
		BlockId: 5, InputTxHash: []byte{6}, SpentBlock: 5}).Error)
	require.NoError(t, tx.Create(&SpentInfo{OutputTxHash: []byte{7}, OutputKeyId: 1, OutputValue: "1", Ecosystem: 1,
		BlockId: 5}).Error)
}

func countRows(t *testing.T, db *DbTransaction, sql string, args ...any) int64 {
	var n int64
	require.NoError(t, GetDB(db).Raw(sql, args...).Row().Scan(&n))
	return n
}

func TestRetain(t *testing.T) {
	db := retentionDB(t)
	conf.Config.Storage.Archive = false
	T := int64(1) << 32
	chainRows(t, db, T)
	blocks := `SELECT count(*) FROM "block_chain"`

	// at 10000 range 0 is in the window, and its transactions are within MaxTxBack
	require.NoError(t, Retain(db, 10000, T, testWindow))
	addBlocks(t, db, T, 20000, 29999)
	require.EqualValues(t, 7, countRows(t, db, blocks))
	require.EqualValues(t, 1, countRows(t, db, `SELECT count(*) FROM "log_transactions"`))
	require.EqualValues(t, 1, countRows(t, db, `SELECT count(*) FROM pg_tables WHERE schemaname = ? AND tablename = ?`,
		migration.PartsSchema, "rollback_tx_p2"))

	// at 20000 range 0 leaves, block 1 stays, and the spent output of block 5 leaves with range 0 of
	// spent_info, not by a deletion of its rows; the unspent output stays
	require.NoError(t, Retain(db, 20000, T+consts.MaxTxForw+consts.MaxTxBack+1, testWindow))
	require.EqualValues(t, 5, countRows(t, db, blocks))
	found, err := (&BlockChain{}).Get(1)
	require.NoError(t, err)
	require.True(t, found)
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "rollback_tx"`))
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "log_transactions"`))
	require.EqualValues(t, 1, countRows(t, db, `SELECT count(*) FROM "spent_info"`))
	require.EqualValues(t, 1, countRows(t, db, `SELECT count(*) FROM "spent_info" WHERE "spent_block" = 0`))
	require.EqualValues(t, 1, countRows(t, db, fmt.Sprintf(`SELECT count(*) FROM "%s"."spent_info_p0"`, migration.RetiredSchema)))
	// range 0 of the seven tables, and range 1 of log_transactions, complete and older than MaxTxBack
	require.EqualValues(t, 8, countRows(t, db, `SELECT count(*) FROM "retired_partitions" WHERE "boundary" = 20000`))
	id, err := NewDbTransaction(GetDB(db)).GetNextID("rollback_tx")
	require.NoError(t, err)
	require.EqualValues(t, 4, id, "the ids of the rows that left are not given again")
	// nothing has been dropped yet: the rollback of block 20000 puts back what it took out
	require.NoError(t, CheckRollbackTo(db, 5))

	// the rollback of block 20000 puts range 0 back
	require.NoError(t, RestoreRetired(db, 20000))
	require.EqualValues(t, 7, countRows(t, db, blocks))
	require.EqualValues(t, 3, countRows(t, db, `SELECT count(*) FROM "rollback_tx"`))
	require.EqualValues(t, 2, countRows(t, db, `SELECT count(*) FROM "spent_info"`))
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "retired_partitions"`))

	// at 30000 the ranges that left at 20000 are dropped, and the chain is not rolled back before
	require.NoError(t, Retain(db, 20000, T+consts.MaxTxForw+consts.MaxTxBack+1, testWindow))
	require.NoError(t, Retain(db, 30000, T+consts.MaxTxForw+consts.MaxTxBack+2, testWindow))
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM pg_tables WHERE schemaname = ? AND tablename = ?`,
		migration.RetiredSchema, "rollback_tx_p0"))
	require.EqualValues(t, 1, countRows(t, db, `SELECT count(*) FROM pg_tables WHERE schemaname = ? AND tablename = ?`,
		migration.RetiredSchema, "rollback_tx_p1"))
	require.NoError(t, CheckRollbackTo(db, 20000))
	require.True(t, errors.Is(CheckRollbackTo(db, 19999), ErrBeyondRetention))
	first, err := FirstKeptBlock()
	require.NoError(t, err)
	require.EqualValues(t, 20000, first, "range 1 is out of the window at 30000")
}

// retainTime is the time of the retention at the boundary 20000 with rows outputs spent in range 0
func retainTime(t *testing.T, rows int64) time.Duration {
	db := retentionDB(t)
	conf.Config.Storage.Archive = false
	T := int64(1) << 32
	chainRows(t, db, T)
	require.NoError(t, GetDB(db).Exec(`INSERT INTO "spent_info" (input_tx_hash, input_index, output_tx_hash,
			output_index, output_key_id, output_value, ecosystem, block_id, type, spent_block)
		SELECT sha256(int8send(-g)), 0, sha256(int8send(g)), 0, 1 + g % 100000, 1, 1, 2 + g % 9990, 0, 10 + g % 9990
		FROM generate_series(1, ?) g`, rows).Error)
	require.NoError(t, Retain(db, 10000, T, testWindow))
	addBlocks(t, db, T, 20000, 29999)
	start := time.Now()
	require.NoError(t, Retain(db, 20000, T+consts.MaxTxBack+1, testWindow))
	took := time.Since(start)
	require.EqualValues(t, 1, countRows(t, db, `SELECT count(*) FROM "spent_info"`))
	return took
}

// TestRetainSpentRows: the boundary takes the spent outputs of a range out in a time that does not
// grow with their number. IBAX_TEST_SPENT_ROWS sets the number of the large range.
func TestRetainSpentRows(t *testing.T) {
	rows, _ := strconv.ParseInt(os.Getenv("IBAX_TEST_SPENT_ROWS"), 10, 64)
	if rows == 0 {
		t.Skip("IBAX_TEST_SPENT_ROWS sets no number of spent outputs")
	}
	// each in a transaction of its own, rolled back before the next takes the locks of the retention
	var small, large time.Duration
	t.Run("small", func(t *testing.T) { small = retainTime(t, 9999) })
	t.Run("large", func(t *testing.T) { large = retainTime(t, rows) })
	t.Logf("retention at the boundary: %v with 9999 spent outputs, %v with %d", small, large, rows)
	require.Less(t, large-small, 100*time.Millisecond)
}

func TestRetainArchive(t *testing.T) {
	db := retentionDB(t)
	conf.Config.Storage.Archive = true
	t.Cleanup(func() { conf.Config.Storage.Archive = false })
	require.NoError(t, PrepareArchive())
	T := int64(1) << 32
	chainRows(t, db, T)
	require.NoError(t, Retain(db, 10000, T, testWindow))
	addBlocks(t, db, T, 20000, 29999)
	require.NoError(t, Retain(db, 20000, T+consts.MaxTxForw+consts.MaxTxBack+1, testWindow))
	require.NoError(t, Retain(db, 30000, T+consts.MaxTxForw+consts.MaxTxBack+2, testWindow))

	// the active tables hold the window, and the reads of the API find the rest in the archive
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "rollback_tx"`))
	require.EqualValues(t, 3, countRows(t, db, `SELECT count(*) FROM `+ArchivedTable("rollback_tx")))
	b := &BlockChain{}
	found, err := b.GetArchived(5)
	require.NoError(t, err)
	require.True(t, found)
	found, err = (&BlockChain{}).Get(5)
	require.NoError(t, err)
	require.False(t, found)
	found, err = (&LogTransaction{}).GetByHash([]byte{5})
	require.NoError(t, err)
	require.True(t, found)
	require.EqualValues(t, 1, countRows(t, db, `SELECT count(*) FROM "spent_info"`))
	require.EqualValues(t, 2, countRows(t, db, `SELECT count(*) FROM `+ArchivedTable("spent_info")))
	first, err := FirstKeptBlock()
	require.NoError(t, err)
	require.EqualValues(t, 5, first)
}

func TestLogTxCount(t *testing.T) {
	db := retentionDB(t)
	conf.Config.Storage.Archive = false
	T := int64(1) << 32
	chainRows(t, db, T)
	lts := []*LogTransaction{
		{Hash: []byte{8}, Block: 9999, Timestamp: T, EcosystemID: 2},
		{Hash: []byte{9}, Block: 9999, Timestamp: T, EcosystemID: 2},
		{Hash: []byte{10}, Block: 9999, Timestamp: T, EcosystemID: 3, Status: 1},
	}
	for _, lt := range lts {
		require.NoError(t, GetDB(db).Create(lt).Error)
	}
	require.NoError(t, CountLogTransactions(GetDB(db), lts))
	count := func(eco int64) int64 {
		n, err := GetLogTxCount(db, eco)
		require.NoError(t, err)
		return n
	}
	require.EqualValues(t, 2, count(2))
	require.EqualValues(t, 0, count(3), "failed transactions are not counted")

	// the rollback of a transaction takes it off the count
	n, err := DeleteLogTransactionsByHash(db, []byte{9})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	require.EqualValues(t, 1, count(2))

	// the count outlives the rows
	require.NoError(t, Retain(db, 10000, T, testWindow))
	addBlocks(t, db, T, 20000, 29999)
	require.NoError(t, Retain(db, 20000, T+consts.MaxTxForw+consts.MaxTxBack+1, testWindow))
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "log_transactions" WHERE "ecosystem_id" = 2`))
	require.EqualValues(t, 1, count(2))
}

// A range of log_transactions leaves once its last block, plus MaxTxForw, is earlier than
// MaxTxBack before the block time: until then a transaction of it, signed at most MaxTxForw after
// its block, passes the check of its time, and its hash refuses it
func TestRetainLogTransactionsSigned(t *testing.T) {
	db := retentionDB(t)
	conf.Config.Storage.Archive = false
	T := int64(1) << 32
	chainRows(t, db, T)
	require.NoError(t, GetDB(db).Create(&LogTransaction{Hash: []byte{8}, Block: 9999, Timestamp: T}).Error)
	require.NoError(t, Retain(db, 10000, T, testWindow))
	addBlocks(t, db, T, 20000, 29999)
	logged := `SELECT count(*) FROM "log_transactions" WHERE "hash" = ?`

	require.NoError(t, Retain(db, 20000, T+consts.MaxTxForw+consts.MaxTxBack, testWindow))
	require.EqualValues(t, 1, countRows(t, db, logged, []byte{8}), "range 0 stays while a transaction of it is in the window")
	require.NoError(t, RestoreRetired(db, 20000))
	require.NoError(t, Retain(db, 20000, T+consts.MaxTxForw+consts.MaxTxBack+1, testWindow))
	require.EqualValues(t, 0, countRows(t, db, logged, []byte{8}))
}

// On an archive node a range that leaves at a boundary is read at once, out of the window, by
// the synchronization of other nodes: the blocks and the certificates of no range are out of
// reach; a rollback of the boundary puts the range back in the active tables
func TestRetainArchiveAtOnce(t *testing.T) {
	db := retentionDB(t)
	conf.Config.Storage.Archive = true
	t.Cleanup(func() { conf.Config.Storage.Archive = false })
	require.NoError(t, PrepareArchive())
	T := int64(1) << 32
	chainRows(t, db, T)
	require.NoError(t, (&BftCertificate{Height: 9999, Hash: []byte{1}, Data: []byte{1}}).Create(db))
	require.NoError(t, Retain(db, 10000, T, testWindow))
	addBlocks(t, db, T, 20000, 29999)
	readable := func(when string) {
		found, err := (&BlockChain{}).GetArchived(9999)
		require.NoError(t, err)
		require.True(t, found, "block 9999 %s", when)
		found, err = (&BftCertificate{}).GetArchived(9999)
		require.NoError(t, err)
		require.True(t, found, "the certificate of 9999 %s", when)
	}

	require.NoError(t, Retain(db, 20000, T+consts.MaxTxForw+consts.MaxTxBack+1, testWindow))
	found, err := (&BlockChain{}).Get(9999)
	require.NoError(t, err)
	require.False(t, found, "range 0 left the active table")
	readable("as range 0 leaves")
	require.NoError(t, RestoreRetired(db, 20000))
	found, err = (&BlockChain{}).Get(9999)
	require.NoError(t, err)
	require.True(t, found, "the rollback of 20000 puts range 0 back")
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "archive"."block_chain"`))

	require.NoError(t, Retain(db, 20000, T+consts.MaxTxForw+consts.MaxTxBack+1, testWindow))
	require.NoError(t, Retain(db, 30000, T+consts.MaxTxForw+consts.MaxTxBack+2, testWindow))
	readable("at the next boundary")
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM pg_tables WHERE schemaname = ?`, migration.RetiredSchema))
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "retired_partitions" WHERE "boundary" = 20000`))
}
