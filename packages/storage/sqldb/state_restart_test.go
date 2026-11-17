package sqldb

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestRestartTakesBackPG checks a run in order that has to run again takes back what it wrote
// in PostgreSQL: the transaction whose contract failed and paid gave no error, and a savepoint
// the view refused left the row of the run in the block
func TestRestartTakesBackPG(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	ctx := context.Background()
	plain := testConn(t)
	for _, q := range []string{
		`DROP TABLE IF EXISTS restart_a, restart_b`,
		`CREATE TABLE restart_a (id bigint PRIMARY KEY, v text NOT NULL DEFAULT '')`,
		`CREATE TABLE restart_b (id bigint PRIMARY KEY, v text NOT NULL DEFAULT '')`,
	} {
		_, err := plain.ExecContext(ctx, q)
		require.NoError(t, err)
	}
	t.Cleanup(func() { plain.ExecContext(ctx, `DROP TABLE IF EXISTS restart_a, restart_b`) })
	bumpSchema()

	db, err := openDB(dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)
	tx := gdb.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { tx.Rollback() })
	st, err := NewBlockState(NewDbTransaction(tx), []string{""})
	require.NoError(t, err)

	// a read the state does not run of a table the run changed: the table goes to PostgreSQL
	unsupported := func(table string) string { return `SELECT id FROM "` + table + `" WHERE lower(v) = 'x'` }

	// the first run writes restart_a in memory and has to run again
	v := st.NewOrderedView(0, "")
	vtx := v.Transaction()
	require.NoError(t, vtx.Savepoint("run"))
	require.NoError(t, vtx.conn.Exec(`INSERT INTO "restart_a" (id, v) VALUES (1, 'a')`).Error)
	require.Error(t, vtx.conn.Exec(unsupported("restart_a")).Error)
	require.True(t, v.Restart())
	require.NoError(t, vtx.RollbackSavepoint("run"))
	require.NoError(t, v.Prepare())

	// the second writes restart_a in PostgreSQL, restart_b in memory, and has to run again
	v = st.NewOrderedView(0, "")
	vtx = v.Transaction()
	require.NoError(t, vtx.Savepoint("run"))
	require.NoError(t, vtx.conn.Exec(`INSERT INTO "restart_a" (id, v) VALUES (1, 'a')`).Error)
	require.NoError(t, vtx.conn.Exec(`INSERT INTO "restart_b" (id, v) VALUES (1, 'b')`).Error)
	require.Error(t, vtx.conn.Exec(unsupported("restart_b")).Error)
	require.True(t, v.Restart())
	require.NoError(t, vtx.RollbackSavepoint("run"))
	require.NoError(t, v.Prepare())

	// the third sees none of the rows of the runs before it
	v = st.NewOrderedView(0, "")
	vtx = v.Transaction()
	require.NoError(t, vtx.Savepoint("run"))
	var n int64
	require.NoError(t, vtx.conn.Raw(`SELECT count(*) FROM "restart_a"`).Scan(&n).Error)
	require.Zero(t, n)
	require.NoError(t, vtx.conn.Raw(`SELECT count(*) FROM "restart_b"`).Scan(&n).Error)
	require.Zero(t, n)
	require.False(t, v.Restart())
}

// TestFailedReadRollsBack checks a run in order whose read fails in PostgreSQL, before the block
// has a table there, leaves the transaction of the block usable once it rolls back to its
// savepoint: the savepoint the view had in memory only is made in PostgreSQL before the read
func TestFailedReadRollsBack(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	ctx := context.Background()
	plain := testConn(t)
	for _, q := range []string{
		`DROP TABLE IF EXISTS failread_a, failread_b`,
		`CREATE TABLE failread_a (id bigint PRIMARY KEY, v text NOT NULL DEFAULT '')`,
		`CREATE TABLE failread_b (id bigint PRIMARY KEY, v text NOT NULL DEFAULT '')`,
	} {
		_, err := plain.ExecContext(ctx, q)
		require.NoError(t, err)
	}
	t.Cleanup(func() { plain.ExecContext(ctx, `DROP TABLE IF EXISTS failread_a, failread_b`) })
	bumpSchema()

	db, err := openDB(dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)
	tx := gdb.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { tx.Rollback() })
	st, err := NewBlockState(NewDbTransaction(tx), []string{""})
	require.NoError(t, err)

	// the transaction of a contract that fails: it writes a table in memory, its read of a column
	// another table has not fails in PostgreSQL, and it rolls back to its savepoint
	v := st.NewOrderedView(0, "")
	vtx := v.Transaction()
	require.NoError(t, vtx.Savepoint("tx"))
	require.NoError(t, vtx.conn.Exec(`INSERT INTO "failread_b" (id, v) VALUES (1, 'a')`).Error)
	require.Error(t, vtx.conn.Exec(`SELECT nosuch FROM "failread_a" WHERE id = 1`).Error)
	require.False(t, v.Restart())
	require.NoError(t, vtx.RollbackSavepoint("tx"))

	// the payment of the failure and the next statements of the block run
	require.NoError(t, vtx.conn.Exec(`INSERT INTO "failread_b" (id, v) VALUES (2, 'b')`).Error)
	var ids []int64
	require.NoError(t, vtx.conn.Raw(`SELECT id FROM "failread_b" WHERE id = 1`).Scan(&ids).Error)
	require.Empty(t, ids)
	require.NoError(t, vtx.conn.Raw(`SELECT id FROM "failread_b" WHERE id = 2`).Scan(&ids).Error)
	require.Equal(t, []int64{2}, ids)
	require.False(t, v.Restart())
	// the transaction of the block is not aborted
	var one int
	require.NoError(t, tx.Raw(`SELECT 1`).Scan(&one).Error)
}
