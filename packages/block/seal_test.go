/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestHeldTrialWaitsForItsWrites checks the commit of a held trial waits for the writes after its
// seal and takes their error, and a drop stops its writer and waits for the writes too
func TestHeldTrialWaitsForItsWrites(t *testing.T) {
	require.NoError(t, (&heldTrial{}).wait(), "a trial that wrote its block before it was held")

	tr := &heldTrial{done: make(chan struct{})}
	waited := make(chan error, 1)
	go func() { waited <- tr.wait() }()
	select {
	case <-waited:
		t.Fatal("the commit did not wait for the writes of the trial")
	case <-time.After(50 * time.Millisecond):
	}
	tr.err = ErrTrialDropped
	close(tr.done)
	require.ErrorIs(t, <-waited, ErrTrialDropped)

	s := &afterStream{}
	tr = &heldTrial{done: make(chan struct{}), stream: s}
	dropped := make(chan struct{})
	go func() {
		tr.drop()
		close(dropped)
	}()
	require.Eventually(t, func() bool { return tr.dropped.Load() && s.canceled.Load() }, time.Second, time.Millisecond)
	select {
	case <-dropped:
		t.Fatal("the drop did not wait for the writes of the trial")
	case <-time.After(50 * time.Millisecond):
	}
	close(tr.done)
	<-dropped
}

// afterTxs are n transactions of the block blockID with their logs
func afterTxs(blockID int64, from, n int) []*types.AfterTx {
	txs := make([]*types.AfterTx, n)
	for i := range txs {
		k := from + i
		txs[i] = &types.AfterTx{Lts: &types.LogTransaction{
			Hash:  []byte{byte(k >> 24), byte(k >> 16), byte(k >> 8), byte(k)},
			Block: blockID,
		}}
	}
	return txs
}

// TestCanceledStreamWritesNothing checks a writer canceled before it wrote its parts writes none,
// takes none for its backlog, and ends with errStreamCanceled
func TestCanceledStreamWritesNothing(t *testing.T) {
	s := newAfterStream(nil, true)
	s.cancel()
	afters := &types.AfterTxs{Txs: afterTxs(1, 0, afterPartTxs)}
	s.send(afters)
	written, err := s.stop()
	require.ErrorIs(t, err, errStreamCanceled)
	require.Zero(t, written.txs)
	require.Zero(t, s.pending.Load())
	require.Zero(t, s.backlog())
}

// TestStreamCancelBoundsStall checks how long the end of a writer takes while each statement of it
// stalls, as on a storage stall: a writer the block stops writes all it was given first, one the
// node cancels ends after its statement running. A trial drop, or the rollback of a failed seal,
// cancels the writer, so what the database takes delays neither the proposal nor the next trial.
func TestStreamCancelBoundsStall(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database")
	}
	const stall = 300 * time.Millisecond
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	sqlDB, err := conn.DB()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	// run writes parts parts of afterPartTxs transactions in a schema of its own, with a statement
	// of log_transactions that sleeps stall, cancels the writer once its first statement runs if
	// cancel, and returns what it wrote and the time the stop took
	run := func(parts int, cancel bool) (afterWritten, error, time.Duration) {
		tx := conn.Begin()
		require.NoError(t, tx.Error)
		defer tx.Rollback()
		require.NoError(t, tx.Exec(`CREATE SCHEMA seal_stall_test; SET LOCAL search_path TO seal_stall_test`).Error)
		require.NoError(t, tx.AutoMigrate(&sqldb.LogTransaction{}))
		require.NoError(t, tx.Exec(fmt.Sprintf(`CREATE FUNCTION stall() RETURNS trigger LANGUAGE plpgsql AS
			$$ BEGIN PERFORM pg_sleep(%g); RETURN NULL; END $$`, stall.Seconds())).Error)
		require.NoError(t, tx.Exec(`CREATE TRIGGER stall BEFORE INSERT ON log_transactions
			FOR EACH STATEMENT EXECUTE FUNCTION stall()`).Error)

		s := newAfterStream(sqldb.NewDbTransaction(tx), false)
		afters := &types.AfterTxs{}
		for i := range parts {
			afters.Txs = append(afters.Txs, afterTxs(1, i*afterPartTxs, afterPartTxs)...)
			s.send(afters)
		}
		require.Equal(t, int64(parts*afterPartTxs), s.pending.Load())
		s.release()
		if cancel {
			time.Sleep(stall / 3)
			s.cancel()
		}
		start := time.Now()
		written, err := s.stop()
		took := time.Since(start)
		require.Zero(t, s.pending.Load())
		var rows int64
		require.NoError(t, tx.Table("log_transactions").Count(&rows).Error)
		if err == nil {
			require.Equal(t, int64(written.txs), rows)
		}
		return written, err, took
	}

	const parts = 3
	statements := parts * afterPartTxs / afterChunk
	written, err, took := run(parts, false)
	require.NoError(t, err)
	require.Equal(t, parts*afterPartTxs, written.txs)
	require.GreaterOrEqual(t, took, time.Duration(statements-1)*stall, "the stop waits for all the writer was given")

	// the statement running at the cancel has two thirds of a stall left; a statement more would
	// take a whole one
	written, err, took = run(parts, true)
	require.ErrorIs(t, err, errStreamCanceled)
	require.Less(t, written.txs, parts*afterPartTxs)
	require.Less(t, took, 2*stall-stall/3, "a canceled writer ends after its statement running")
}

