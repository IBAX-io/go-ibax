/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// The schemas public and parts are of the chain: what is in them is what the blocks made, the
// same on every node, and the state hash covers it, so that a node that imports a snapshot
// verifies the tables it creates as well as their rows. A table of the state has its description,
// Desc, as the description of its Table; what of the schema is not of a table, the functions and
// the views, is the description of the entry SchemaEntry, of no rows. The text of the definitions
// is that of the catalog functions of PostgreSQL, the same for the same major version, which a
// chain fixes.

// SchemaEntry is the name of the entry of the state of the schema but its tables: it sorts after
// the names of the tables, which are of letters, digits and underscores
const SchemaEntry = "~schema"

// Desc is the description of a table of the state
type Desc struct {
	Columns []Column `json:"columns"`
	// Key is the columns of the primary key, in its order
	Key []string `json:"key,omitempty"`
	// Constraints is the constraints but the primary key and the not-null ones, by name
	Constraints []Def `json:"constraints,omitempty"`
	// Indexes is the definitions of the indexes that back no constraint
	Indexes  []string `json:"indexes,omitempty"`
	Triggers []string `json:"triggers,omitempty"`
	// PartKey is the partition key of a partitioned table, and Partitions its partitions
	PartKey    string      `json:"partKey,omitempty"`
	Partitions []Partition `json:"partitions,omitempty"`
}

// Column is a column of a table of the state
type Column struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	NotNull bool   `json:"notNull,omitempty"`
	// Default is the default, or the expression of a generated column
	Default   string `json:"default,omitempty"`
	Generated bool   `json:"generated,omitempty"`
	// Collation is the collation of the column if not that of its type
	Collation string `json:"collation,omitempty"`
}

// Def is a named definition
type Def struct {
	Name string `json:"name"`
	Def  string `json:"def"`
}

// Partition is a partition of a partitioned table, with the indexes of its own
type Partition struct {
	Schema  string   `json:"schema"`
	Name    string   `json:"name"`
	Bound   string   `json:"bound"`
	Indexes []string `json:"indexes,omitempty"`
}

// Schema is the description of SchemaEntry: the functions and the views of the schema public
type Schema struct {
	Functions []string `json:"functions,omitempty"`
	Views     []Def    `json:"views,omitempty"`
}

// ParseDesc parses the description of a table
func ParseDesc(desc string) (*Desc, error) {
	d := &Desc{}
	if err := json.Unmarshal([]byte(desc), d); err != nil {
		return nil, fmt.Errorf("parsing the description: %w", err)
	}
	return d, nil
}

// ParseSchema parses the description of SchemaEntry
func ParseSchema(desc string) (*Schema, error) {
	s := &Schema{}
	if err := json.Unmarshal([]byte(desc), s); err != nil {
		return nil, fmt.Errorf("parsing the schema: %w", err)
	}
	return s, nil
}

