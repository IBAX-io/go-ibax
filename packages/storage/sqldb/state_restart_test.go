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
