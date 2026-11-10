/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package snapshot

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/stdlib"
)

// Import makes the state of the database that of the snapshot of a height of the store, of the
// state hash: the schema of the state, its tables, partitions, functions and views, as the
// manifest describes them, and the rows of the files, in one transaction, then hashes what it
// made and checks it is of the state hash. The tables of the node are as they were, and the node
// columns of the tables of the state as the database made them, of their default. What is of
// the state in the database before is dropped: the tables, the functions and the views of the
// schema public out of the node tables, with their partitions.
//
// A failed check leaves the state imported: the caller imports again, of another snapshot or
// peers.
func Import(ctx context.Context, db *sql.DB, store Store, height int64, hash []byte, workers int) error {
	manifest, err := store.Manifest(height)
	if err != nil {
		return err
	}
	if manifest == nil {
		return fmt.Errorf("the store has no snapshot %d", height)
	}
	state, err := ParseManifest(manifest, hash)
	if err != nil {
		return err
	}
	if err := importState(ctx, db, store, height, state); err != nil {
		return fmt.Errorf("importing snapshot %d: %w", height, err)
	}
	s, err := Begin(ctx, db)
	if err != nil {
		return err
	}
	defer s.Close()
	got, err := s.Hash(ctx, workers, nil)
	if err != nil {
		return fmt.Errorf("hashing snapshot %d imported: %w", height, err)
	}
	if !bytes.Equal(got.Hash, hash) {
		return fmt.Errorf("snapshot %d imported: %s", height, diff(state, got))
	}
	return nil
}

// diff names what differs of two states
func diff(want, got *State) string {
	tables := map[string]Table{}
	for _, t := range got.Tables {
		tables[t.Name] = t
	}
	var out []string
	for _, w := range want.Tables {
		g, ok := tables[w.Name]
		switch {
		case !ok:
			out = append(out, w.Name+" missing")
		case g.Desc != w.Desc:
			out = append(out, w.Name+" of another schema")
		case !bytes.Equal(g.Hash, w.Hash):
			out = append(out, w.Name+" of other rows")
		}
		delete(tables, w.Name)
	}
	for name := range tables {
		out = append(out, name+" more")
	}
	return "the state hash differs: " + strings.Join(out, ", ")
}

