/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	qb "github.com/IBAX-io/go-ibax/packages/storage/sqldb/queryBuilder"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// shapeFuzzTypes are the data types of the columns of the fuzzing of the shapes
var shapeFuzzTypes = map[string]map[string]string{
	"1_keys": {"pub": "bytea", "amount": "numeric"},
	"1_t": {"pub": "bytea", "created_at": "timestamp without time zone", "d": "date",
		"tm": "time without time zone", "id": "bigint", "name": "character varying"},
	"2_t": {"pub": "bytea", "created_at": "timestamp without time zone", "d": "date"},
}

// shapeGen makes the statements of the fuzzing: the structure of a statement of the bytes of the
// input, its values of a source seeded with the input and a salt, so that two statements of the
// same input and other salts differ only in their values
type shapeGen struct {
	data []byte
	pos  int
	rnd  *rand.Rand
}

func newShapeGen(data []byte, salt int64) *shapeGen {
	h := fnv.New64a()
	h.Write(data)
	return &shapeGen{data: data, rnd: rand.New(rand.NewSource(int64(h.Sum64()) + salt))}
}

// pick is a structural choice of n
func (g *shapeGen) pick(n int) int {
	if g.pos >= len(g.data) {
		return 0
	}
	b := g.data[g.pos]
	g.pos++
	return int(b) % n
}

var shapeFuzzValues = []func(r *rand.Rand) any{
	func(*rand.Rand) any { return "" },
	func(*rand.Rand) any { return "NULL" },
	func(*rand.Rand) any { return "timestamp 2020-01-02 03:04:05" },
	func(*rand.Rand) any { return "timestamp now" },
	func(*rand.Rand) any { return "2020-01-02" },
	func(*rand.Rand) any { return "12:30" },
	func(*rand.Rand) any { return "now" },
	func(*rand.Rand) any { return "1700000000" },
	func(*rand.Rand) any { return "0a0b" },
	func(*rand.Rand) any { return "4e554c4c" },
	func(*rand.Rand) any { return "74696d657374616d7020" },
	func(*rand.Rand) any { return "$isnull" },
	func(*rand.Rand) any { return "-1" },
	func(*rand.Rand) any { return "x\"<&> \xff" },
	func(r *rand.Rand) any { return fmt.Sprint(r.Int63n(1000) - 500) },
	func(r *rand.Rand) any { return r.Int63n(1000) - 500 },
	func(r *rand.Rand) any { return int(r.Int63n(10)) },
	func(r *rand.Rand) any { return r.Float64() * 10 },
	func(r *rand.Rand) any { return decimal.New(r.Int63n(100000)-5000, int32(r.Intn(8))-4) },
	func(r *rand.Rand) any { return r.Intn(2) == 0 },
	func(r *rand.Rand) any { return []byte{byte(r.Intn(256)), 'a'} },
	func(*rand.Rand) any { return nil },
	func(*rand.Rand) any { return map[string]any{"a": "b"} },
	func(*rand.Rand) any { return types.LoadMap(map[string]any{"k": "v"}) },
}

// value is a value of the value source
func (g *shapeGen) value() any {
	return shapeFuzzValues[g.rnd.Intn(len(shapeFuzzValues))](g.rnd)
}

var shapeFuzzWhereKeys = []string{"id", "account", "name", "Name", "created_at", "d", "tm", "pub",
	"info->a", "info->a->b", "$and", "$or", "$in", "$nin", "$eq", "$neq", "$gt", "$lt", "$lte", "$gte",
	"$like", "$begin", "$end", "$ilike", "$iend", "$ibegin", "$IN", "Ecosystem", "ecosystem", "a b",
	"now()", "", "x>y", "$Or"}

// where is a where map of depth at most depth
func (g *shapeGen) where(depth int) *types.Map {
	m := types.NewMap()
	for n := g.pick(4); n > 0; n-- {
		m.Set(shapeFuzzWhereKeys[g.pick(len(shapeFuzzWhereKeys))], g.whereValue(depth))
	}
	return m
}

func (g *shapeGen) whereValue(depth int) any {
	kind := g.pick(6)
	if depth == 0 {
		kind = 0
	}
	switch kind {
	case 1:
		return g.where(depth - 1)
	case 2:
		list := make([]any, g.pick(4))
		for i := range list {
			if g.pick(2) == 0 {
				list[i] = g.where(depth - 1)
			} else {
				list[i] = g.value()
			}
		}
		return list
	case 3:
		return []byte("ab")
	}
	return g.value()
}

var shapeFuzzFields = []string{"amount", "+amount", "-amount", "pub", "timestamp created_at", "created_at",
	"d", "tm", "info->a", "info->b", "info->a->b", "ecosystem", "id", " name", "Name", "+ amount", "x"}

