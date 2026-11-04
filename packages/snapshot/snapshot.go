/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package snapshot takes the state of the database of a node as of a height and its state hash
// (P7 design 4.3): every node computes the same hash from the same state, whatever the order the
// rows were written in, the physical layout of the tables or the settings of the server.
//
// The state is every table of the schema but those of the chain history and those of the node
// alone (nodeTables), and the columns of the node alone (nodeColumns). A table encodes as the
// output of COPY TO STDOUT in the text format of its rows in the order of its primary key, the
// text of a column in the "C" collation, and of the text of the whole row for a table without
// one, under fixed session settings. The encoding splits into chunks of chunkBytes; a table hash
// is the hash of its description, its rows, its bytes and the hashes of its chunks; the state
// hash is the hash of the names of the tables, in the "C" order, each with its table hash. The
// hash function is that of the chain. A chunk is the unit a node verifies of a snapshot it
// downloads. The description of a table is its schema, Desc, and the entry SchemaEntry describes
// the functions and the views (schema.go), so that the state hash covers what an import creates.
//
// Begin takes a snapshot of the database in a read-only transaction of REPEATABLE READ, which
// the node opens right after it commits the block of the height and before it commits the next:
// the hash is then computed in the background, by workers that import the snapshot, without
// holding the database for the blocks that follow.
package snapshot

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/jackc/pgx/v5/stdlib"
)

// chunkBytes is the length of the chunks the encoding of a table splits into: a rule of the
// chain, as the hash function, since the table hash takes the hashes of its chunks
var chunkBytes = 1 << 20

// nodeTables are the tables out of the state: the history of the chain, which blocks and their
// certificates carry, and what a node writes of its own
var nodeTables = map[string]bool{
	"block_chain":         true,
	"bft_certificates":    true,
	"confirmations":       true,
	"rollback_tx":         true,
	"bft_sign_state":      true,
	"bft_state_hashes":    true,
	"install":             true,
	"migration_history":   true,
	"queue_blocks":        true,
	"stop_daemons":        true,
	"transactions_status": true,
}

// nodeColumns are the columns of the tables of the state a node writes of its own
var nodeColumns = map[string][]string{
	"info_block": {"sent"},
}

// session is the settings the encoding of the rows depends on
var session = []string{
	`SET LOCAL search_path TO public`,
	`SET LOCAL TimeZone TO 'UTC'`,
	`SET LOCAL DateStyle TO 'ISO, YMD'`,
	`SET LOCAL IntervalStyle TO 'postgres'`,
	`SET LOCAL extra_float_digits TO 1`,
	`SET LOCAL bytea_output TO 'hex'`,
	`SET LOCAL client_encoding TO 'UTF8'`,
}

// Table is a table of the state and its hash
type Table struct {
	Name string
	// Desc is the canonical description of its columns and its primary key
	Desc string
	// Rows and Bytes are the rows and the length of its encoding
	Rows, Bytes int64
	// Chunks are the hashes of the chunks of its encoding, in their order
	Chunks [][]byte
	Hash   []byte
}

// State is the tables of the state, in the "C" order of their names, and the state hash
type State struct {
	Tables []Table
	Hash   []byte
}

// Sink takes the encoding of each table of the state, which it writes to the writer it returns
// and closes after the last byte; a nil Sink takes nothing
type Sink func(table string) (io.WriteCloser, error)

// Snapshot is the state of the database as of the moment Begin took it
type Snapshot struct {
	db     *sql.DB
	conn   *sql.Conn
	id     string
	layout map[string]string
}

// Begin takes a snapshot of the database: what it has committed so far, and nothing it commits
// after Begin returns. The caller calls it before anything changes the tables of the state after
// what the snapshot is of: Hash checks that their layout is still that of Begin.
func Begin(ctx context.Context, db *sql.DB) (*Snapshot, error) {
	conn, err := begin(ctx, db, "")
	if err != nil {
		return nil, err
	}
	s := &Snapshot{db: db, conn: conn}
	if err := conn.QueryRowContext(ctx, `SELECT pg_export_snapshot()`).Scan(&s.id); err != nil {
		end(conn)
		return nil, fmt.Errorf("exporting the snapshot: %w", err)
	}
	if s.layout, err = layouts(ctx, conn); err != nil {
		end(conn)
		return nil, fmt.Errorf("reading the layout of the tables: %w", err)
	}
	return s, nil
}

