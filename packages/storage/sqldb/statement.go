/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Statement is a statement of a fixed text that runs many times with other values. It keeps what
// is made of the text: the table it writes, the text gorm's Exec binds, and the plan of the state
// of the block, for the gen of the schema it was made of.
type Statement struct {
	Text string
	// table, none and all are what written tells of the text; follow tells the statement is not
	// on savepoints, which the cache of a transaction follows by their names
	table     string
	none, all bool
	follow    bool
	// named tells the text has a named argument, afterParen tells of each placeholder whether
	// it follows a parenthesis, as bindPlain reads them
	named      bool
	afterParen []bool
	bound      atomic.Pointer[boundText]
	plan       atomic.Pointer[statementPlan]
}

// boundText is the text bindPlain makes of a statement with n values
type boundText struct {
	n    int
	text string
}

// statementPlan is the plan of the text a statement ran, of the gen of the schema
type statementPlan struct {
	gen  uint64
	text string
	p    *stmt
}

// NewStatement is the statement of the text
func NewStatement(text string) *Statement {
	s := &Statement{Text: text, named: strings.IndexByte(text, '@') >= 0}
	s.table, s.none, s.all = written(text)
	word := strings.ToLower(leadingWord(strings.TrimSpace(text)))
	s.follow = word != "savepoint" && word != "release" && word != "rollback"
	after := false
	for i := 0; i < len(text); i++ {
		if text[i] == '?' {
			s.afterParen = append(s.afterParen, after)
			continue
		}
		after = text[i] == '('
	}
	return s
}

// boundFor is the text gorm's Exec runs of the statement with the values args, as bindPlain makes
// it; false when gorm makes another
func (s *Statement) boundFor(args []any) (string, bool) {
	if s.named {
		return "", false
	}
	for i, a := range args {
		if !plainArg(a, i < len(s.afterParen), i < len(s.afterParen) && s.afterParen[i]) {
			return "", false
		}
	}
	if b := s.bound.Load(); b != nil && b.n == len(args) {
		return b.text, true
	}
	text, ok := bindPlain(s.Text, args)
	if ok {
		s.bound.Store(&boundText{n: len(args), text: text})
	}
	return text, ok
}

// planOf is the plan of the text the statement runs, made once for each gen of the schema. A
// savepoint statement has no Statement: its plan is made each time.
func (s *Statement) planOf(ctx context.Context, c *stateConn, text string) (*stmt, error) {
	if sp := s.plan.Load(); sp != nil && sp.gen == schema.gen.Load() && sp.text == text {
		return sp.p, nil
	}
	p, gen, err := schema.planGen(ctx, c.pg, text)
	if err == nil {
		s.plan.Store(&statementPlan{gen: gen, text: text, p: p})
	}
	return p, err
}

// The statements of a transaction of a view run in the view without database/sql: on the
// connection of the view, as stateConn runs them, then counted and followed by the cache of the
// transaction as the paths through database/sql count and follow them.

// directRun runs the statement of the text with the values args in the view of the transaction,
// on its connection, as database/sql would run it there; ran is false when the transaction has no
// view, its connection is not at hand, or a value is one database/sql would convert first.
func (tr *DbTransaction) directRun(s *Statement, text string, args []any, isQuery bool) (rows driver.Rows, ran bool, err error) {
	if tr == nil || tr.view == nil || tr.conn == nil || tr.conn.Error != nil {
		return nil, false, nil
	}
	v := tr.view
	ctx := tr.conn.Statement.Context
	if ctx == nil || ctx.Err() != nil {
		return nil, false, nil
	}
	named := make([]driver.NamedValue, len(args))
	for i, a := range args {
		if _, ok := a.(sql.NamedArg); ok {
			return nil, false, nil
		}
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: a}
	}
	run := func(c *stateConn) {
		ran = true
		if StatementTrace != nil {
			traceNamed(text, named)
		}
		n := statsOf(ctx)
		var start time.Time
		if n != nil {
			start = time.Now()
		}
		var p *stmt
		if s != nil {
			p, err = s.planOf(ctx, c, text)
		} else {
			p, err = schema.plan(ctx, c.pg, text)
		}
		var res driver.Result
		if err == nil {
			rows, res, err = v.runPlan(ctx, c, p, text, named, isQuery)
		}
		if n == nil {
			return
		}
		if isQuery {
			rows, err = timeQuery(n, start, rows, err)
		} else {
			_, err = timeExec(n, start, res, err)
		}
	}
	if v.ordered {
		st := v.st
		c := st.conn
		if c == nil || !c.inTx || c.txSeq != st.connTx {
			return nil, false, nil
		}
		run(c)
	} else {
		if v.conn == nil {
			return nil, false, nil
		}
		if errRaw := v.conn.Raw(func(dc any) error {
			if c, ok := dc.(*stateConn); ok {
				run(c)
			}
			return nil
		}); errRaw != nil || !ran {
			return nil, false, nil
		}
	}
	if isQuery {
		rowStatements.Inc()
	} else {
		rawStatements.Inc()
	}
	if s != nil && s.follow {
		followWritten(ctx, s.table, s.none, s.all)
	} else {
		followStatement(ctx, text)
	}
	return rows, true, err
}

// followWritten is followStatement of a statement that is not on savepoints, of what written
// tells of it
func followWritten(ctx context.Context, table string, none, all bool) {
	if n := statsOf(ctx); n != nil {
		n.statements.Add(1)
	}
	if c, ok := ctx.Value(txCacheKey{}).(*txCache); ok && c != nil {
		switch {
		case none:
		case all:
			c.forgetAll()
		default:
			c.forget(table)
		}
	}
}

// QueryStatement runs the select of the statement with the values args, which bind its $n
// placeholders, as QueryText runs its text
func (tr *DbTransaction) QueryStatement(s *Statement, args []any) (*Rows, error) {
	return tr.queryText(s, s.Text, args)
}

// ExecStatement runs the statement with the values args, which bind its ? placeholders, as ExecSql
// runs its text
func (tr *DbTransaction) ExecStatement(s *Statement, args ...any) error {
	if text, ok := s.boundFor(args); ok {
		if _, ran, err := tr.directRun(s, text, args, false); ran {
			return err
		}
	}
	return tr.ExecSql(s.Text, args...)
}

// OneRowStatement is GetOneRowTransaction of the statement, whose ? placeholders the values args
// bind
func (tr *DbTransaction) OneRowStatement(s *Statement, args ...any) *OneRow {
	rows, err := tr.queryStatement(s, args...)
	return oneRow(s.Text, args, rows, err)
}

// idStatements are the selects of the highest id of the tables, by table
var idStatements sync.Map

// idStatement is the select of the highest id of the table
func idStatement(table string) *Statement {
	if s, ok := idStatements.Load(table); ok {
		return s.(*Statement)
	}
	s, _ := idStatements.LoadOrStore(table, NewStatement(`select id from "`+table+`" order by id desc limit 1`))
	return s.(*Statement)
}

// queryStatement is queryRaw of the statement
func (tr *DbTransaction) queryStatement(s *Statement, args ...any) (*Rows, error) {
	if text, ok := s.boundFor(args); ok {
		if drv, ran, err := tr.directRun(s, text, args, true); ran {
			if err != nil {
				return nil, err
			}
			return driverRows(drv), nil
		}
	}
	return tr.queryRaw(s.Text, args...)
}
