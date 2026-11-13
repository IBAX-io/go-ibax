/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

// t5Calls are the calls of a phase of a scenario of TestShapeInvalidation: each phase makes the
// statements of the same shapes
func t5Calls(c *b7Call, phase int) {
	tbl := "@1" + b7hTyped
	b7hTransfer(c, 2*phase)
	c.Insert(tbl, b7m("name", "t5-"+b7Fmt(phase), "bin", "0a0b", "at", "2020-01-02 03:04:05", "day", "2020-01-02", "tm", "10:11"))
	c.UpdateExt(tbl, b7m("name", "r1"), b7m("bin", "0102", "at", "timestamp 2022-01-01 00:00:00", "day", "2023-03-03"))
	c.UpdateExt(tbl, b7m("id", int64(-2)), b7m("data->a", "1"))
	c.UpdateExt(tbl, b7m("day", b7m("$lt", "2030-01-01")), b7m("+amount", int64(1)))
	c.Select(tbl, []any{"id", "name", "bin", "at", "day", "tm"}, 0, b7m("id", int64(1)), 0, 10,
		b7m("id", b7m("$lte", int64(-1))), nil, "", false)
	c.Select(tbl, "id,bin", 0, nil, 0, 10, b7m("bin", b7m("$eq", "\\x01")), nil, "", false)
}

// t5Event is a change of the schema, the permissions or the savepoints between the phases
type t5Event struct {
	name string
	// rebuild tells the shapes of the phases before are made again in the phase after
	rebuild bool
	// ddl tells the event changes the schema, which a view leaves to PostgreSQL
	ddl    bool
	before func(tr *sqldb.DbTransaction)
	run    func(t *testing.T, c *b7Call)
}

// t5Column adds a column to the custom table as CreateColumn does past its access check: the
// statement, the permission of the column and the data types of syspar
func t5Column(t *testing.T, c *b7Call) {
	tr := c.sc.DbTransaction
	require.NoError(t, tr.AlterTableAddColumn("1_"+b7hTyped, "extra", "timestamp"))
	_, _, err := c.sc.update([]string{"columns"}, []any{`{"name": "true", "data": "true", "bin": "true", "at": "true",
		"day": "true", "tm": "true", "amount": "true", "flag": "true", "extra": "true",
		"secret": "{\"read\": \"false\", \"update\": \"true\"}", "locked": "ContractAccess(\"@1Nobody\")"}`}, "1_tables", "name", b7hTyped)
	require.NoError(t, err)
	require.NoError(t, syspar.SysTableColType(tr))
}

// t5Permission changes the permissions of the columns of the custom table as PermColumn does past
// its access check: bin is no longer read
func t5Permission(t *testing.T, c *b7Call) {
	_, _, err := c.sc.update([]string{"columns"}, []any{`{"name": "true", "data": "true",
		"bin": "{\"read\": \"false\", \"update\": \"true\"}", "at": "true", "day": "true", "tm": "true",
		"amount": "true", "flag": "true", "secret": "{\"read\": \"false\", \"update\": \"true\"}",
		"locked": "ContractAccess(\"@1Nobody\")"}`}, "1_tables", "name", b7hTyped)
	require.NoError(t, err)
}

func t5Events() []t5Event {
	return []t5Event{
		{name: "none", run: func(*testing.T, *b7Call) {}},
		{name: "new-column", rebuild: true, ddl: true, run: t5Column},
		{name: "new-table", rebuild: true, ddl: true, run: func(t *testing.T, c *b7Call) {
			// CreateTable past its access check
			require.NoError(t, sqldb.CreateTable(c.sc.DbTransaction, "1_b7_t5", "at timestamp"))
			require.NoError(t, syspar.SysTableColType(c.sc.DbTransaction))
		}},
		{name: "column-permission", run: t5Permission},
		{name: "column-types", rebuild: true, run: func(t *testing.T, c *b7Call) {
			// the day and at columns read as text, which takes any value
			types := maps.Clone(syspar.GetTableColType())
			typed := maps.Clone(types["1_"+b7hTyped])
			typed["day"], typed["at"] = "text", "text"
			types["1_"+b7hTyped] = typed
			syspar.SetTableColType(types)
		}},
		{name: "savepoint-rollback", run: func(t *testing.T, c *b7Call) {
			tr := c.sc.DbTransaction
			require.NoError(t, tr.Savepoint(`"t5"`))
			t5Permission(t, c)
			c.Select("@1"+b7hTyped, "id,bin", 0, nil, 0, 10, b7m("bin", b7m("$eq", "\\x01")), nil, "", false)
			require.NoError(t, tr.RollbackSavepoint(`"t5"`))
		}},
		{name: "savepoint-rollback-ddl", ddl: true, run: func(t *testing.T, c *b7Call) {
			tr := c.sc.DbTransaction
			require.NoError(t, tr.Savepoint(`"t5"`))
			t5Column(t, c)
			t5Calls(c, 9)
			require.NoError(t, tr.RollbackSavepoint(`"t5"`))
		}},
		{name: "unknown-savepoint-rollback", before: func(tr *sqldb.DbTransaction) {
			if err := tr.Savepoint(`"t5u"`); err != nil {
				panic(err)
			}
		}, run: func(t *testing.T, c *b7Call) {
			// a savepoint made before the view and the caches of the transaction: the rollback
			// undoes the first phase
			c.rec("rollback", c.sc.DbTransaction.RollbackSavepoint(`"t5u"`))
		}},
	}
}

// TestShapeInvalidation (T5): the statements of a shape made before a change of the schema, the
// data types of the columns, the permissions of the columns or a rollback to a savepoint are those
// of the old functions after it, and the shapes the change replaces are made again
func TestShapeInvalidation(t *testing.T) {
	e := b7Open(t)
	restore := func() {
		tr, err := sqldb.StartTransaction()
		require.NoError(t, err)
		require.NoError(t, syspar.SysTableColType(tr))
		require.NoError(t, tr.Rollback())
	}
	for _, ev := range t5Events() {
		modes := []string{"ordered", "fallback", "pg"}
		if ev.ddl {
			modes = []string{"pg"}
		}
		for _, mode := range modes {
			var builds int64
			s := b7Scenario{name: "t5-" + ev.name, before: ev.before, run: func(c *b7Call) {
				// the second phase makes the statements of the shapes of the rows the first changed
				t5Calls(c, 0)
				t5Calls(c, 1)
				ev.run(t, c)
				from := shapeBuilds.Load()
				t5Calls(c, 2)
				builds = shapeBuilds.Load() - from
			}}
			s.after = restore
			// a first run fills the caches of the node both ways of running share
			e.settled(s, mode, b7Old)
			e.settled(s, mode, b7New)
			for pass := 1; pass <= 2; pass++ {
				want, got := e.same(s, mode, b7Old, b7New)
				if got != want {
					t.Errorf("%s %s pass %d: the records differ at %s", s.name, mode, pass, b7Diff(want, got))
				}
				switch {
				case ev.rebuild && builds == 0:
					t.Errorf("%s %s: the second phase made no shape again", s.name, mode)
				case ev.name == "none" && builds != 0:
					t.Errorf("%s %s: the second phase made %d shapes of the first again", s.name, mode, builds)
				}
			}
		}
	}
	if shapeCheck && shapeHits.Load() == 0 {
		t.Fatal("no statement was made of a known shape")
	}
}
