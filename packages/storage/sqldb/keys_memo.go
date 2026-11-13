/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

// KeyMemo keeps the rows of 1_keys one run of one transaction read, so that the run reads a row
// once: the check of the signature, the check of the fees and their payment read the row of the
// signer again and again. The memo keeps its rows while the generation of 1_keys in the cache of
// the transaction stays: a statement that may write the table, a rollback to a savepoint that
// takes such a statement back, or another transaction of the database makes it forget them all.
// A row of the memo is thus what a read of the database gives. A transaction without a cache, a
// key with an id, and a run that has to run again or in order read the database each time. The
// public key of a row is shared by the keys the memo sets: it is read only.
type KeyMemo struct {
	tr   *DbTransaction
	gen  uint64
	rows map[keyMemoID]keyRow
}

type keyMemoID struct{ ecosystem, wallet int64 }

// Get reads the key of the account wallet in the ecosystem of m into m, as m.Get does
func (km *KeyMemo) Get(tr *DbTransaction, m *Key, wallet int64) (bool, error) {
	return km.get(tr, m, wallet, true)
}

// GetState reads the key of the account wallet in the ecosystem of m into m without its public
// key, as m.GetState does
func (km *KeyMemo) GetState(tr *DbTransaction, m *Key, wallet int64) (bool, error) {
	return km.get(tr, m, wallet, false)
}

func (km *KeyMemo) get(tr *DbTransaction, m *Key, wallet int64, pub bool) (bool, error) {
	if km == nil || m.ID != 0 || !tr.memoizes() {
		if pub {
			return m.Get(tr, wallet)
		}
		return m.GetState(tr, wallet)
	}
	if gen := tr.cache.generation(m.TableName()); km.tr != tr || km.gen != gen || km.rows == nil {
		km.tr, km.gen = tr, gen
		if km.rows == nil {
			km.rows = make(map[keyMemoID]keyRow)
		} else {
			clear(km.rows)
		}
	}
	id := keyMemoID{m.ecosystem, wallet}
	r, ok := km.rows[id]
	// a row read without the public key, found, does not serve a read of it
	if !ok || pub && r.found && r.seen&colPub == 0 {
		text := keyStateSQL
		if pub {
			text = firstKeySQL
		}
		var err error
		if r, err = tr.readKey(text, wallet, m.ecosystem, 0); err != nil {
			setKey(&r, m, pub)
			return true, err
		}
		km.rows[id] = r
	}
	setKey(&r, m, pub)
	return r.found, nil
}

// setKey sets m to the row, without its public key unless pub
func setKey(r *keyRow, m *Key, pub bool) {
	if !pub && r.seen&colPub != 0 {
		s := *r
		s.seen &^= colPub
		r = &s
	}
	r.set(m)
}

// memoizes tells a memo of keys may answer for the transaction: it has a cache that follows its
// statements, and a read of the database would not fail for the state of the transaction
func (tr *DbTransaction) memoizes() bool {
	if tr == nil || tr.cache == nil || tr.conn == nil || tr.conn.Error != nil {
		return false
	}
	v := tr.view
	return v == nil || !v.needsOrder && !v.restart
}
