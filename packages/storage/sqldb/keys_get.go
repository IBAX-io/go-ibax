/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql"

	"github.com/IBAX-io/go-ibax/packages/metrics"
)

var queryStatements = metrics.DBStatements.WithLabelValues("query")

// firstKeySQL is the text gorm's First makes of the key of an account; firstKeyByIDSQL the text
// for a key with an id already, which gorm adds to the condition. keyStateSQL reads the key without
// its public key, for the reads that do not use it.
const (
	firstKeySQL     = `SELECT * FROM "1_keys" WHERE id = $1 and ecosystem = $2 ORDER BY "1_keys"."id" LIMIT 1`
	firstKeyByIDSQL = `SELECT * FROM "1_keys" WHERE (id = $1 and ecosystem = $2) AND "1_keys"."id" = $3 ORDER BY "1_keys"."id" LIMIT 1`
	keyStateSQL     = `SELECT id,account,amount,maxpay,deleted,blocked,conditions FROM "1_keys" WHERE id = $1 and ecosystem = $2 ORDER BY "1_keys"."id" LIMIT 1`
)

// The columns of a row of keys a read gives: a bit of keyRow.seen each
const (
	colID = 1 << iota
	colAccount
	colPub
	colAmount
	colMaxpay
	colDeleted
	colBlocked
	colConditions
)

// keyRow is a row of keys as a read gives it: a column of text or a number that is null leaves
// the field of the key as it is, as gorm scans it, a null key empties it
type keyRow struct {
	found                          bool
	seen                           int
	id, deleted, blocked           sql.NullInt64
	account, amount, maxpay, conds sql.NullString
	pub                            []byte
}

// set sets the key m to the row, as gorm sets the columns it read; it leaves m as it is for no row
func (r *keyRow) set(m *Key) {
	if !r.found {
		return
	}
	setInt := func(f *int64, v sql.NullInt64) {
		if v.Valid {
			*f = v.Int64
		}
	}
	setString := func(f *string, v sql.NullString) {
		if v.Valid {
			*f = v.String
		}
	}
	setInt(&m.ID, r.id)
	setString(&m.AccountID, r.account)
	if r.seen&colPub != 0 {
		m.PublicKey = r.pub
	}
	setString(&m.Amount, r.amount)
	setString(&m.Maxpay, r.maxpay)
	setInt(&m.Deleted, r.deleted)
	setInt(&m.Blocked, r.blocked)
	setString(&m.Conditions, r.conds)
}

// firstKey reads the key of an account in the ecosystem of m into m as gorm's First reads it: the
// same statement, counted the same, its columns set as gorm sets them. It tells false for no key.
func (tr *DbTransaction) firstKey(m *Key, wallet int64) (bool, error) {
	text := firstKeySQL
	if m.ID != 0 {
		text = firstKeyByIDSQL
	}
	r, err := tr.readKey(text, wallet, m.ecosystem, m.ID)
	r.set(m)
	if err != nil {
		return true, err
	}
	return r.found, nil
}

// readKey reads the row of keys of the statement text, of the account wallet in the ecosystem and,
// for firstKeyByIDSQL, with the id. A row that the statement gave stays in the result with an error
// of the rows after it.
func (tr *DbTransaction) readKey(text string, wallet, ecosystem, id int64) (keyRow, error) {
	db := GetDB(tr)
	ctx := db.Statement.Context
	queryStatements.Inc()
	followStatement(ctx, text)
	if db.Error != nil {
		return keyRow{}, db.Error
	}
	var rows *sql.Rows
	var err error
	if text == firstKeyByIDSQL {
		rows, err = db.Statement.ConnPool.QueryContext(ctx, text, wallet, ecosystem, id)
	} else {
		rows, err = db.Statement.ConnPool.QueryContext(ctx, text, wallet, ecosystem)
	}
	if err != nil {
		return keyRow{}, err
	}
	r, err := scanKey(rows)
	if cerr := rows.Close(); err == nil {
		err = cerr
	}
	return r, err
}

func scanKey(rows *sql.Rows) (keyRow, error) {
	var r keyRow
	columns, err := rows.Columns()
	if err != nil {
		return r, err
	}
	if !rows.Next() {
		return r, rows.Err()
	}
	var other sql.RawBytes
	var small [12]any
	dest := small[:0]
	for _, c := range columns {
		var d any = &other
		var bit int
		switch c {
		case "id":
			d, bit = &r.id, colID
		case "account":
			d, bit = &r.account, colAccount
		case "pub":
			d, bit = &r.pub, colPub
		case "amount":
			d, bit = &r.amount, colAmount
		case "maxpay":
			d, bit = &r.maxpay, colMaxpay
		case "deleted":
			d, bit = &r.deleted, colDeleted
		case "blocked":
			d, bit = &r.blocked, colBlocked
		case "conditions":
			d, bit = &r.conds, colConditions
		}
		// a column named twice is set by its first
		if bit != 0 && r.seen&bit != 0 {
			d = &other
		}
		r.seen |= bit
		dest = append(dest, d)
	}
	if err = rows.Scan(dest...); err != nil {
		return keyRow{}, err
	}
	r.found = true
	return r, rows.Err()
}