// writeSample is a statement of writeRows
type writeSample struct {
	table   string
	fields  []string
	values  []any
	where   *types.Map
	eco     int64
	exists  bool
	logData map[string]string
}

func (g *shapeGen) write() writeSample {
	s := writeSample{table: []string{"1_keys", "1_t", "2_t", "2_keys", "t"}[g.pick(5)], eco: int64(g.pick(3)),
		exists: g.pick(3) != 0}
	for n := g.pick(7); n > 0; n-- {
		s.fields = append(s.fields, strings.ToLower(shapeFuzzFields[g.pick(len(shapeFuzzFields))]))
		s.values = append(s.values, g.value())
	}
	if g.pick(8) == 0 && len(s.values) > 0 {
		s.values = s.values[:len(s.values)-1]
	}
	if s.exists {
		s.where = g.where(3)
	} else if g.pick(8) == 0 {
		s.where = g.where(1)
	}
	s.logData = map[string]string{}
	for _, col := range []string{"info"} {
		s.logData[col] = []string{"", "NULL", "{}"}[g.rnd.Intn(3)]
	}
	return s
}

// cloneWhere is a copy of a where map, whose maps and lists are copied
func cloneWhere(v any) any {
	switch x := v.(type) {
	case *types.Map:
		if x == nil {
			return x
		}
		m := types.NewMap()
		for _, k := range x.Keys() {
			e, _ := x.Get(k)
			m.Set(k, cloneWhere(e))
		}
		return m
	case []any:
		l := make([]any, len(x))
		for i, e := range x {
			l[i] = cloneWhere(e)
		}
		return l
	}
	return v
}

func cloneMap(m *types.Map) *types.Map {
	if m == nil {
		return nil
	}
	return cloneWhere(m).(*types.Map)
}

// shapeFuzzLog is the logger of the builders of the fuzzing, which log the statements they refuse
var shapeFuzzLog = func() *log.Entry {
	l := log.New()
	l.SetOutput(io.Discard)
	return log.NewEntry(l)
}()

// writeResult is what the walks and the builder make of a statement of writeRows
type writeResult struct {
	keyed, ok bool
	key       string
	updKey    string
	// the builder
	err                    error
	selectText, text       string
	selectArgs, updateArgs []any
	whereExpr              string
	// the walks
	whereArgs, setArgs []any
	hasID              bool
	tableID            string
	builtID            string
	builtArgs          []any
}

func runWriteSample(s writeSample) writeResult {
	var r writeResult
	chk := sqldb.KeyTableChecker{}
	var w qb.WriteWalk
	wValues, wWhere := append([]any(nil), s.values...), cloneMap(s.where)
	r.keyed = w.Walk(chk, s.table, append([]string(nil), s.fields...), wValues, wWhere, s.eco, s.exists) &&
		!(w.IsKeyTable && interfaceTables[w.KeyName]) && (!s.exists || w.WalkWhere(wWhere))
	if r.keyed {
		r.key = string(w.Key)
		meta := w.Meta()
		r.ok = w.Decode(meta, wValues)
		if r.ok {
			if s.exists {
				r.ok = qb.CheckWhere(w.Values, qb.WhereTypes(w.Table, w.Values))
				if r.ok {
					r.setArgs, r.ok = w.SetArgs(meta, nil)
				}
				r.whereArgs = qb.AppendWhereArgs(nil, w.Values)
				key := append([]byte{'U'}, w.Key...)
				for _, col := range w.JSONColumns() {
					if qb.HasJSONValue(s.logData, col) {
						key = append(key, 1)
					} else {
						key = append(key, 0)
					}
				}
				r.updKey = string(key)
			} else {
				r.setArgs, r.ok = w.InsertArgs(meta, nil)
				r.hasID, r.tableID = w.HasID, w.TableID
			}
		}
	}
	if len(s.values) != len(s.fields) {
		// the builder indexes the values of the fields, the walk refuses the statement
		r.err = errors.New("fields and values differ")
		return r
	}
	b := &qb.SQLQueryBuilder{Table: s.table, Fields: append([]string(nil), s.fields...),
		FieldValues: append([]any(nil), s.values...), Where: cloneMap(s.where), TxEcoID: s.eco, KeyTableChkr: chk}
	b.Entry = shapeFuzzLog
	if s.exists {
		r.selectText, r.selectArgs, r.err = b.GetSelectExpr()
		if r.err != nil {
			return r
		}
		var updateExpr string
		updateExpr, r.updateArgs, r.err = b.GetSQLUpdateExpr(s.logData)
		if r.err != nil {
			return r
		}
		r.whereExpr, _, r.err = b.GetSQLWhereExpr()
		r.text = `UPDATE "` + b.Table + `" SET ` + updateExpr + " " + r.whereExpr
		return r
	}
	r.text, r.builtArgs, r.err = b.GetSQLInsertQuery(zeroID{})
	r.builtID = b.TableID()
	return r
}

