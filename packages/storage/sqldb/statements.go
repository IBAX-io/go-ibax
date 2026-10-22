/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"sync/atomic"

	"github.com/IBAX-io/go-ibax/packages/metrics"

	"gorm.io/gorm"
)

// statementsKey keys, in the context of a database transaction, the count of its statements
type statementsKey struct{}

// registerStatementCount counts each statement gorm sends: in the metric of its operation, and in
// the count of the database transaction it runs in, if StartTransaction began it
func registerStatementCount(db *gorm.DB) error {
	count := func(op string) func(*gorm.DB) {
		c := metrics.DBStatements.WithLabelValues(op)
		return func(db *gorm.DB) {
			c.Inc()
			if db.Statement.Context == nil {
				return
			}
			if n, ok := db.Statement.Context.Value(statementsKey{}).(*atomic.Int64); ok {
				n.Add(1)
			}
		}
	}
	cb := db.Callback()
	for _, err := range []error{
		cb.Create().After("gorm:create").Register("ibax:count_create", count("create")),
		cb.Query().After("gorm:query").Register("ibax:count_query", count("query")),
		cb.Update().After("gorm:update").Register("ibax:count_update", count("update")),
		cb.Delete().After("gorm:delete").Register("ibax:count_delete", count("delete")),
		cb.Row().After("gorm:row").Register("ibax:count_row", count("row")),
		cb.Raw().After("gorm:raw").Register("ibax:count_raw", count("raw")),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

// withStatementCount is a context counting the statements of a database transaction
func withStatementCount() (context.Context, *atomic.Int64) {
	n := new(atomic.Int64)
	return context.WithValue(context.Background(), statementsKey{}, n), n
}

// Statements is the number of statements the transaction has sent; 0 for a transaction
// StartTransaction did not begin
func (tr *DbTransaction) Statements() int64 {
	if tr == nil || tr.statements == nil {
		return 0
	}
	return tr.statements.Load()
}