// importState makes the schema and the rows of the state in a transaction
func importState(ctx context.Context, db *sql.DB, store Store, height int64, state *State) error {
	var schema *Schema
	descs := map[string]*Desc{}
	for _, t := range state.Tables {
		var err error
		if t.Name == SchemaEntry {
			schema, err = ParseSchema(t.Desc)
		} else {
			descs[t.Name], err = ParseDesc(t.Desc)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", t.Name, err)
		}
	}
	if schema == nil {
		return fmt.Errorf("the manifest has no %s", SchemaEntry)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	run := func(q string, args ...any) error {
		if _, err := conn.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("%s: %w", firstLine(q), err)
		}
		return nil
	}
	if err := run(`BEGIN`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	// the settings the rows were encoded under, which they parse under
	for _, q := range session {
		if err := run(q); err != nil {
			return err
		}
	}
	own, err := nodeColumnDefs(ctx, conn, descs)
	if err != nil {
		return err
	}
	if err := dropState(ctx, conn, run); err != nil {
		return err
	}
	for _, f := range schema.Functions {
		if err := run(f); err != nil {
			return err
		}
	}
	for _, t := range state.Tables {
		if d := descs[t.Name]; d != nil {
			if err := createTable(t.Name, d, own[t.Name], run); err != nil {
				return err
			}
		}
	}
	for _, t := range state.Tables {
		if d := descs[t.Name]; d != nil {
			if err := copyTable(ctx, conn, store, height, t.Name, d); err != nil {
				return fmt.Errorf("copying %s: %w", t.Name, err)
			}
		}
	}
	// what checks or derives of the rows comes after them: the constraints, the indexes and the
	// triggers, which would fire on the rows copied
	for _, t := range state.Tables {
		d := descs[t.Name]
		if d == nil {
			continue
		}
		for _, c := range d.Constraints {
			if err := run(fmt.Sprintf(`ALTER TABLE public.%s ADD CONSTRAINT %s %s`, quote(t.Name), quote(c.Name), c.Def)); err != nil {
				return err
			}
		}
		for _, ix := range d.Indexes {
			// the index of a partitioned table, on its partitions too
			if err := run(strings.Replace(ix, " ON ONLY ", " ON ", 1)); err != nil {
				return err
			}
		}
		for _, p := range d.Partitions {
			for _, ix := range p.Indexes {
				if err := run(ix); err != nil {
					return err
				}
			}
		}
		for _, tr := range d.Triggers {
			if err := run(tr); err != nil {
				return err
			}
		}
	}
	for _, v := range schema.Views {
		if err := run(fmt.Sprintf(`CREATE VIEW public.%s AS %s`, quote(v.Name), v.Def)); err != nil {
			return err
		}
	}
	if err := run(`COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}

func firstLine(q string) string {
	q, _, _ = strings.Cut(strings.TrimSpace(q), "\n")
	return q
}

// dropState drops the views, the tables but the node tables and the functions of the schema public
func dropState(ctx context.Context, conn *sql.Conn, run func(string, ...any) error) error {
	list := func(q string) ([]string, error) {
		rows, err := conn.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var all []string
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				return nil, err
			}
			all = append(all, v)
		}
		return all, rows.Err()
	}
	views, err := list(`SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'v'`)
	if err != nil {
		return err
	}
	for _, v := range views {
		if err := run(`DROP VIEW IF EXISTS public.` + quote(v) + ` CASCADE`); err != nil {
			return err
		}
	}
	tables, err := list(`SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND NOT c.relispartition`)
	if err != nil {
		return err
	}
	for _, t := range tables {
		if !nodeTables[t] {
			if err := run(`DROP TABLE IF EXISTS public.` + quote(t) + ` CASCADE`); err != nil {
				return err
			}
		}
	}
	funcs, err := list(`SELECT p.oid::regprocedure::text FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public' AND NOT EXISTS (SELECT 1 FROM pg_depend d
			WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND d.deptype = 'e')`)
	if err != nil {
		return err
	}
	for _, f := range funcs {
		// a regprocedure names the function with its schema when out of the search path alone
		if err := run(`DROP FUNCTION public.` + strings.TrimPrefix(f, "public.")); err != nil {
			return err
		}
	}
	return nil
}

// nodeColumnDefs reads the definitions of the node columns of the tables of the state from the
// database before the import drops the tables: the import makes them again as the node made them,
// since the snapshot carries neither them nor their values, and the indexes of the state may be
// on them. A made column takes its default.
func nodeColumnDefs(ctx context.Context, conn *sql.Conn, descs map[string]*Desc) (map[string][]Column, error) {
	own := map[string][]Column{}
	for name, cols := range nodeColumns {
		if descs[name] == nil {
			continue
		}
		for _, col := range cols {
			c := Column{Name: col}
			err := conn.QueryRowContext(ctx, `SELECT format_type(a.atttypid, a.atttypmod), a.attnotnull,
				coalesce(pg_get_expr(d.adbin, d.adrelid), ''),
				CASE WHEN a.attcollation NOT IN (0, ty.typcollation) THEN quote_ident(cn.nspname) || '.' || quote_ident(co.collname) ELSE '' END
				FROM pg_attribute a JOIN pg_type ty ON ty.oid = a.atttypid
				LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
				LEFT JOIN pg_collation co ON co.oid = a.attcollation LEFT JOIN pg_namespace cn ON cn.oid = co.collnamespace
				WHERE a.attrelid = to_regclass('public.' || quote_ident($1)) AND a.attname = $2 AND NOT a.attisdropped`,
				name, col).Scan(&c.Type, &c.NotNull, &c.Default, &c.Collation)
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("the database has no column %s of %s, which the node makes", col, name)
			}
			if err != nil {
				return nil, err
			}
			own[name] = append(own[name], c)
		}
	}
	return own, nil
}

// createTable creates a table of the state, with the node columns of it, and its partitions
func createTable(name string, d *Desc, own []Column, run func(string, ...any) error) error {
	defs := make([]string, 0, len(d.Columns)+len(own)+1)
	for _, c := range slices.Concat(d.Columns, own) {
		def := quote(c.Name) + " " + c.Type
		if c.Collation != "" {
			def += " COLLATE " + c.Collation
		}
		if c.Default != "" {
			def += " DEFAULT " + c.Default
		}
		if c.NotNull {
			def += " NOT NULL"
		}
		defs = append(defs, def)
	}
	if len(d.Key) > 0 {
		key := make([]string, len(d.Key))
		for i, k := range d.Key {
			key[i] = quote(k)
		}
		defs = append(defs, "PRIMARY KEY ("+strings.Join(key, ", ")+")")
	}
	q := fmt.Sprintf("CREATE TABLE public.%s (\n%s\n)", quote(name), strings.Join(defs, ",\n"))
	if d.PartKey != "" {
		q += " PARTITION BY " + d.PartKey
	}
	if err := run(q); err != nil {
		return err
	}
	for _, p := range d.Partitions {
		if err := run(fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, quote(p.Schema))); err != nil {
			return err
		}
		if err := run(fmt.Sprintf(`CREATE TABLE %s.%s PARTITION OF public.%s %s`,
			quote(p.Schema), quote(p.Name), quote(name), p.Bound)); err != nil {
			return err
		}
	}
	return nil
}

// copyTable copies the rows of the file of a table of the snapshot into it
func copyTable(ctx context.Context, conn *sql.Conn, store Store, height int64, name string, d *Desc) error {
	f, err := os.Open(store.path(height, name+tableSuffix))
	if err != nil {
		return err
	}
	defer f.Close()
	cols := make([]string, len(d.Columns))
	for i, c := range d.Columns {
		cols[i] = quote(c.Name)
	}
	q := fmt.Sprintf(`COPY public.%s (%s) FROM STDIN`, quote(name), strings.Join(cols, ", "))
	return conn.Raw(func(dc any) error {
		pg, ok := dc.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("a connection of %T, not of pgx", dc)
		}
		_, err := pg.Conn().PgConn().CopyFrom(ctx, f, q)
		return err
	})
}
