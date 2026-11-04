/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package snapshot

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

// testDB makes a database of its own on the server of IBAX_TEST_DSN, which it drops after the
// test
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database")
	}
	crypto.InitHashAlgo("SHA256")
	admin, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close() })
	name := "snapshot_" + strings.ToLower(rand.Text()[:10])
	_, err = admin.Exec(`CREATE DATABASE ` + name)
	require.NoError(t, err)
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.Database = name
	db := stdlib.OpenDB(*cfg)
	t.Cleanup(func() {
		db.Close()
		admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`)
	})
	return db
}

func exec(t *testing.T, db *sql.DB, qs ...string) {
	t.Helper()
	for _, q := range qs {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}
}

const schema = `CREATE TABLE "1_keys" (id bigint, ecosystem bigint, amount numeric(30), pub bytea, meta jsonb, PRIMARY KEY (ecosystem, id));
CREATE TABLE "1_names" (name varchar(64) PRIMARY KEY, at timestamptz, f double precision);
CREATE TABLE "1_history" (id bigint, block_id bigint, note text, PRIMARY KEY (block_id, id)) PARTITION BY RANGE (block_id);
CREATE TABLE "1_history_0" PARTITION OF "1_history" FOR VALUES FROM (0) TO (100);
CREATE TABLE "1_history_1" PARTITION OF "1_history" FOR VALUES FROM (100) TO (200);
CREATE TABLE info_block (hash bytea, block_id integer, sent smallint);
CREATE TABLE block_chain (id bigint PRIMARY KEY, data bytea);
CREATE TABLE "1_bag" (v text, n int)`

func hashOf(t *testing.T, db *sql.DB, workers int) *State {
	t.Helper()
	s, err := Begin(context.Background(), db)
	require.NoError(t, err)
	defer s.Close()
	st, err := s.Hash(context.Background(), workers, nil)
	require.NoError(t, err)
	return st
}

// The same rows give the same hash whatever the order they were written in, the layout of the
// tables, the settings of the session that wrote them and the number of workers
func TestStateHashCanonical(t *testing.T) {
	a, b := testDB(t), testDB(t)
	exec(t, a, schema,
		`SET TimeZone TO 'Asia/Shanghai'`,
		`INSERT INTO "1_keys" VALUES (1, 1, 100, '\x01', '{"b":1,"a":2}'), (2, 1, 5, NULL, NULL), (1, 2, 7, '\x02', '[]')`,
		`INSERT INTO "1_names" VALUES ('b', '2030-01-01 08:00:00+08', 0.1), ('B', NULL, 1e300), ('a', NULL, -0.0)`,
		`INSERT INTO "1_history" VALUES (1, 5, 'x'), (2, 150, E'line\nnext\ttab')`,
		`INSERT INTO "1_bag" VALUES ('z', 1), ('y', 2), (NULL, 3)`,
		`INSERT INTO info_block VALUES ('\xaa', 7, 0)`,
		`INSERT INTO block_chain VALUES (1, '\x00')`)
	exec(t, b, schema,
		`SET TimeZone TO 'UTC'`,
		`SET extra_float_digits TO 0`,
		`INSERT INTO "1_bag" VALUES (NULL, 3), ('y', 2), ('z', 1)`,
		`INSERT INTO "1_history" VALUES (2, 150, E'line\nnext\ttab'), (1, 5, 'x')`,
		`INSERT INTO "1_names" VALUES ('a', NULL, -0.0), ('B', NULL, 1e300), ('b', '2030-01-01 00:00:00+00', 0.1)`,
		`INSERT INTO "1_keys" VALUES (1, 2, 7, '\x02', '[]'), (2, 1, 5, NULL, NULL), (1, 1, 100, '\x01', '{"a":2, "b":1}')`,
		`INSERT INTO info_block VALUES ('\xaa', 7, 1)`,
		`INSERT INTO block_chain VALUES (1, '\xff'), (2, '\x00')`,
		`UPDATE "1_keys" SET amount = amount + 0 WHERE id = 1`)
	sa, sb := hashOf(t, a, 1), hashOf(t, b, 4)
	require.Equal(t, sa.Hash, sb.Hash)
	require.Equal(t, sa.Tables, sb.Tables)
	names := make([]string, len(sa.Tables))
	for i, tb := range sa.Tables {
		names[i] = tb.Name
	}
	require.Equal(t, []string{"1_bag", "1_history", "1_keys", "1_names", "info_block"}, names)
	require.Equal(t, int64(2), sa.Tables[1].Rows)
	require.Equal(t, sa.Hash, hashOf(t, a, 3).Hash)

	// a value of the state changes the hash, and so does a column of its description
	exec(t, b, `UPDATE info_block SET hash = '\xab'`)
	require.NotEqual(t, sa.Hash, hashOf(t, b, 2).Hash)
	exec(t, b, `UPDATE info_block SET hash = '\xaa'`)
	require.Equal(t, sa.Hash, hashOf(t, b, 2).Hash)
	exec(t, b, `ALTER TABLE "1_bag" ALTER COLUMN n SET DEFAULT 0`)
	require.NotEqual(t, sa.Hash, hashOf(t, b, 2).Hash)
}

// A table with a column dropped encodes as one made without it
func TestStateHashDroppedColumn(t *testing.T) {
	a, b := testDB(t), testDB(t)
	exec(t, a, `CREATE TABLE "1_t" (id bigint PRIMARY KEY, old text, v text)`, `ALTER TABLE "1_t" DROP COLUMN old`,
		`INSERT INTO "1_t" VALUES (1, 'v')`)
	exec(t, b, `CREATE TABLE "1_t" (id bigint PRIMARY KEY, v text)`, `INSERT INTO "1_t" VALUES (1, 'v')`)
	require.Equal(t, hashOf(t, a, 1).Hash, hashOf(t, b, 1).Hash)
}

// The snapshot is the state as of Begin: what is committed after it does not change the hash
func TestStateHashSnapshot(t *testing.T) {
	db := testDB(t)
	exec(t, db, schema, `INSERT INTO "1_keys" VALUES (1, 1, 100, NULL, NULL)`)
	before := hashOf(t, db, 1)
	s, err := Begin(context.Background(), db)
	require.NoError(t, err)
	defer s.Close()
	exec(t, db, `INSERT INTO "1_keys" VALUES (2, 1, 1, NULL, NULL)`, `CREATE TABLE "1_new" (id int PRIMARY KEY)`)
	st, err := s.Hash(context.Background(), 4, nil)
	require.NoError(t, err)
	require.Equal(t, before.Hash, st.Hash)
	require.NotEqual(t, before.Hash, hashOf(t, db, 1).Hash)
}

type sinkBuf struct {
	mu   *sync.Mutex
	all  map[string]*bytes.Buffer
	name string
}

func (s sinkBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.all[s.name].Write(p)
}

func (s sinkBuf) Close() error { return nil }

// The encoding a sink takes is the one hashed, by chunks of chunkBytes
func TestStateHashChunks(t *testing.T) {
	saved := chunkBytes
	chunkBytes = 64
	t.Cleanup(func() { chunkBytes = saved })
	db := testDB(t)
	exec(t, db, schema, `INSERT INTO "1_history" SELECT g, g % 200, repeat('n', g % 50) FROM generate_series(1, 300) g`)
	var mu sync.Mutex
	all := map[string]*bytes.Buffer{}
	sink := func(name string) (io.WriteCloser, error) {
		mu.Lock()
		defer mu.Unlock()
		all[name] = &bytes.Buffer{}
		return sinkBuf{mu: &mu, all: all, name: name}, nil
	}
	s, err := Begin(context.Background(), db)
	require.NoError(t, err)
	defer s.Close()
	st, err := s.Hash(context.Background(), 3, sink)
	require.NoError(t, err)
	var h Table
	for _, tb := range st.Tables {
		if tb.Name == "1_history" {
			h = tb
		}
	}
	data := all["1_history"].Bytes()
	require.Equal(t, int64(len(data)), h.Bytes)
	require.Equal(t, int64(300), h.Rows)
	require.Equal(t, int64(bytes.Count(data, []byte{'\n'})), h.Rows)
	require.Len(t, h.Chunks, (len(data)+63)/64)
	for i, c := range h.Chunks {
		require.Equal(t, crypto.Hash(data[i*64:min(len(data), (i+1)*64)]), c)
	}
	require.Equal(t, TableHash(h), h.Hash)
	require.Equal(t, hashOf(t, db, 1).Hash, st.Hash)
}
