package sqldb

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestNumIndexesKept checks the numbers of indexes the schema keeps: PostgreSQL counts the indexes
// of a table once, and again after each statement that may change the schema, in a transaction
// run in PostgreSQL or in the order of a block, rolled back or committed, and after a change of
// the schema; while a transaction that ran one is open, each transaction counts them itself
func TestNumIndexesKept(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	ctx := context.Background()
	plain := testConn(t)
	for _, q := range []string{
		`DROP TABLE IF EXISTS b7_ix`,
		`CREATE TABLE b7_ix (id bigint PRIMARY KEY, a text NOT NULL DEFAULT '', b text NOT NULL DEFAULT '')`,
	} {
		_, err := plain.ExecContext(ctx, q)
		require.NoError(t, err)
	}
	t.Cleanup(func() { plain.ExecContext(ctx, `DROP TABLE IF EXISTS b7_ix`) })
	bumpSchema()

	db, err := openDB(dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)

	var counts atomic.Int64
	StatementTrace = func(query string, _ []any) {
		if strings.Contains(query, "pg_class t, pg_class i") {
			counts.Add(1)
		}
	}
	t.Cleanup(func() { StatementTrace = nil })
	begin := func() *DbTransaction {
		cctx, cache := withTxCache(ctx)
		conn := gdb.WithContext(cctx).Begin()
		require.NoError(t, conn.Error)
		tr := &DbTransaction{conn: conn, cache: cache}
		t.Cleanup(func() { conn.Rollback() })
		return tr
	}
	// num is the number of indexes of b7_ix in the transaction, and the counts PostgreSQL made
	num := func(tr *DbTransaction) (int, int64) {
		before := counts.Load()
		n, err := tr.NumIndexes("b7_ix")
		require.NoError(t, err)
		return n, counts.Load() - before
	}
	exec := func(tr *DbTransaction, q string) { require.NoError(t, tr.conn.Exec(q).Error) }

	// the first count is kept for the transactions after it
	tr := begin()
	n0, c := num(tr)
	require.Equal(t, int64(1), c)
	require.Equal(t, 0, n0)
	n, c := num(tr)
	require.Equal(t, [2]int64{int64(n0), 0}, [2]int64{int64(n), c})
	require.NoError(t, tr.conn.Commit().Error)
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0), 0}, [2]int64{int64(n), c})

	// an index a transaction adds and rolls back: in the transaction it counts it, as do the others
	// while it is open, and after it the number is counted again
	tr = begin()
	exec(tr, `CREATE INDEX b7_ix_a ON b7_ix (a)`)
	n, c = num(tr)
	require.Equal(t, [2]int64{int64(n0 + 1), 1}, [2]int64{int64(n), c})
	other := begin()
	n, c = num(other)
	require.Equal(t, [2]int64{int64(n0), 1}, [2]int64{int64(n), c}, "another transaction counts while one that may change the schema is open")
	n, c = num(begin())
	require.Equal(t, int64(1), c, "nothing is kept while one that may change the schema is open")
	require.NoError(t, tr.conn.Rollback().Error)
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0), 1}, [2]int64{int64(n), c})
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0), 0}, [2]int64{int64(n), c})

	// a savepoint rolled back keeps the hold until the transaction ends
	tr = begin()
	exec(tr, `SAVEPOINT "s"`)
	exec(tr, `CREATE INDEX b7_ix_a ON b7_ix (a)`)
	n, c = num(tr)
	require.Equal(t, [2]int64{int64(n0 + 1), 1}, [2]int64{int64(n), c})
	exec(tr, `ROLLBACK TO SAVEPOINT "s"`)
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0), 1}, [2]int64{int64(n), c})
	require.NoError(t, tr.conn.Commit().Error)
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0), 1}, [2]int64{int64(n), c})

	// an index added after a statement whose writes are not known
	tr = begin()
	exec(tr, `SELECT txid_current()`)
	exec(tr, `CREATE INDEX b7_ix_a ON b7_ix (a)`)
	n, c = num(tr)
	require.Equal(t, [2]int64{int64(n0 + 1), 1}, [2]int64{int64(n), c})
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0), 1}, [2]int64{int64(n), c})
	require.NoError(t, tr.conn.Rollback().Error)
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0), 1}, [2]int64{int64(n), c})

	// an index committed is counted after its transaction
	tr = begin()
	exec(tr, `CREATE INDEX b7_ix_a ON b7_ix (a)`)
	require.NoError(t, tr.conn.Commit().Error)
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0 + 1), 1}, [2]int64{int64(n), c})
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0 + 1), 0}, [2]int64{int64(n), c})

	// an index added in the order of a block, the tables of the block in PostgreSQL
	tr = begin()
	exec(tr, `SELECT txid_current()`)
	st, err := NewBlockState(tr, []string{""})
	require.NoError(t, err)
	v := st.NewOrderedView(0, "")
	vtx := v.Transaction()
	require.NoError(t, vtx.Savepoint("run"))
	n, c = num(vtx)
	require.Equal(t, [2]int64{int64(n0 + 1), 0}, [2]int64{int64(n), c})
	require.NoError(t, vtx.conn.Exec(`CREATE INDEX b7_ix_b ON b7_ix (b)`).Error)
	n, c = num(vtx)
	require.Equal(t, [2]int64{int64(n0 + 2), 1}, [2]int64{int64(n), c})
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0 + 1), 1}, [2]int64{int64(n), c})
	require.NoError(t, tr.conn.Rollback().Error)
	st.End()
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0 + 1), 1}, [2]int64{int64(n), c})

	// a change of the schema clears the numbers
	bumpSchema()
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0 + 1), 1}, [2]int64{int64(n), c})
	n, c = num(begin())
	require.Equal(t, [2]int64{int64(n0 + 1), 0}, [2]int64{int64(n), c})
	require.Zero(t, schema.held)
}