// selectSample is a select of DBSelect
type selectSample struct {
	tblname       string
	columns       any
	id            int64
	order         any
	offset, limit int64
	where         *types.Map
	query         any
	group         string
	all           bool
}

var shapeFuzzOrders = []func(g *shapeGen) any{
	func(*shapeGen) any { return nil },
	func(*shapeGen) any { return "name" },
	func(*shapeGen) any { return "now()" },
	func(g *shapeGen) any { return types.LoadMap(map[string]any{"id": g.value()}) },
	func(g *shapeGen) any { return []any{"id", types.LoadMap(map[string]any{"name": g.value()}), 5} },
	func(*shapeGen) any { return int64(3) },
	func(*shapeGen) any { return "" },
	func(*shapeGen) any { return []any{} },
}

var shapeFuzzColumns = []any{nil, "", "a,b", "*", []any{"a", 1, "info->x"}, "now()", []any{"Info->a->b", "id"},
	"A, b", int64(1), []any{}}

func (g *shapeGen) selectSample() selectSample {
	s := selectSample{
		tblname: qb.GetTableName(1, []string{"1_keys", "1_t", "2_t", "1_members", "x", "keys", "t"}[g.pick(7)]),
		columns: shapeFuzzColumns[g.pick(len(shapeFuzzColumns))],
		order:   shapeFuzzOrders[g.pick(len(shapeFuzzOrders))](g),
		offset:  []int64{0, 1, -1, 5}[g.pick(4)],
		limit:   []int64{0, 1, -1, 25, 1000000, 3}[g.pick(6)],
		query:   []any{nil, "", "count(*) as n", 1}[g.pick(4)],
		group:   []string{"", "name", "name, id"}[g.pick(3)],
		all:     g.pick(4) == 0,
	}
	if g.pick(3) == 0 {
		s.id = g.rnd.Int63n(100) + 1
	}
	if g.pick(5) != 0 {
		s.where = g.where(3)
	}
	return s
}

// selectResult is what the walk and the builder make of a select of DBSelect
type selectResult struct {
	keyed, ok   bool
	key         string
	args        []any
	early, late error
	text        string
	builtArgs   []any
}

func runSelectSample(s selectSample) selectResult {
	var r selectResult
	var sw shapeWalk
	where := cloneMap(s.where)
	r.keyed = sw.selectKey(s.tblname, s.columns, s.id, s.order, s.offset, s.limit, where, s.query, s.group, s.all)
	if r.keyed {
		r.key = string(sw.sel.Key)
		r.ok = qb.CheckWhere(sw.sel.Values, qb.WhereTypes(s.tblname, sw.sel.Values))
		if s.id != 0 {
			r.args = []any{converter.Int64ToStr(s.id)}
		} else {
			r.args = qb.AppendWhereArgs(nil, sw.sel.Values)
		}
	}
	b, early, late := buildSelect(s.tblname, s.columns, s.id, s.order, s.offset, s.limit, cloneMap(s.where),
		s.query, s.group, s.all)
	r.early, r.late = early, late
	if early == nil && late == nil {
		// the text of the select but its table, which SQL quotes with the dialect of the database
		r.text = fmt.Sprintf("%q %q %q %q %d %d %d %t %q", b.sel.Columns, b.sel.Where, b.sel.Group, b.sel.Order,
			b.sel.Offset, b.sel.Limit, len(b.sel.Args), b.query, b.columns)
		r.builtArgs = b.sel.Args
	}
	return r
}

// shapeFuzzCompared counts the statements whose walks and builder were compared: updates, inserts
// and selects
var shapeFuzzCompared [3]atomic.Int64

var shapeFuzzSeen = struct {
	sync.Mutex
	texts map[string]string
}{texts: make(map[string]string)}

// sameText tells the key has the text whenever both were made
func sameText(key, text string) (string, bool) {
	shapeFuzzSeen.Lock()
	defer shapeFuzzSeen.Unlock()
	if prev, ok := shapeFuzzSeen.texts[key]; ok {
		return prev, prev == text
	}
	if len(shapeFuzzSeen.texts) > 1<<16 {
		shapeFuzzSeen.texts = make(map[string]string)
	}
	shapeFuzzSeen.texts[key] = text
	return text, true
}

