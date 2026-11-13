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
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// oldReadKey is readKey as it read every key through database/sql
func oldReadKey(tr *DbTransaction, text string, wallet, ecosystem, id int64) (keyRow, error) {
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

// The rows of the test, of ids no node has, below the ids of the tables: a new row of another test
// takes the id it would take
const (
	mrEco   = int64(-9900010)
	mrKey   = int64(-9900001)
	mrParam = int64(-9900010)
)

// modelOf is the model a read left, nothing for a read that failed
func modelOf(m any, err error) any {
	if err != nil {
		return "-"
	}
	return m
}

// modelScript reads models of each kind and writes what each read; old reads the keys as readKey
// read them through database/sql
func modelScript(tr *DbTransaction, w *strings.Builder, old bool) {
	rec := func(name string, v ...any) {
		fmt.Fprintf(w, "%s:", name)
		for _, x := range v {
			if err, ok := x.(error); ok {
				fmt.Fprintf(w, " error(%v)", err)
				continue
			}
			fmt.Fprintf(w, " %+v", x)
		}
		w.WriteByte('\n')
	}
	for i := int64(0); i < 6; i++ {
		// a model of earlier values: a NULL leaves them
		e := &Ecosystem{Name: "pre", Info: "pre-info", FeeModeInfo: "pre-fee", EmissionAmount: "pre-em",
			TokenSymbol: "pre-sym", TokenName: "pre-name", IsValued: true, Frozen: 7, Digits: 9}
		// gorm sets the fields of the columns past a column that failed to scan from the values
		// of its pool, which a read that failed leaves to no caller: the test compares the model
		// of a read without an error
		found, err := e.firstByID(tr, mrEco+i)
		rec("ecosystem", i, found, err, modelOf(*e, err))
		var fresh Ecosystem
		found, err = fresh.firstByID(tr, mrEco+i)
		rec("ecosystem-fresh", i, found, err, modelOf(fresh, err))
		// with an id, its own or another
		for _, id := range []int64{mrEco + i, mrEco} {
			withID := &Ecosystem{ID: id, Name: "pre"}
			found, err = withID.firstByID(tr, mrEco+i)
			rec("ecosystem-id", i, id, found, err, modelOf(*withID, err))
			// Get reads an ecosystem of another id as First reads it, past the cache, which
			// the views of a block share and the transaction without a view does not
			if id != mrEco+i {
				found, err = (&Ecosystem{ID: id}).Get(tr, mrEco+i)
				rec("ecosystem-get", i, id, found, err)
			}
		}
	}
	for _, name := range []string{"p-one", "p-two", "p-three", "p-four", "nope", ""} {
		sp := &StateParameter{ecosystem: mrParam, Value: "pre", Conditions: "pre"}
		found, err := sp.firstByName(tr, name)
		rec("param", name, found, err, *sp)
		// with an id, read as gorm reads it
		// a parameter with an id is read past the cache
		for _, id := range []int64{sp.ID, mrParam, -1} {
			if id == 0 {
				continue
			}
			byID := &StateParameter{ecosystem: mrParam, ID: id}
			found, err = byID.Get(tr, name)
			rec("param-id", name, id, found, err, *byID)
		}
	}
	read := tr.readKey
	if old {
		read = func(text string, wallet, ecosystem, id int64) (keyRow, error) {
			return oldReadKey(tr, text, wallet, ecosystem, id)
		}
	}
	for i := int64(0); i < 4; i++ {
		for _, text := range []string{firstKeySQL, keyStateSQL, firstKeyByIDSQL} {
			for _, id := range []int64{mrKey - i, mrKey} {
				r, err := read(text, mrKey-i, 1, id)
				rec("key", i, text == firstKeySQL, text == keyStateSQL, id, err, r)
			}
		}
		for _, eco := range []int64{1, 2} {
			var k Key
			k.SetTablePrefix(eco)
			found, err := k.Get(tr, mrKey-i)
			rec("key-get", i, eco, found, err, k)
			var s Key
			s.SetTablePrefix(eco)
			found, err = s.GetState(tr, mrKey-i)
			rec("key-state", i, eco, found, err, s)
		}
	}
}

// TestModelReadsMatchGorm compares the reads of the models of a transaction with a view with the
// reads gorm and database/sql made of them in the view: the models, their errors, the counts of
// the statements, the metrics and the view
func TestModelReadsMatchGorm(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	ctx := context.Background()
	db, err := openDB(dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)
	// the callbacks of a node: gorm counts its statements and empties the cache of the transaction
	require.NoError(t, registerStatementCount(gdb))
	require.NoError(t, registerTxCache(gdb))
	saved := DBConn
	DBConn = gdb
	t.Cleanup(func() { DBConn = saved })

	var viaSQL, traced atomic.Int64
	StatementTrace = func(string, []any) {
		traced.Add(1)
		if viaDatabaseSQL() {
			viaSQL.Add(1)
		}
	}
	t.Cleanup(func() { StatementTrace = nil })

	// the rows are committed, for the views on connections of their own to read them
	plain := testConn(t)
	clean := func() {
		for _, q := range []string{
			fmt.Sprintf(`DELETE FROM "1_ecosystems" WHERE id BETWEEN %d AND %d`, mrEco, mrEco+9),
			fmt.Sprintf(`DELETE FROM "1_parameters" WHERE id BETWEEN %d AND %d`, mrParam, mrParam+9),
			fmt.Sprintf(`DELETE FROM "1_keys" WHERE id BETWEEN %d AND %d`, mrKey-9, mrKey),
		} {
			_, err := plain.ExecContext(ctx, q)
			require.NoError(t, err)
		}
	}
	clean()
	t.Cleanup(clean)
	for _, q := range []string{
		fmt.Sprintf(`INSERT INTO "1_ecosystems" (id, name, info, fee_mode_info, is_valued, emission_amount,
			token_symbol, token_name, type_emission, type_withdraw, control_mode, digits, frozen) VALUES
			(%[1]d, 'eco-a', '{"b": 2, "a": [1, "x"]}', '{"fee_mode_detail": {}}', 1, '[{"x": "1"}]', 'SYM', 'Token', 2, 3, 1, 12, 0),
			(%[1]d + 1, 'eco-b', NULL, NULL, 0, NULL, NULL, NULL, 0, 0, 0, 0, 1),
			(%[1]d + 2, 'eco-c', '{}', NULL, 2, '[]', '', '', -1, 1, 2, 18, 5),
			(%[1]d + 3, 'eco-d', NULL, '{}', -1, NULL, 'x', NULL, 9223372036854775807, -9223372036854775808, 0, 0, 0),
			(%[1]d + 4, 'ünïcode ✓', '" <&>"', NULL, 1, NULL, '<b>', 'tab	name', 0, 0, 0, 0, 0)`, mrEco),
		fmt.Sprintf(`INSERT INTO "1_parameters" (id, name, value, conditions, permissions, ecosystem) VALUES
			(%[1]d, 'p-one', 'v1', 'c1', NULL, %[1]d),
			(%[1]d + 1, 'p-two', '', '', '{"update": "true"}', %[1]d),
			(%[1]d + 2, 'p-three', E'line\nnext \\ "q"', 'ContractConditions("x")', '{}', %[1]d),
			(%[1]d + 3, 'p-four', '12345678901234567890', '', NULL, %[1]d)`, mrParam),
		fmt.Sprintf(`INSERT INTO "1_keys" (id, pub, amount, maxpay, multi, deleted, blocked, ecosystem, account, conditions) VALUES
			(%[1]d, '\x0102ff', 123.45, 0, 0, 0, 1, 1, 'MR-000000000000000000001', ''),
			(%[1]d - 1, '', 0, 10, 1, 1, 0, 1, 'MR-000000000000000000002', '@1Contract'),
			(%[1]d - 2, '\x00', 99999999999999999999.5, 1, 0, 0, 0, 2, 'MR-2', 'c')`, mrKey),
	} {
		_, err := plain.ExecContext(ctx, q)
		require.NoError(t, err)
	}
	tx := gdb.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { tx.Rollback() })
	st, err := NewBlockState(NewDbTransaction(tx), []string{""})
	require.NoError(t, err)
	t.Cleanup(st.End)
	pconn, err := db.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pconn.Close() })

	type metricCounts struct{ query, row, raw float64 }
	counts := func() metricCounts {
		value := func(c prometheus.Counter) float64 {
			var m dto.Metric
			require.NoError(t, c.Write(&m))
			return m.GetCounter().GetValue()
		}
		return metricCounts{value(queryStatements), value(rowStatements), value(rawStatements)}
	}
	run := func(ordered, direct bool) (string, TxStats, string, metricCounts) {
		var v *TxView
		if ordered {
			v = st.NewOrderedView(0, "")
		} else {
			v = st.NewParallelView(0, "", pconn)
		}
		vtx := v.Transaction()
		if ordered {
			require.NoError(t, vtx.Savepoint("run"))
		}
		tr := vtx
		if !direct {
			tr = &DbTransaction{conn: vtx.conn, statements: vtx.statements, cache: vtx.cache}
		}
		viaSQL.Store(0)
		traced.Store(0)
		before := counts()
		var w strings.Builder
		modelScript(tr, &w, !direct)
		after := counts()
		if direct {
			require.Zero(t, viaSQL.Load(), "statements of the view ran through database/sql")
		} else {
			require.Equal(t, traced.Load(), viaSQL.Load(), "statements ran without database/sql")
		}
		require.NotZero(t, traced.Load())
		stats := vtx.Stats()
		stats.Time = 0
		digest := v.Digest()
		_, reason := v.NeedsOrder()
		require.False(t, v.Restart(), reason)
		if ordered {
			require.NoError(t, vtx.RollbackSavepoint("run"))
		}
		return w.String(), stats, digest, metricCounts{after.query - before.query, after.row - before.row, after.raw - before.raw}
	}
	// the first run loads the tables in the block, some of them to stay in PostgreSQL: the
	// savepoints of the runs after it are in PostgreSQL then
	run(true, false)
	for _, ordered := range []bool{true, false} {
		for pass := range 2 {
			want, wantStats, wantDigest, wantCounts := run(ordered, false)
			got, gotStats, gotDigest, gotCounts := run(ordered, true)
			require.Equal(t, want, got, "ordered %v pass %d", ordered, pass)
			require.Equal(t, wantStats, gotStats, "ordered %v pass %d", ordered, pass)
			require.Equal(t, wantDigest, gotDigest, "ordered %v pass %d", ordered, pass)
			require.Equal(t, wantCounts, gotCounts, "ordered %v pass %d", ordered, pass)
			if pass == 0 {
				t.Logf("ordered %v: %d statements, %d rows read; metrics %+v", ordered, gotStats.Statements,
					gotStats.Read, gotCounts)
			}
		}
	}
	if os.Getenv("B7MODELDUMP") != "" {
		got, _, _, _ := run(false, true)
		require.NoError(t, os.WriteFile("/tmp/b7impl/model_reads.txt", []byte(got), 0o644))
	}
}

