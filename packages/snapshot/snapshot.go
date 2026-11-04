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
// downloads.
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
	db   *sql.DB
	conn *sql.Conn
	id   string
}

// Begin takes a snapshot of the database: what it has committed so far, and nothing it commits
// after Begin returns
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
	return s, nil
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
	name  string
	desc  string
	query string
}

// tables reads the tables of the state of the snapshot and how to encode each
func (s *Snapshot) tables(ctx context.Context) ([]table, error) {
	rows, err := s.conn.QueryContext(ctx, `SELECT c.oid, c.relname FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND NOT c.relispartition
		ORDER BY c.relname COLLATE "C"`)
	if err != nil {
		return nil, err
	}
	var ts []table
	for rows.Next() {
		var t table
		if err := rows.Scan(&t.oid, &t.name); err != nil {
			rows.Close()
			return nil, err
		}
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
	return ts, nil
}

// describe sets the description of a table, its columns in their order with their types, whether
// they may be null and their defaults, and its primary key, and the query of its encoding
func (s *Snapshot) describe(ctx context.Context, t *table) error {
	type column struct {
		name, typ, def string
		notNull, text  bool
	}
	rows, err := s.conn.QueryContext(ctx, `SELECT a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull,
		coalesce(pg_get_expr(d.adbin, d.adrelid), ''), a.attcollation <> 0
		FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum`, t.oid)
	if err != nil {
		return err
	}
	var cols []column
	for rows.Next() {
		var c column
		if err := rows.Scan(&c.name, &c.typ, &c.notNull, &c.def, &c.text); err != nil {
			rows.Close()
			return err
		}
		if !slices.Contains(nodeColumns[t.name], c.name) {
			cols = append(cols, c)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(cols) == 0 {
		return errors.New("a table without columns")
	}
	rows, err = s.conn.QueryContext(ctx, `SELECT a.attname FROM pg_index i
		CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, ord)
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
		WHERE i.indrelid = $1 AND i.indisprimary ORDER BY k.ord`, t.oid)
	if err != nil {
		return err
	}
	var key []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		key = append(key, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	var desc strings.Builder
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = quote(c.name)
		fmt.Fprintf(&desc, "%s %s %t %s\n", c.name, c.typ, c.notNull, c.def)
	}
	fmt.Fprintf(&desc, "key %s\n", strings.Join(key, ","))
	t.desc = desc.String()
	order := make([]string, len(key))
	for i, k := range key {
		order[i] = quote(k)
		if j := slices.IndexFunc(cols, func(c column) bool { return c.name == k }); j >= 0 && cols[j].text {
			order[i] += ` COLLATE "C"`
		}
	}
	from := "public." + quote(t.name)
	if len(order) == 0 {
		// the text of the row is that of the columns of the state alone
		order = []string{fmt.Sprintf(`(ROW(%s))::text COLLATE "C"`, strings.Join(names, ", "))}
	}
	t.query = fmt.Sprintf(`COPY (SELECT %s FROM %s ORDER BY %s) TO STDOUT`, strings.Join(names, ", "), from, strings.Join(order, ", "))
	return nil
}

func quote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Hash computes the state of the snapshot and its hash by workers, each in a transaction of its
// own of the snapshot, and gives the encoding of each table to sink
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
			conn, err := begin(ctx, s.db, s.id)
			if err != nil {
				errs[w] = err
				cancel()
				return
			}
			defer end(conn)
			for i := range next {
				if errs[w] != nil {
					continue
				}
				if state.Tables[i], errs[w] = encode(ctx, conn, ts[i], sink); errs[w] != nil {
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
	var all bytes.Buffer
	for _, t := range state.Tables {
		all.Write(lenPrefix(t.Name))
		all.Write(t.Hash)
	}
	state.Hash = crypto.Hash(all.Bytes())
	return state, nil
}

// encode encodes a table and computes its hash
func encode(ctx context.Context, conn *sql.Conn, t table, sink Sink) (Table, error) {
	c := &chunker{buf: make([]byte, 0, chunkBytes)}
	var out io.WriteCloser
	if sink != nil {
		var err error
		if out, err = sink(t.name); err != nil {
			return Table{}, err
		}
		c.out = out
	}
	err := conn.Raw(func(dc any) error {
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