// layouts reads the storage of each table, the files of its rows and those of its partitions, as
// the catalog has it in the transaction of conn. A snapshot sees the rows of a table as of its moment only while the
// storage is the same: PostgreSQL gives the transactions of an earlier snapshot the rows of the
// new storage of a table rewritten, truncated or with partitions detached or attached, the
// changes that are not MVCC-safe.
func layouts(ctx context.Context, conn *sql.Conn) (map[string]string, error) {
	// the catalog alone, as of the snapshot of the transaction: no function that locks a table
	rows, err := conn.QueryContext(ctx, `WITH RECURSIVE tree(root, rel) AS (
			SELECT r.oid, r.oid FROM pg_class r JOIN pg_namespace n ON n.oid = r.relnamespace
			WHERE n.nspname = 'public' AND r.relkind IN ('r', 'p') AND NOT r.relispartition
			UNION ALL SELECT t.root, i.inhrelid FROM tree t JOIN pg_inherits i ON i.inhparent = t.rel)
		SELECT r.relname, string_agg(c.oid::text || ':' || c.relfilenode::text, ',' ORDER BY c.oid)
		FROM tree t JOIN pg_class r ON r.oid = t.root JOIN pg_class c ON c.oid = t.rel
		GROUP BY r.relname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all := map[string]string{}
	for rows.Next() {
		var name, layout string
		if err := rows.Scan(&name, &layout); err != nil {
			return nil, err
		}
		all[name] = layout
	}
	return all, rows.Err()
}

// Close ends the transaction of the snapshot
func (s *Snapshot) Close() {
	if s.conn != nil {
		end(s.conn)
		s.conn = nil
	}
}

// begin takes a connection of db in a read-only transaction of REPEATABLE READ under the
// settings of the encoding, of the snapshot id if not empty
func begin(ctx context.Context, db *sql.DB, id string) (*sql.Conn, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	stmts := []string{`BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY`}
	if id != "" {
		stmts = append(stmts, fmt.Sprintf(`SET TRANSACTION SNAPSHOT '%s'`, id))
	}
	for _, q := range append(stmts, session...) {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			end(conn)
			return nil, fmt.Errorf("%s: %w", q, err)
		}
	}
	return conn, nil
}

// end ends the transaction of a connection and gives the connection back, or drops it if the
// transaction does not end
func end(conn *sql.Conn) {
	if _, err := conn.ExecContext(context.Background(), `ROLLBACK`); err != nil {
		conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	conn.Close()
}

// table is a table of the state as the snapshot describes it
type table struct {
	oid   uint32
	kind  byte
	name  string
	desc  string
	query string
}

// tables reads the tables of the state of the snapshot and how to encode each
func (s *Snapshot) tables(ctx context.Context) ([]table, error) {
	rows, err := s.conn.QueryContext(ctx, `SELECT c.oid, c.relkind, c.relname FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND NOT c.relispartition
		ORDER BY c.relname COLLATE "C"`)
	if err != nil {
		return nil, err
	}
	var ts []table
	for rows.Next() {
		var t table
		var kind string
		if err := rows.Scan(&t.oid, &kind, &t.name); err != nil {
			rows.Close()
			return nil, err
		}
		t.kind = kind[0]
		if !nodeTables[t.name] {
			ts = append(ts, t)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range ts {
		if err := s.describe(ctx, &ts[i]); err != nil {
			return nil, fmt.Errorf("describing %s: %w", ts[i].name, err)
		}
	}
	schema, err := s.describeSchema(ctx)
	if err != nil {
		return nil, fmt.Errorf("describing the schema: %w", err)
	}
	ts = append(ts, schema)
	// the order of the bytes of the names, that of the "C" collation
	slices.SortFunc(ts, func(a, b table) int { return strings.Compare(a.name, b.name) })
	return ts, nil
}

func quote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Hash computes the state of the snapshot and its hash by workers, and gives the encoding of each
// table to sink. Each table encodes in a transaction of its own of the snapshot, so that what the
// node commits meanwhile waits for the lock of a table only while the table encodes. A table
// whose layout changed since Begin fails the hash: its rows would not be those of the snapshot.
func (s *Snapshot) Hash(ctx context.Context, workers int, sink Sink) (*State, error) {
	ts, err := s.tables(ctx)
	if err != nil {
		return nil, err
	}
	state := &State{Tables: make([]Table, len(ts))}
	workers = max(1, min(workers, len(ts)))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	next := make(chan int)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range next {
				if errs[w] != nil {
					continue
				}
				if state.Tables[i], errs[w] = s.encode(ctx, ts[i], sink); errs[w] != nil {
					errs[w] = fmt.Errorf("encoding %s: %w", ts[i].name, errs[w])
					cancel()
				}
			}
		})
	}
	for i := range ts {
		select {
		case next <- i:
		case <-ctx.Done():
		}
	}
	close(next)
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.checkLayout(ctx, ts); err != nil {
		return nil, err
	}
	state.Hash = StateHash(state.Tables)
	return state, nil
}

// StateHash is the state hash of the tables of a state: of the name and the hash of each, in
// their order
func StateHash(tables []Table) []byte {
	var all bytes.Buffer
	for _, t := range tables {
		all.Write(lenPrefix(t.Name))
		all.Write(t.Hash)
	}
	return crypto.Hash(all.Bytes())
}

// checkLayout checks the tables encoded have the layout of Begin still: what changed it after
// encoding them would not have changed their rows, but the encoding cannot tell when
func (s *Snapshot) checkLayout(ctx context.Context, ts []table) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	now, err := layouts(ctx, conn)
	if err != nil {
		return fmt.Errorf("reading the layout of the tables: %w", err)
	}
	for _, t := range ts {
		if now[t.name] != s.layout[t.name] {
			return fmt.Errorf("the storage of %s changed while the snapshot was hashed", t.name)
		}
	}
	return nil
}

// encode encodes a table, in a transaction of the snapshot, and computes its hash
func (s *Snapshot) encode(ctx context.Context, t table, sink Sink) (Table, error) {
	conn, err := begin(ctx, s.db, s.id)
	if err != nil {
		return Table{}, err
	}
	defer end(conn)
	c := &chunker{buf: make([]byte, 0, chunkBytes)}
	var out io.WriteCloser
	if sink != nil {
		var err error
		if out, err = sink(t.name); err != nil {
			return Table{}, err
		}
		c.out = out
	}
	err = conn.Raw(func(dc any) error {
		if t.query == "" {
			// the entry of the schema, of no rows
			return nil
		}
		pg, ok := dc.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("a connection of %T, not of pgx", dc)
		}
		_, err := pg.Conn().PgConn().CopyTo(ctx, c, t.query)
		return err
	})
	if out != nil {
		if errClose := out.Close(); err == nil {
			err = errClose
		}
	}
	if err != nil {
		return Table{}, err
	}
	c.flush()
	res := Table{Name: t.name, Desc: t.desc, Rows: c.rows, Bytes: c.bytes, Chunks: c.chunks}
	res.Hash = TableHash(res)
	return res, nil
}

// TableHash is the hash of a table: of its description, its rows, its bytes and the hashes of its
// chunks
func TableHash(t Table) []byte {
	var b bytes.Buffer
	b.Write(lenPrefix(t.Desc))
	b.Write(binary.BigEndian.AppendUint64(nil, uint64(t.Rows)))
	b.Write(binary.BigEndian.AppendUint64(nil, uint64(t.Bytes)))
	for _, h := range t.Chunks {
		b.Write(h)
	}
	return crypto.Hash(b.Bytes())
}

func lenPrefix(s string) []byte {
	return append(binary.BigEndian.AppendUint32(nil, uint32(len(s))), s...)
}

// chunker takes the encoding of a table: it counts its rows and its bytes, hashes its chunks and
// writes it on to out
type chunker struct {
	buf         []byte
	chunks      [][]byte
	rows, bytes int64
	out         io.Writer
}

func (c *chunker) Write(p []byte) (int, error) {
	if c.out != nil {
		if _, err := c.out.Write(p); err != nil {
			return 0, err
		}
	}
	n := len(p)
	// COPY of the text format ends each row with a newline and escapes those of the values
	c.rows += int64(bytes.Count(p, []byte{'\n'}))
	c.bytes += int64(n)
	for len(p) > 0 {
		k := min(len(p), chunkBytes-len(c.buf))
		c.buf = append(c.buf, p[:k]...)
		p = p[k:]
		if len(c.buf) == chunkBytes {
			c.flush()
		}
	}
	return n, nil
}

func (c *chunker) flush() {
	if len(c.buf) > 0 {
		c.chunks = append(c.chunks, crypto.Hash(c.buf))
		c.buf = c.buf[:0]
	}
}