// TestModelTextsAreGorms checks the texts of the reads of the models are those gorm's First makes
func TestModelTextsAreGorms(t *testing.T) {
	gdb, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=localhost"}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)
	sp := &StateParameter{ecosystem: 3}
	q := gdb.Where("ecosystem = ? and name = ?", sp.ecosystem, "n").First(sp)
	require.Equal(t, firstParamSQL, q.Statement.SQL.String())
	e := new(Ecosystem)
	q = gdb.First(e, "id = ?", int64(4))
	require.Equal(t, firstEcosystemSQL, q.Statement.SQL.String())
	sp = &StateParameter{ecosystem: 3, ID: 5}
	q = gdb.Where("ecosystem = ? and name = ?", sp.ecosystem, "n").First(sp)
	require.Equal(t, firstParamByIDSQL, q.Statement.SQL.String())
	require.Equal(t, []any{int64(3), "n", int64(5)}, q.Statement.Vars)
	e = &Ecosystem{ID: 7}
	q = gdb.First(e, "id = ?", int64(4))
	require.Equal(t, firstEcosystemByIDSQL, q.Statement.SQL.String())
	require.Equal(t, []any{int64(4), int64(7)}, q.Statement.Vars)
}

// fixedConnector connects to a database whose every query reads the rows rows makes
type fixedConnector struct{ rows func() driver.Rows }