// strings reads the first column of the rows of a query
func (s *Snapshot) strings(ctx context.Context, q string, args ...any) ([]string, error) {
	rows, err := s.conn.QueryContext(ctx, q, args...)
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

// defs reads the names and the definitions of the rows of a query
func (s *Snapshot) defs(ctx context.Context, q string, args ...any) ([]Def, error) {
	rows, err := s.conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []Def
	for rows.Next() {
		var d Def
		if err := rows.Scan(&d.Name, &d.Def); err != nil {
			return nil, err
		}
		all = append(all, d)
	}
	return all, rows.Err()
}

// ownIndexes is the definitions of the indexes of a table that back no constraint of it and are
// not partitions of an index of its parent
const ownIndexes = `SELECT pg_get_indexdef(i.indexrelid) FROM pg_index i
	WHERE i.indrelid = $1
	AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conrelid = i.indrelid AND c.conindid = i.indexrelid)
	AND NOT EXISTS (SELECT 1 FROM pg_inherits h WHERE h.inhrelid = i.indexrelid)
	ORDER BY pg_get_indexdef(i.indexrelid) COLLATE "C"`

// describe sets the description of a table and the query of its encoding: the columns of the
// rows, by the primary key, or by the text of the whole row for a table without one
func (s *Snapshot) describe(ctx context.Context, t *table) error {
	var d Desc
	// a column of a collation is one of text, which orders by the "C" collation in the key
	text := map[string]bool{}
	rows, err := s.conn.QueryContext(ctx, `SELECT a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull,
		coalesce(pg_get_expr(d.adbin, d.adrelid), ''), a.attgenerated <> '', a.attcollation <> 0,
		CASE WHEN a.attcollation NOT IN (0, ty.typcollation) THEN quote_ident(cn.nspname) || '.' || quote_ident(co.collname) ELSE '' END
		FROM pg_attribute a JOIN pg_type ty ON ty.oid = a.atttypid
		LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		LEFT JOIN pg_collation co ON co.oid = a.attcollation LEFT JOIN pg_namespace cn ON cn.oid = co.collnamespace
		WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum`, t.oid)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c Column
		var collated bool
		if err := rows.Scan(&c.Name, &c.Type, &c.NotNull, &c.Default, &c.Generated, &collated, &c.Collation); err != nil {
			rows.Close()
			return err
		}
		if !slices.Contains(nodeColumns[t.name], c.Name) {
			d.Columns = append(d.Columns, c)
			text[c.Name] = collated
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(d.Columns) == 0 {
		return errors.New("a table without columns")
	}
	if d.Key, err = s.strings(ctx, `SELECT a.attname FROM pg_index i
		CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, ord)
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
		WHERE i.indrelid = $1 AND i.indisprimary ORDER BY k.ord`, t.oid); err != nil {
		return err
	}
	if d.Constraints, err = s.defs(ctx, `SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conrelid = $1 AND contype NOT IN ('p', 'n') ORDER BY conname COLLATE "C", pg_get_constraintdef(oid) COLLATE "C"`, t.oid); err != nil {
		return err
	}
	if d.Indexes, err = s.strings(ctx, ownIndexes, t.oid); err != nil {
		return err
	}
	if d.Triggers, err = s.strings(ctx, `SELECT pg_get_triggerdef(oid) FROM pg_trigger
		WHERE tgrelid = $1 AND NOT tgisinternal AND tgparentid = 0 ORDER BY tgname COLLATE "C"`, t.oid); err != nil {
		return err
	}
	if t.kind == 'p' {
		if err := s.partitions(ctx, t, &d); err != nil {
			return err
		}
	}
	desc, err := json.Marshal(d)
	if err != nil {
		return err
	}
	t.desc = string(desc)

	names := make([]string, len(d.Columns))
	for i, c := range d.Columns {
		names[i] = quote(c.Name)
	}
	order := make([]string, len(d.Key))
	for i, k := range d.Key {
		order[i] = quote(k)
		if text[k] {
			order[i] += ` COLLATE "C"`
		}
	}
	if len(order) == 0 {
		// the text of the row is that of the columns of the state alone
		order = []string{fmt.Sprintf(`(ROW(%s))::text COLLATE "C"`, strings.Join(names, ", "))}
	}
	t.query = fmt.Sprintf(`COPY (SELECT %s FROM public.%s ORDER BY %s) TO STDOUT`,
		strings.Join(names, ", "), quote(t.name), strings.Join(order, ", "))
	return nil
}

// partitions sets the partition key and the partitions of a partitioned table, of one level
func (s *Snapshot) partitions(ctx context.Context, t *table, d *Desc) error {
	var key []string
	var err error
	if key, err = s.strings(ctx, `SELECT pg_get_partkeydef($1)`, t.oid); err != nil {
		return err
	}
	d.PartKey = key[0]
	rows, err := s.conn.QueryContext(ctx, `SELECT c.oid, c.relkind, n.nspname, c.relname, pg_get_expr(c.relpartbound, c.oid)
		FROM pg_inherits h JOIN pg_class c ON c.oid = h.inhrelid JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE h.inhparent = $1 ORDER BY n.nspname COLLATE "C", c.relname COLLATE "C"`, t.oid)
	if err != nil {
		return err
	}
	var oids []uint32
	for rows.Next() {
		var p Partition
		var oid uint32
		var kind string
		if err := rows.Scan(&oid, &kind, &p.Schema, &p.Name, &p.Bound); err != nil {
			rows.Close()
			return err
		}
		if kind != "r" {
			rows.Close()
			return fmt.Errorf("partition %s.%s is partitioned", p.Schema, p.Name)
		}
		oids = append(oids, oid)
		d.Partitions = append(d.Partitions, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for i, oid := range oids {
		if d.Partitions[i].Indexes, err = s.strings(ctx, ownIndexes, oid); err != nil {
			return err
		}
	}
	return nil
}

// describeSchema is the entry of the schema but its tables. The state is of tables, partitions,
// functions and views alone: a sequence, out of those a node table owns, a materialized view, a
// foreign table, a type or an aggregate of the schema fail the hash, as the snapshot could not
// carry it.
func (s *Snapshot) describeSchema(ctx context.Context) (table, error) {
	nodes := make([]string, 0, len(nodeTables))
	for name := range nodeTables {
		nodes = append(nodes, name)
	}
	other, err := s.strings(ctx, `SELECT c.relkind::text || ' ' || c.relname FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND (c.relkind IN ('m', 'f')
			OR c.relkind = 'S' AND NOT EXISTS (SELECT 1 FROM pg_depend d JOIN pg_class o ON o.oid = d.refobjid
				WHERE d.classid = 'pg_class'::regclass AND d.objid = c.oid AND d.refclassid = 'pg_class'::regclass
				AND d.deptype IN ('a', 'i') AND o.relname = ANY($1)))
		UNION ALL SELECT 'type ' || t.typname FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
		WHERE n.nspname = 'public' AND t.typtype IN ('c', 'd', 'e', 'r', 'm') AND t.typrelid = 0
		UNION ALL SELECT 'function ' || p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public' AND p.prokind NOT IN ('f', 'p')
		ORDER BY 1`, nodes)
	if err != nil {
		return table{}, err
	}
	if len(other) > 0 {
		return table{}, fmt.Errorf("the schema has what the state cannot carry: %s", strings.Join(other, ", "))
	}
	var sc Schema
	if sc.Functions, err = s.strings(ctx, `SELECT pg_get_functiondef(p.oid) FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public' AND NOT EXISTS (SELECT 1 FROM pg_depend d
			WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND d.deptype = 'e')
		ORDER BY p.oid::regprocedure::text COLLATE "C"`); err != nil {
		return table{}, err
	}
	if sc.Views, err = s.defs(ctx, `SELECT c.relname, pg_get_viewdef(c.oid) FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'v' ORDER BY c.relname COLLATE "C"`); err != nil {
		return table{}, err
	}
	desc, err := json.Marshal(sc)
	if err != nil {
		return table{}, err
	}
	return table{name: SchemaEntry, desc: string(desc)}, nil
}