// checkWrite checks a statement of writeRows: the walks bind the values the builder binds, refuse
// the values the builder refuses, and a key has one text
func checkWrite(t *testing.T, s writeSample) {
	r := runWriteSample(s)
	desc := func() string {
		return fmt.Sprintf("table %s fields %q values %#v where %s eco %d exists %t", s.table, s.fields, s.values,
			b7Fmt(s.where), s.eco, s.exists)
	}
	if len(s.values) != len(s.fields) && r.keyed {
		t.Fatalf("the walk takes fields and values that differ: %s", desc())
	}
	if errors.Is(r.err, qb.ErrDateTimeValue) && r.ok {
		t.Fatalf("the walks accept a value the builder refuses: %s", desc())
	}
	if !r.ok || r.err != nil {
		return
	}
	if s.exists {
		if !sameArgs(r.selectArgs, r.whereArgs) || !sameArgs(r.updateArgs, r.setArgs) {
			t.Fatalf("the walks bind %v %v, the builder %v %v: %s", r.whereArgs, r.setArgs, r.selectArgs, r.updateArgs, desc())
		}
		shapeFuzzCompared[0].Add(1)
		if len(r.whereExpr) == 0 {
			return
		}
		if prev, ok := sameText(r.key, r.selectText); !ok {
			t.Fatalf("the key of the select %q is the key of %q: %s", r.selectText, prev, desc())
		}
		if prev, ok := sameText(r.updKey, r.text); !ok {
			t.Fatalf("the key of the update %q is the key of %q: %s", r.text, prev, desc())
		}
		return
	}
	shapeFuzzCompared[1].Add(1)
	args := r.builtArgs
	if !r.hasID {
		args = args[:len(args)-1]
	} else if r.tableID != r.builtID {
		t.Fatalf("the walks take the id %q, the builder %q: %s", r.tableID, r.builtID, desc())
	}
	if !sameArgs(args, r.setArgs) {
		t.Fatalf("the walks bind %v, the builder %v: %s", r.setArgs, r.builtArgs, desc())
	}
	if prev, ok := sameText(r.key, r.text); !ok {
		t.Fatalf("the key of the insert %q is the key of %q: %s", r.text, prev, desc())
	}
}

// checkSelect checks a select of DBSelect as checkWrite checks a write
func checkSelect(t *testing.T, s selectSample) {
	r := runSelectSample(s)
	desc := func() string {
		return fmt.Sprintf("table %s columns %s id %d order %s offset %d limit %d where %s query %v group %q all %t",
			s.tblname, b7Fmt(s.columns), s.id, b7Fmt(s.order), s.offset, s.limit, b7Fmt(s.where), s.query, s.group, s.all)
	}
	if errors.Is(r.early, qb.ErrDateTimeValue) && r.keyed && r.ok {
		t.Fatalf("the walk accepts a value the builder refuses: %s", desc())
	}
	if !r.keyed || !r.ok || r.early != nil || r.late != nil {
		return
	}
	shapeFuzzCompared[2].Add(1)
	if !sameArgs(r.args, r.builtArgs) {
		t.Fatalf("the walk binds %v, the builder %v: %s", r.args, r.builtArgs, desc())
	}
	if prev, ok := sameText(r.key, r.text); !ok {
		t.Fatalf("the key of the select %s is the key of %s: %s", r.text, prev, desc())
	}
}

// withShapeFuzzTypes runs f with the data types of the columns of the fuzzing
func withShapeFuzzTypes(t testing.TB) {
	prev := syspar.GetTableColType()
	syspar.SetTableColType(shapeFuzzTypes)
	t.Cleanup(func() { syspar.SetTableColType(prev) })
}

// FuzzShapeKey: statements of the same key have the same text, and the walks bind the values the
// builder binds and refuse those it refuses
func FuzzShapeKey(f *testing.F) {
	withShapeFuzzTypes(f)
	for _, seed := range [][]byte{
		{}, {1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, []byte("shape key of the statements"),
		{0, 0, 2, 3, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		{1, 1, 1, 5, 2, 7, 3, 1, 4, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 3},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		for salt := int64(0); salt < 4; salt++ {
			checkWrite(t, newShapeGen(data, salt).write())
			checkSelect(t, newShapeGen(data, salt).selectSample())
		}
	})
}

// TestShapeKeyRandom runs the checks of FuzzShapeKey on random inputs
func TestShapeKeyRandom(t *testing.T) {
	withShapeFuzzTypes(t)
	r := rand.New(rand.NewSource(7))
	n := 20000
	if testing.Short() {
		n = 2000
	}
	for i := 0; i < n; i++ {
		data := make([]byte, 8+r.Intn(48))
		r.Read(data)
		for salt := int64(0); salt < 4; salt++ {
			checkWrite(t, newShapeGen(data, salt).write())
			checkSelect(t, newShapeGen(data, salt).selectSample())
		}
	}
	t.Logf("compared %d updates, %d inserts, %d selects of %d statements", shapeFuzzCompared[0].Load(),
		shapeFuzzCompared[1].Load(), shapeFuzzCompared[2].Load(), 4*n)
	if shapeFuzzCompared[0].Load() == 0 || shapeFuzzCompared[1].Load() == 0 || shapeFuzzCompared[2].Load() == 0 {
		t.Fatal("the statements of a kind were never compared")
	}
}