func (c fixedConnector) Connect(context.Context) (driver.Conn, error) { return fixedConn(c), nil }
func (c fixedConnector) Driver() driver.Driver                        { return nil }

type fixedConn fixedConnector

func (c fixedConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("no prepare") }
func (c fixedConn) Close() error                        { return nil }
func (c fixedConn) Begin() (driver.Tx, error)           { return nil, errors.New("no begin") }
func (c fixedConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return c.rows(), nil
}

// modelColumns are the columns of a table of a model, their types as the state keeps them, and the
// value of the inputs of FuzzModelScan each reads: the strings a, b, c, d, then the integers i, j
type modelColumn struct {
	name string
	oid  uint32
	typ  string
	mod  int32
	val  int
}

var (
	ecosystemColumns = []modelColumn{{"id", 20, "int8", -1, 4}, {"name", 1043, "varchar", 259, 1},
		{"info", 3802, "jsonb", -1, 2}, {"fee_mode_info", 3802, "jsonb", -1, 3}, {"is_valued", 20, "int8", -1, 5},
		{"emission_amount", 3802, "jsonb", -1, 2}, {"token_symbol", 1043, "varchar", 259, 1},
		{"token_name", 1043, "varchar", 259, 3}, {"type_emission", 20, "int8", -1, 4},
		{"type_withdraw", 20, "int8", -1, 0}, {"control_mode", 20, "int8", -1, 5}, {"digits", 20, "int8", -1, 4},
		{"frozen", 20, "int8", -1, 5}}
	parameterColumns = []modelColumn{{"id", 20, "int8", -1, 4}, {"name", 1043, "varchar", 259, 1},
		{"value", 25, "text", -1, 2}, {"conditions", 25, "text", -1, 3}, {"permissions", 3802, "jsonb", -1, 2},
		{"ecosystem", 20, "int8", -1, 0}}
)

