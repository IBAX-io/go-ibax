/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/metrics"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var rawStatements = metrics.DBStatements.WithLabelValues("raw")

// bindPlain is the text gorm's Exec makes of a statement with ? placeholders and values each bound
// to one: each ? with a value becomes $n, a ? past the values stays, the values past the
// placeholders are bound to none. It tells false for a statement gorm would make another text of:
// one with named arguments, a value gorm expands into a list or builds as an expression.
func bindPlain(s string, args []any) (string, bool) {
	if strings.IndexByte(s, '@') >= 0 {
		return "", false
	}
	if len(args) == 0 {
		return s, true
	}
	var b strings.Builder
	b.Grow(len(s) + 2*len(args))
	n := 0
	afterParenthesis := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '?' || n == len(args) {
			afterParenthesis = c == '('
			b.WriteByte(c)
			continue
		}
		switch args[n].(type) {
		case nil, string, bool, int, int8, int16, int32, int64, uint, uint16, uint32, uint64,
			float32, float64, time.Time:
		case []byte:
			// gorm writes the bytes of a slice after a parenthesis each as a value
			if afterParenthesis {
				return "", false
			}
		case gorm.Valuer, clause.Expression, clause.Interface:
			return "", false
		case driver.Valuer:
		default:
			return "", false
		}
		n++
		b.WriteByte('$')
		b.WriteString(strconv.Itoa(n))
	}
	for _, v := range args[n:] {
		switch v.(type) {
		case nil, string, bool, int, int8, int16, int32, int64, uint, uint16, uint32, uint64,
			float32, float64, time.Time, []byte:
		case gorm.Valuer, clause.Expression, clause.Interface:
			return "", false
		case driver.Valuer:
		default:
			return "", false
		}
	}
	return b.String(), true
}

// queryRaw runs a select in the transaction, or out of any if tr is nil, as gorm's Raw(...).Rows()
// runs it: counted, and emptying in the cache of the transaction what it may change
func (tr *DbTransaction) queryRaw(s string, args ...any) (*sql.Rows, error) {
	text, ok := bindPlain(s, args)
	if !ok {
		return GetDB(tr).Raw(s, args...).Rows()
	}
	db := GetDB(tr)
	ctx, err := db.Statement.Context, db.Error
	var rows *sql.Rows
	if err == nil {
		rows, err = db.Statement.ConnPool.QueryContext(ctx, text, args...)
	}
	rowStatements.Inc()
	followStatement(ctx, text)
	return rows, err
}

// rowRaw is queryRaw for one row, as gorm's Raw(...).Row() runs it
func (tr *DbTransaction) rowRaw(s string, args ...any) *sql.Row {
	text, ok := bindPlain(s, args)
	db := GetDB(tr)
	if !ok || db.Error != nil {
		return db.Raw(s, args...).Row()
	}
	ctx := db.Statement.Context
	row := db.Statement.ConnPool.QueryRowContext(ctx, text, args...)
	rowStatements.Inc()
	followStatement(ctx, text)
	return row
}

// followStatement counts a statement in the transaction of the context, and empties in its cache
// what the statement may change, as the callbacks of gorm do
func followStatement(ctx context.Context, text string) {
	if ctx == nil {
		return
	}
	if n, ok := ctx.Value(statementsKey{}).(*atomic.Int64); ok {
		n.Add(1)
	}
	if c, ok := ctx.Value(txCacheKey{}).(*txCache); ok && c != nil && !savepointStatement(c, text) {
		table, none, all := written(text)
		switch {
		case none:
		case all:
			c.forgetAll()
		default:
			c.forget(table)
		}
	}
}

// exec runs a statement in the transaction, or out of any if tr is nil, as gorm's Exec runs it:
// counted, and emptying in the cache of the transaction what it may change, whether it fails or not
func (tr *DbTransaction) exec(s string, args ...any) error {
	text, ok := bindPlain(s, args)
	if !ok {
		return GetDB(tr).Exec(s, args...).Error
	}
	db := GetDB(tr)
	ctx, err := db.Statement.Context, db.Error
	if err == nil {
		_, err = db.Statement.ConnPool.ExecContext(ctx, text, args...)
	}
	rawStatements.Inc()
	followStatement(ctx, text)
	return err
}