// TestDropCancelsStalledWrite checks a drop of a held trial whose writes stall in a statement, as a
// flush does on a storage stall, returns once PostgreSQL cancels the statement rather than once it
// ends, and the trial's transaction is rolled back, so the next trial waits for no lock of it
func TestDropCancelsStalledWrite(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database")
	}
	pc, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	savedDB := sqldb.DBConn
	require.NoError(t, sqldb.GormInit(conf.DBConfig{Name: pc.Database, Host: pc.Host, Port: int(pc.Port),
		User: pc.User, Password: pc.Password, MaxIdleConns: 2, MaxOpenConns: 4}))
	t.Cleanup(func() {
		sqldb.GormClose()
		sqldb.DBConn = savedDB
	})

	const stall = 600 * time.Millisecond
	dbTx, err := sqldb.StartTransaction()
	require.NoError(t, err)
	conn := dbTx.Connection()
	require.NoError(t, conn.Exec(`CREATE SCHEMA drop_stall_test; SET LOCAL search_path TO drop_stall_test`).Error)
	require.NoError(t, conn.AutoMigrate(&sqldb.LogTransaction{}))
	require.NoError(t, conn.Exec(fmt.Sprintf(`CREATE FUNCTION stall() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN PERFORM pg_sleep(%g); RETURN NULL; END $$`, stall.Seconds())).Error)
	require.NoError(t, conn.Exec(`CREATE TRIGGER stall BEFORE INSERT ON log_transactions
		FOR EACH STATEMENT EXECUTE FUNCTION stall()`).Error)

	// the writes of the trial: a statement that stalls, and the ones after it that a drop stops
	tr := &heldTrial{done: make(chan struct{}), dbTx: dbTx}
	statements := 0
	go func() {
		defer close(tr.done)
		for i := range 3 {
			if tr.dropped.Load() {
				tr.err = ErrTrialDropped
				return
			}
			statements++
			if err := conn.Create(&sqldb.LogTransaction{Hash: []byte{byte(i)}, Block: 1}).Error; err != nil {
				tr.err = err
				return
			}
		}
	}()
	time.Sleep(stall / 3)
	start := time.Now()
	tr.drop()
	took := time.Since(start)
	require.Error(t, tr.err)
	require.Equal(t, 1, statements, "the writes go on past the drop")
	// the statement running had two thirds of a stall left
	require.Less(t, took, stall/2, "the drop waits for the statement running to end")
	require.NoError(t, dbTx.Rollback())

	var schemas int64
	require.Eventually(t, func() bool {
		return sqldb.DBConn.Raw(`SELECT count(*) FROM pg_namespace WHERE nspname = 'drop_stall_test'`).Scan(&schemas).Error == nil && schemas == 0
	}, time.Second, 5*time.Millisecond, "the transaction of the dropped trial is not rolled back")
}