// modelRows are n rows of the columns of the values vals, the columns of the bits of nulls NULL
func modelRows(cols []modelColumn, n int, nulls uint32, vals []string) func() *memRows {
	head := memRows{}
	for _, c := range cols {
		head.names = append(head.names, c.name)
		head.types = append(head.types, newColType(c.oid, c.typ, c.mod))
		head.mods = append(head.mods, c.mod)
	}
	var cells [][]*string
	for r := range n {
		row := make([]*string, len(cols))
		for i, c := range cols {
			if nulls>>((i+r*len(cols))%32)&1 == 0 {
				row[i] = &vals[c.val]
			}
		}
		cells = append(cells, row)
	}
	return func() *memRows {
		return &memRows{names: head.names, types: head.types, mods: head.mods, rows: cells}
	}
}

// FuzzModelScan checks the reads of the models in a view against gorm's First reading the same
// rows through database/sql: whether a row is found, the error and, without one, the model
func FuzzModelScan(f *testing.F) {
	for _, s := range [][4]string{{"1", "eco", `{"a": 1}`, "{}"}, {"2", "x", "null", `"s"`}, {"-1", "", "[]", "1.5"},
		{"0", "abc", "{", "x"}, {"9223372036854775807", "7", "", " "}, {"x", "1", "2", "3"}} {
		for _, ij := range [][2]int64{{1, 1}, {-7, 0}, {3, 2}, {math.MaxInt64, -1}, {math.MinInt64, 1}} {
			for _, n := range []uint8{0, 1, 2} {
				for _, nulls := range []uint32{0, 0xffffffff, 0x10, 0x5555, 0x200} {
					f.Add(n, nulls, ij[0], ij[1], s[0], s[1], s[2], s[3])
				}
			}
		}
	}
	var serve func() driver.Rows
	db := sql.OpenDB(fixedConnector{rows: func() driver.Rows { return serve() }})
	f.Cleanup(func() { db.Close() })
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}),
		&gorm.Config{DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Discard})
	require.NoError(f, err)
	f.Fuzz(func(t *testing.T, n uint8, nulls uint32, i, j int64, a, b, c, d string) {
		vals := []string{a, b, c, d, strconv.FormatInt(i, 10), strconv.FormatInt(j, 10)}
		cmp := func(what string, cols []modelColumn, model func() (any, func(string) any), first func(any) (bool, error)) {
			mem := modelRows(cols, int(n%3), nulls, vals)
			serve = func() driver.Rows { return decodedRows{mem()} }
			old, _ := model()
			oldFound, oldErr := first(old)
			cur, field := model()
			found, err := readModel(driverRows(mem()), field)
			if found != oldFound || fmt.Sprint(err) != fmt.Sprint(oldErr) {
				t.Fatalf("%s: found %v %v, gorm %v %v", what, found, err, oldFound, oldErr)
			}
			if err == nil && fmt.Sprintf("%+v", cur) != fmt.Sprintf("%+v", old) {
				t.Fatalf("%s: model %+v, gorm %+v", what, cur, old)
			}
		}
		cmp("ecosystem", ecosystemColumns, func() (any, func(string) any) {
			e := &Ecosystem{ID: 5, Name: "pre", IsValued: true, EmissionAmount: "pre", TokenSymbol: "pre",
				TokenName: "pre", TypeEmission: 9, TypeWithdraw: 9, Digits: 9, Info: "pre", FeeModeInfo: "pre", Frozen: 9}
			return e, e.field
		}, func(m any) (bool, error) { return isFound(gdb.First(m.(*Ecosystem), "id = ?", int64(1))) })
		cmp("parameter", parameterColumns, func() (any, func(string) any) {
			sp := &StateParameter{ecosystem: 3, ID: 5, Name: "pre", Value: "pre", Conditions: "pre"}
			return sp, sp.field
		}, func(m any) (bool, error) {
			sp := m.(*StateParameter)
			return isFound(gdb.Where("ecosystem = ? and name = ?", sp.ecosystem, "n").First(sp))
		})
	})
}
