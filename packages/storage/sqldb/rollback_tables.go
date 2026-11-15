/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"sync"

	"gorm.io/gorm"
)

// rollback_tables gives the tables rollback_tx has records of their refs. It is of the state, as
// rollback_tx: a block that first has a record of a table gives the table the next ref, in the
// order of its records, and the rollback of the block deletes the refs it gave, so that the refs
// are those of the blocks on every node.

// knownRefs are the refs of rollback_tables blocks read that earlier blocks gave, which are
// committed. A rollback, which deletes refs, and an import of the state forget them
// (ResetTableRefs).
var knownRefs = struct {
	sync.RWMutex
	refs map[string]int32
}{refs: map[string]int32{}}

// ResetTableRefs forgets the refs read from rollback_tables, after its rows changed but by blocks
func ResetTableRefs() {
	knownRefs.Lock()
	knownRefs.refs = map[string]int32{}
	knownRefs.Unlock()
}

func knownRef(name string) (int32, bool) {
	knownRefs.RLock()
	defer knownRefs.RUnlock()
	ref, ok := knownRefs.refs[name]
	return ref, ok
}

// TableRefs are the refs of the tables of the records of a block, those it gave among them: an
// execution of the block has its own, as the refs it gave leave with its transaction if it fails
type TableRefs struct {
	given map[string]int32
}

// NewTableRefs returns the refs of an execution of a block
func NewTableRefs() *TableRefs {
	return &TableRefs{given: map[string]int32{}}
}

func (r *TableRefs) ref(name string) (int32, bool) {
	if ref, ok := r.given[name]; ok {
		return ref, true
	}
	return knownRef(name)
}

// set sets the refs of the tables of rts, records of one block, giving the tables rollback_tables
// does not have the next refs in the order of rts
func (r *TableRefs) set(db *gorm.DB, rts []*RollbackTx) error {
	var missing []string
	seen := map[string]bool{}
	for _, rt := range rts {
		if _, ok := r.ref(rt.NameTable); !ok && !seen[rt.NameTable] {
			seen[rt.NameTable] = true
			missing = append(missing, rt.NameTable)
		}
	}
	if len(missing) > 0 {
		if err := r.add(db, rts[0].BlockID, missing); err != nil {
			return err
		}
	}
	for _, rt := range rts {
		rt.TableRef, _ = r.ref(rt.NameTable)
	}
	return nil
}

// add finds the refs of names in rollback_tables, and gives those it does not have the next refs
// in the order of names
func (r *TableRefs) add(db *gorm.DB, blockID int64, names []string) error {
	var found []struct {
		Ref     int32
		Name    string
		BlockID int64
	}
	if err := db.Raw(`SELECT "ref", "name", "block_id" FROM "rollback_tables" WHERE "name" = ANY(?::varchar[])`,
		column(names, func(n string) string { return n })).Scan(&found).Error; err != nil {
		return err
	}
	knownRefs.Lock()
	for _, f := range found {
		// a ref of the block itself is of its transaction, which may not commit
		if f.BlockID < blockID {
			knownRefs.refs[f.Name] = f.Ref
		} else {
			r.given[f.Name] = f.Ref
		}
	}
	knownRefs.Unlock()
	var added []string
	for _, n := range names {
		if _, ok := r.ref(n); !ok {
			added = append(added, n)
		}
	}
	if len(added) == 0 {
		return nil
	}
	var last int32
	if err := db.Raw(`SELECT coalesce(max("ref"), 0) FROM "rollback_tables"`).Scan(&last).Error; err != nil {
		return err
	}
	refs := make([]int32, len(added))
	for i, n := range added {
		refs[i] = last + int32(i) + 1
		r.given[n] = refs[i]
	}
	return db.Exec(`INSERT INTO "rollback_tables" ("ref", "name", "block_id")
		SELECT r, n, ? FROM unnest(?::integer[], ?::varchar[]) AS u(r, n)`, blockID,
		column(refs, func(v int32) int32 { return v }), column(added, func(n string) string { return n })).Error
}

// deleteTableRefs deletes the refs the block gave, and forgets those read
func deleteTableRefs(db *gorm.DB, blockID int64) error {
	ResetTableRefs()
	return db.Exec(`DELETE FROM "rollback_tables" WHERE "block_id" = ?`, blockID).Error
}
