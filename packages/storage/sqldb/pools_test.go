/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// The blocks run in a pool of their own: with every connection of DBConn held by requests, the
// transaction of a block and the connections of its workers still open at once, and none of them
// takes a connection of DBConn
func TestBlocksHaveTheirOwnPool(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	pc, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	savedDB, savedExec, savedWorkers := DBConn, execConn, conf.Config.Exec.Workers
	t.Cleanup(func() {
		DBConn, execConn, conf.Config.Exec.Workers = savedDB, savedExec, savedWorkers
	})
	const workers = 3
	conf.Config.Exec.Workers = workers
	require.NoError(t, GormInit(conf.DBConfig{Host: pc.Host, Port: int(pc.Port), User: pc.User, Name: pc.Database,
		Password: pc.Password, LockTimeout: 5000, IdleInTxTimeout: 5000, MaxIdleConns: 1, MaxOpenConns: 1}))
	defer GormClose()

	api, err := DBConn.DB()
	require.NoError(t, err)
	exec, err := execConn.DB()
	require.NoError(t, err)
	require.NotSame(t, api, exec)
	require.Equal(t, execConns(workers), exec.Stats().MaxOpenConnections)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := api.Conn(ctx)
	require.NoError(t, err)
	defer request.Close()

	tx, err := StartTransaction()
	require.NoError(t, err)
	defer tx.Rollback()
	var one int
	require.NoError(t, tx.conn.Raw(`SELECT 1`).Scan(&one).Error)
	conns := make([]*sql.Conn, 0, workers+1)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for range workers + 1 {
		c, err := PinConn(ctx)
		require.NoError(t, err)
		require.NoError(t, c.QueryRowContext(ctx, `SELECT 1`).Scan(&one))
		conns = append(conns, c)
	}
	require.Equal(t, 1, api.Stats().InUse)
	require.Equal(t, workers+2, exec.Stats().InUse)
}
