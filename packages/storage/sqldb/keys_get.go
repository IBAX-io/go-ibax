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
// for a key with an id already, which gorm adds to the condition
const (
	firstKeySQL     = `SELECT * FROM "1_keys" WHERE id = $1 and ecosystem = $2 ORDER BY "1_keys"."id" LIMIT 1`
	firstKeyByIDSQL = `SELECT * FROM "1_keys" WHERE (id = $1 and ecosystem = $2) AND "1_keys"."id" = $3 ORDER BY "1_keys"."id" LIMIT 1`
)

// keyScan holds the values of a row of keys as gorm scans them: a column of text or a number that
// is null leaves its field as it is, a null key empties it
type keyScan struct {
	id, deleted, blocked           sql.NullInt64
	account, amount, maxpay, conds sql.NullString
	pub                            []byte
	other                          sql.RawBytes
	dest                           []any
}

// firstKey reads the key of an account in the ecosystem of m into m as gorm's First reads it: the
// same statement, counted the same, its columns set as gorm sets them. It tells false for no key.
func (tr *DbTransaction) firstKey(m *Key, wallet int64) (bool, error) {
	db := GetDB(tr)
	ctx := db.Statement.Context
	text := firstKeySQL
	if m.ID != 0 {
		text = firstKeyByIDSQL
	}
	queryStatements.Inc()
	followStatement(ctx, text)
	if db.Error != nil {
		return true, db.Error
	}
	var rows *sql.Rows
	var err error
	if m.ID == 0 {
		rows, err = db.Statement.ConnPool.QueryContext(ctx, text, wallet, m.ecosystem)
	} else {
		rows, err = db.Statement.ConnPool.QueryContext(ctx, text, wallet, m.ecosystem, m.ID)
	}
	if err != nil {
		return true, err
	}
	found, err := scanKey(rows, m)
	if cerr := rows.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return true, err
	}
	return found, nil
}

func scanKey(rows *sql.Rows, m *Key) (bool, error) {
	columns, err := rows.Columns()
	if err != nil {
		return false, err
	}
	if !rows.Next() {
		return false, rows.Err()
	}
	var k keyScan
	var small [12]any
	k.dest = small[:0]
	seen := 0
	for _, c := range columns {
		var d any = &k.other
		var bit int
		switch c {
		case "id":
			d, bit = &k.id, 1
		case "account":
			d, bit = &k.account, 2
		case "pub":
			d, bit = &k.pub, 4
		case "amount":
			d, bit = &k.amount, 8
		case "maxpay":
			d, bit = &k.maxpay, 16
		case "deleted":
			d, bit = &k.deleted, 32
		case "blocked":
			d, bit = &k.blocked, 64
		case "conditions":
			d, bit = &k.conds, 128
		}
		// a column named twice is set by its first
		if bit != 0 && seen&bit != 0 {
			d = &k.other
		}
		seen |= bit
		k.dest = append(k.dest, d)
	}
	if err = rows.Scan(k.dest...); err != nil {
		return true, err
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
	setInt(&m.ID, k.id)
	setString(&m.AccountID, k.account)
	if seen&4 != 0 {
		m.PublicKey = k.pub
	}
	setString(&m.Amount, k.amount)
	setString(&m.Maxpay, k.maxpay)
	setInt(&m.Deleted, k.deleted)
	setInt(&m.Blocked, k.blocked)
	setString(&m.Conditions, k.conds)
	return true, rows.Err()
}
