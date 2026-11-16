/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testPool opens a pool of the node's connections on IBAX_TEST_DSN as the connection of the node
func testPool(t *testing.T) *gorm.DB {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database")
	}
	db, err := openDB(dsn, nil)
	require.NoError(t, err)
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	savedDB, savedExec := DBConn, execConn
	DBConn, execConn = gdb, nil
	t.Cleanup(func() {
		DBConn, execConn = savedDB, savedExec
		db.Close()
	})
	return gdb
}

// TestCancelStopsStatement checks Cancel stops the statement of a transaction running in
// PostgreSQL as the server cancels it, rather than once the statement ends, and rolls the
// transaction back: its backend runs nothing more, its writes are gone, and Rollback afterwards
// succeeds
func TestCancelStopsStatement(t *testing.T) {
	gdb := testPool(t)
	const stall = 600 * time.Millisecond
	tr, err := StartTransaction()
	require.NoError(t, err)
	t.Cleanup(func() { tr.Rollback() })
	require.NoError(t, tr.conn.Exec(`CREATE SCHEMA cancel_stall_test; SET LOCAL search_path TO cancel_stall_test`).Error)
	require.NoError(t, tr.conn.Exec(`CREATE TABLE written (id int)`).Error)
	require.NoError(t, tr.conn.Exec(fmt.Sprintf(`CREATE FUNCTION stall() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN PERFORM pg_sleep(%g); RETURN NULL; END $$`, stall.Seconds())).Error)
	require.NoError(t, tr.conn.Exec(`CREATE TRIGGER stall BEFORE INSERT ON written
		FOR EACH STATEMENT EXECUTE FUNCTION stall()`).Error)
	var pid int
	require.NoError(t, tr.conn.Raw(`SELECT pg_backend_pid()`).Scan(&pid).Error)

	done := make(chan error, 1)
	go func() { done <- tr.conn.Exec(`INSERT INTO written VALUES (1)`).Error }()
	time.Sleep(stall / 3)
	start := time.Now()
	tr.Cancel()
	select {
	case err = <-done:
	case <-time.After(stall):
		t.Fatal("the statement runs on past its cancel")
	}
	took := time.Since(start)
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "%v", err)
	require.Equal(t, "57014", pgErr.Code, "the server does not cancel the statement")
	// the statement had two thirds of a stall left; the cancel takes the request and the 100 ms
	// pgx waits after it
	require.Less(t, took, stall/2, "a canceled statement runs on")
	require.NoError(t, tr.Rollback())

	// the backend stops the statement: a connection closed at once would leave it sleeping the
	// rest of its stall in PostgreSQL
	var state string
	for range 10 {
		require.NoError(t, gdb.Raw(`SELECT coalesce(max(state), '') FROM pg_stat_activity WHERE pid = ?`, pid).Scan(&state).Error)
		if state != "active" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NotEqual(t, "active", state, "the backend runs the canceled statement")
	var schemas int64
	require.NoError(t, gdb.Raw(`SELECT count(*) FROM pg_namespace WHERE nspname = 'cancel_stall_test'`).Scan(&schemas).Error)
	require.Zero(t, schemas, "the canceled transaction is not rolled back")
}

// TestTransactionEndClearsSchema checks what the state knows of the schema, read through a
// transaction that changed it, is cleared once the transaction ends, whether it is rolled back
// without the state of a block ending, as a trial dropped before its flush is, canceled, or
// committed
func TestTransactionEndClearsSchema(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database")
	}
	db, err := openDB(dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Exec(`DROP TABLE IF EXISTS public.schema_end_commit`)
		db.Close()
	})
	ctx := context.Background()
	gen := func() (uint64, bool) {
		schema.mu.Lock()
		defer schema.mu.Unlock()
		return schema.gen, schema.cat != nil
	}
	// change begins a transaction that creates a table, and loads the catalog through the
	// transaction, which has the table only the transaction sees; it returns the generation the
	// catalog is of
	change := func(name string) (*sql.Tx, context.CancelFunc, uint64) {
		conn, err := db.Conn(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { conn.Close() })
		txCtx, cancel := context.WithCancel(ctx)
		tx, err := conn.BeginTx(txCtx, nil)
		require.NoError(t, err)
		_, err = tx.Exec(`CREATE TABLE public.` + name + ` (id int)`)
		require.NoError(t, err)
		require.NoError(t, conn.Raw(func(dc any) error {
			schema.mu.Lock()
			defer schema.mu.Unlock()
			cat, err := schema.catalogLocked(ctx, dc.(*stateConn).pg)
			if err == nil && !cat.rels[name] {
				err = fmt.Errorf("the catalog has no %s", name)
			}
			return err
		}))
		g, known := gen()
		require.True(t, known)
		return tx, cancel, g
	}
	cleared := func(before uint64) {
		require.Eventually(t, func() bool {
			g, known := gen()
			return g > before && !known
		}, time.Second, 5*time.Millisecond, "the catalog the transaction read is kept")
	}

	tx, cancel, g := change("schema_end_rollback")
	require.NoError(t, tx.Rollback())
	cancel()
	cleared(g)

	tx, cancel, g = change("schema_end_cancel")
	cancel()
	cleared(g)
	require.ErrorIs(t, tx.Rollback(), sql.ErrTxDone)

	tx, cancel, g = change("schema_end_commit")
	require.NoError(t, tx.Commit())
	cancel()
	cleared(g)
}

// TestRollbackAfterCancel checks the rollback of a transaction Cancel ended returns nil however
// it races with the end database/sql makes of the transaction, so a dropped trial logs no error
func TestRollbackAfterCancel(t *testing.T) {
	testPool(t)
	for range 20 {
		tr, err := StartTransaction()
		require.NoError(t, err)
		require.NoError(t, tr.conn.Exec(`SELECT 1`).Error)
		tr.Cancel()
		require.NoError(t, tr.Rollback())
	}
}
