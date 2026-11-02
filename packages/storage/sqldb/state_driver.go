/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// The connections of a node go through stateConn. A statement whose context carries the view of a
// transaction on the state of a block (stateView) is the view's to run: in memory, or in
// PostgreSQL on the connection. Any other statement runs in PostgreSQL as it is.

// stateQueryer is what runs the statements of a context in place of PostgreSQL
type stateQueryer interface {
	query(ctx context.Context, c *stateConn, query string, args []driver.NamedValue) (driver.Rows, error)
	exec(ctx context.Context, c *stateConn, query string, args []driver.NamedValue) (driver.Result, error)
}

type stateKey struct{}

// withState is the context whose statements q runs
func withState(ctx context.Context, q stateQueryer) context.Context {
	return context.WithValue(ctx, stateKey{}, q)
}

func stateOf(ctx context.Context) stateQueryer {
	q, _ := ctx.Value(stateKey{}).(stateQueryer)
	return q
}

// openDB opens the database of the DSN with connections that go through stateConn
func openDB(dsn string, params map[string]string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	for k, v := range params {
		cfg.RuntimeParams[k] = v
	}
	return sql.OpenDB(stateConnector{stdlib.GetConnector(*cfg)}), nil
}

type stateConnector struct{ base driver.Connector }

func (c stateConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &stateConn{pg: conn.(*stdlib.Conn)}, nil
}

func (c stateConnector) Driver() driver.Driver { return c.base.Driver() }

// stateConn is a connection of pgx whose statements the state of a block may run. In a
// transaction it keeps the tables the statements PostgreSQL ran wrote: dirtyAll when one may have
// written any table.
type stateConn struct {
	pg       *stdlib.Conn
	inTx     bool
	dirty    map[string]bool
	dirtyAll bool
}

var (
	_ driver.QueryerContext     = (*stateConn)(nil)
	_ driver.ExecerContext      = (*stateConn)(nil)
	_ driver.ConnBeginTx        = (*stateConn)(nil)
	_ driver.ConnPrepareContext = (*stateConn)(nil)
	_ driver.Pinger             = (*stateConn)(nil)
	_ driver.SessionResetter    = (*stateConn)(nil)
	_ driver.NamedValueChecker  = (*stateConn)(nil)
)

func (c *stateConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if q := stateOf(ctx); q != nil {
		return q.query(ctx, c, query, args)
	}
	if err := c.track(ctx, query); err != nil {
		return nil, err
	}
	return c.pg.QueryContext(ctx, query, args)
}

func (c *stateConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if q := stateOf(ctx); q != nil {
		return q.exec(ctx, c, query, args)
	}
	if err := c.track(ctx, query); err != nil {
		return nil, err
	}
	return c.pg.ExecContext(ctx, query, args)
}

func (c *stateConn) Prepare(query string) (driver.Stmt, error) { return c.pg.Prepare(query) }

func (c *stateConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if stateOf(ctx) != nil {
		return nil, errors.New("a statement of the state of a block is not prepared")
	}
	return c.pg.PrepareContext(ctx, query)
}

func (c *stateConn) Close() error { return c.pg.Close() }

func (c *stateConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *stateConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.pg.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	c.endTx()
	c.inTx = true
	return stateTx{c: c, tx: tx}, nil
}

func (c *stateConn) endTx() {
	c.inTx, c.dirtyAll = false, false
	clear(c.dirty)
}

// stateTx is a transaction of a stateConn
type stateTx struct {
	c  *stateConn
	tx driver.Tx
}

func (t stateTx) Commit() error {
	defer t.c.endTx()
	return t.tx.Commit()
}

func (t stateTx) Rollback() error {
	defer t.c.endTx()
	return t.tx.Rollback()
}

// track keeps the tables a statement PostgreSQL runs in a transaction writes; a statement that may
// change the schema clears what the state knows of it
func (c *stateConn) track(ctx context.Context, query string) error {
	if c.dirtyAll && c.inTx {
		if ddl(query) {
			bumpSchema()
		}
		return nil
	}
	p, err := schema.plan(ctx, c.pg, query)
	if err != nil {
		return err
	}
	switch p.kind {
	case stSelect, stMaxID, stRead, stSavepoint, stRollbackTo, stRelease:
		return nil
	case stWrite, stInsert, stUpdate, stDelete:
		if c.inTx {
			if c.dirty == nil {
				c.dirty = make(map[string]bool)
			}
			c.dirty[p.table] = true
		}
		return nil
	}
	if ddl(query) {
		bumpSchema()
	} else if harmless(query) {
		return nil
	}
	if c.inTx {
		c.dirtyAll = true
	}
	return nil
}

// ddl tells a statement may change the schema
func ddl(query string) bool {
	toks := tokenize(query)
	if len(toks) == 0 {
		return false
	}
	switch toks[0].keyword() {
	case "create", "alter", "drop", "truncate", "comment", "grant", "revoke", "do", "call":
		return true
	}
	return false
}

// effects tells a token calls a function with side effects
func effects(toks []token) bool {
	for i, t := range toks {
		if t.kind == tIdent && sideEffects[t.text] && i+1 < len(toks) && toks[i+1].is("(") {
			return true
		}
	}
	return false
}

// harmless tells a statement the plan does not know writes no table: one that sets the session, or
// locks the rows of a select
func harmless(query string) bool {
	toks := tokenize(query)
	if len(toks) == 0 {
		return true
	}
	switch toks[0].keyword() {
	case "set", "show", "reset", "begin", "start", "commit", "end", "lock":
		return true
	case "select":
		return !effects(toks) && !slices.ContainsFunc(toks, func(t token) bool { return t.keyword() == "into" })
	}
	return false
}

func (c *stateConn) Ping(ctx context.Context) error { return c.pg.Ping(ctx) }

func (c *stateConn) ResetSession(ctx context.Context) error { return c.pg.ResetSession(ctx) }

func (c *stateConn) CheckNamedValue(v *driver.NamedValue) error { return c.pg.CheckNamedValue(v) }
