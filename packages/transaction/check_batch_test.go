/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"errors"
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// windowDB is a transaction it rolls back on the database of a node named by IBAX_TEST_DSN, without
// blocks, logs of transactions and StopNetwork hashes; sqldb.DBConn is the transaction meanwhile,
// and the window of the duplicates is loaded from it
func windowDB(t *testing.T) *gorm.DB {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	tx := conn.Begin()
	require.NoError(t, tx.Error)
	saved := sqldb.DBConn
	sqldb.DBConn = tx
	sqldb.ResetTxWindow()
	t.Cleanup(func() {
		sqldb.DBConn = saved
		sqldb.ResetTxWindow()
		tx.Rollback()
	})
	for _, table := range []string{"block_chain", "log_transactions"} {
		require.NoError(t, tx.Exec(`DELETE FROM "`+table+`"`).Error)
	}
	return tx
}

// logBlock commits the block id of time with the transactions of the hashes
func logBlock(t *testing.T, db *gorm.DB, id, time int64, hashes ...string) {
	require.NoError(t, db.Create(&sqldb.BlockChain{ID: id, Hash: []byte{byte(id)}, RollbacksHash: []byte{0},
		Data: []byte{0}, CandidateNodes: []byte{0}, Time: time}).Error)
	for _, h := range hashes {
		require.NoError(t, db.Create(&sqldb.LogTransaction{Hash: []byte(h), Block: id}).Error)
	}
}

func checkErrs(t *testing.T, checkTime int64, txs ...*Transaction) []error {
	errs := CheckBatch(txs, checkTime)
	require.Len(t, errs, len(txs))
	return errs
}

// TestCheckBatchWindow: a transaction a committed block has is refused as a duplicate, from the
// window loaded at the start and from the blocks committed after; one signed more than MaxTxBack
// before is refused as expired; a StopNetwork transaction stays a duplicate past the window; a
// restart loads the window again
func TestCheckBatchWindow(t *testing.T) {
	db := windowDB(t)
	const T = int64(1) << 32
	ms := func(s int64) int64 { return s * 1000 }
	logBlock(t, db, 30, T, "dup")
	// a StopNetwork transaction of a block long before the window, which log_transactions keeps
	logBlock(t, db, 1, T-100000, "stop")
	require.NoError(t, sqldb.LoadTxWindow())

	errs := checkErrs(t, T+1,
		keyTx("dup", 1, ms(T), 1),
		keyTx("fresh", 1, ms(T), 1),
		keyTx("expired", 1, ms(T+1-consts.MaxTxBack-1), 1),
		keyTx("edge", 1, ms(T+1-consts.MaxTxBack), 1),
		poolTx("stop", types.StopNetworkTxType, ms(T-100000), 1))
	require.ErrorIs(t, errs[0], ErrDuplicatedTx)
	require.NoError(t, errs[1])
	require.ErrorIs(t, errs[2], ErrExpiredTime)
	require.NoError(t, errs[3])
	require.ErrorIs(t, errs[4], ErrDuplicatedTx)

	// a block the node commits adds its transactions
	logBlock(t, db, 31, T+1, "fresh")
	require.NoError(t, sqldb.TxWindow.Add(T+1, []sqldb.WindowTx{{Hash: []byte("fresh"), Signed: T}}))
	require.ErrorIs(t, checkErrs(t, T+2, keyTx("fresh", 1, ms(T), 1))[0], ErrDuplicatedTx)

	// past MaxTxBack the repeat is refused as expired, whether or not the window still holds it
	late := T + consts.MaxTxForw + consts.MaxTxBack + 1
	logBlock(t, db, 32, late)
	require.NoError(t, sqldb.TxWindow.Add(late, nil))
	for _, h := range []string{"dup", "fresh"} {
		err := checkErrs(t, late, keyTx(h, 1, ms(T), 1))[0]
		require.True(t, errors.Is(err, ErrExpiredTime), "%s: %v", h, err)
	}

	// a restart loads the transactions of the window
	sqldb.ResetTxWindow()
	require.NoError(t, sqldb.LoadTxWindow())
	require.ErrorIs(t, checkErrs(t, late, keyTx("fresh", 1, ms(late), 1))[0], ErrDuplicatedTx, "the hash is the one of block 31")
}
