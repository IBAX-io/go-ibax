/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

// TableMemo keeps the rows of 1_tables and their permissions one run of one transaction read, so
// that the checks of access of each DBSelect, DBInsert, DBUpdate and DBDelete do not read the
// row again: a select read it three times, for the table being custom, its permissions and the
// permissions of its columns. A memo answers what Table.Get and Table.GetPermissions answer, which
// it reads them through on a miss, and so through the cache of the transaction and the reads the
// view of a parallel run records. It keeps its rows while the generation of 1_tables in the cache
// of the transaction stays: a statement that may write the table, a rollback to a savepoint that
// takes such a statement back, or a rollback to a savepoint the cache does not know makes it forget
// them all. A row of the memo is thus what the cache of the transaction would answer, which is
// what a read of the database gives. A transaction without a cache, a table with an id, and a run
// that has to run again or in order read as Table.Get and Table.GetPermissions read.
type TableMemo struct {
	tr   *DbTransaction
	gen  uint64
	rows map[tableMemoID]*tableMemoRow
}

type tableMemoID struct {
	ecosystem int64
	name      string
}

// tableMemoRow is what the memo knows of a table: its row once read, its permissions once read
type tableMemoRow struct {
	read, found, permsRead bool
	table                  Table
	perms                  map[string]string
}

// row is the entry of the table of the ecosystem, nil when the memo may not answer for tr
func (tm *TableMemo) row(tr *DbTransaction, ecosystem int64, name string) *tableMemoRow {
	if tm == nil || !tr.memoizes() {
		return nil
	}
	if gen := tr.cache.generation(tablesTable); tm.tr != tr || tm.gen != gen || tm.rows == nil {
		tm.tr, tm.gen = tr, gen
		if tm.rows == nil {
			tm.rows = make(map[tableMemoID]*tableMemoRow)
		} else {
			clear(tm.rows)
		}
	}
	id := tableMemoID{ecosystem, name}
	r := tm.rows[id]
	if r == nil {
		r = &tableMemoRow{}
		tm.rows[id] = r
	}
	return r
}

// tablesTable is the table of the rows the memo keeps
const tablesTable = `1_tables`

// Get reads the row of the table name of the ecosystem of t into t, as t.Get does
func (tm *TableMemo) Get(tr *DbTransaction, t *Table, name string) (bool, error) {
	if t.ID != 0 {
		return t.Get(tr, name)
	}
	r := tm.row(tr, t.Ecosystem, name)
	if r == nil {
		return t.Get(tr, name)
	}
	if !r.read {
		found, err := t.Get(tr, name)
		if err != nil {
			return found, err
		}
		r.read, r.found = true, found
		if found {
			r.table = *t
		}
		return found, nil
	}
	if r.found {
		*t = r.table
	}
	return r.found, nil
}

// Permissions are the permissions of the table name of the ecosystem of t, as
// t.GetPermissions(tr, name, "") gives them. The caller does not change the map.
func (tm *TableMemo) Permissions(tr *DbTransaction, t *Table, name string) (map[string]string, error) {
	r := tm.row(tr, t.Ecosystem, name)
	if r == nil {
		return t.GetPermissions(tr, name, "")
	}
	if !r.permsRead {
		perms, err := t.GetPermissions(tr, name, "")
		if err != nil {
			return perms, err
		}
		r.permsRead, r.perms = true, perms
	}
	return r.perms, nil
}
