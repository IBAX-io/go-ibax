package script_test

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// namedSlice is assignable to []any, which a type assertion to []any does not take
type namedSlice []any

// namedString is not assignable to string
type namedString string

var (
	typeString  = reflect.TypeFor[string]()
	typeInt64   = reflect.TypeFor[int64]()
	typeUint32  = reflect.TypeFor[uint32]()
	typeFloat64 = reflect.TypeFor[float64]()
	typeBool    = reflect.TypeFor[bool]()
	typeDecimal = reflect.TypeFor[decimal.Decimal]()
	typeMap     = reflect.TypeFor[*types.Map]()
	typeSlice   = reflect.TypeFor[[]any]()
	typeBytes   = reflect.TypeFor[[]byte]()
	typeAny     = reflect.TypeFor[any]()
)

// typedValues are values of the type t, for the typed calls
func typedValues(t *testing.T, typ reflect.Type) []any {
	switch typ {
	case typeString:
		return []any{"1", "keys", "abc", "0x1f"}
	case typeInt64:
		return []any{int64(1), int64(0), int64(-3)}
	case typeUint32:
		return []any{uint32(1)}
	case typeFloat64:
		return []any{1.5, 4.0}
	case typeBool:
		return []any{true, false}
	case typeDecimal:
		return []any{decimal.NewFromInt(7), decimal.RequireFromString("2.5")}
	case typeMap:
		return []any{types.LoadMap(map[string]any{"id": int64(1)}), types.NewMap()}
	case typeSlice:
		return []any{[]any{"a", int64(1)}, []any{}}
	case typeBytes:
		return []any{[]byte("ab"), []byte{}}
	case typeAny:
		return []any{int64(7), "7", 2.5, decimal.NewFromInt(3), []any{int64(1), "b"},
			types.LoadMap(map[string]any{"a": int64(1)}), true}
	}
	t.Fatalf("no values of %s", typ)
	return nil
}

// otherValues are values the VM may pass for a parameter of the type t that are not of it
func otherValues(typ reflect.Type) []any {
	switch typ {
	case typeString:
		return []any{int64(5), namedString("x"), 1.5}
	case typeInt64:
		return []any{"5", 2.5}
	case typeUint32:
		return []any{int64(1)}
	case typeFloat64:
		return []any{int64(2)}
	case typeBool:
		return []any{"true"}
	case typeDecimal:
		return []any{"1.5"}
	case typeMap:
		return []any{map[string]any{"a": int64(1)}}
	case typeSlice:
		return []any{namedSlice{"a"}, "x"}
	case typeBytes:
		return []any{"ab"}
	case typeAny:
		return []any{namedSlice{"n"}}
	}
	return nil
}

// curated are calls of the functions a transfer of ecosystem 1 runs, with values that pass their
// checks
var curated = map[string][][]any{
	"DBSelect": {
		{"keys", "id,amount", int64(0), "id", int64(0), int64(2), types.NewMap(), "", "", false},
		{"keys", []any{"id", "amount"}, int64(0), "id desc", int64(0), int64(1),
			types.LoadMap(map[string]any{"ecosystem": int64(1)}), "", "", false},
		{"parameters", "name,value", int64(0), "id", int64(0), int64(3), types.NewMap(), "", "", false},
	},
	"DBInsert": {{"keys", types.LoadMap(map[string]any{"id": int64(-7331101), "pub": "", "amount": "1",
		"account": "0000-0000-0000-0000-7331", "ecosystem": int64(1)})}},
	"DBUpdate": {{"keys", int64(-8012325217893711050), types.LoadMap(map[string]any{"amount": "2"})}},
	"DBUpdateExt": {{"keys", types.LoadMap(map[string]any{"id": int64(-8012325217893711050)}),
		types.LoadMap(map[string]any{"amount": "2"})}},
	"DBDelete":     {{"keys", int64(-7331101)}},
	"DBCount":      {{"keys", types.LoadMap(map[string]any{"ecosystem": int64(1)})}},
	"EcosysParam":  {{"founder_account"}},
	"Money":        {{"12.5"}, {int64(3)}, {decimal.NewFromInt(4)}, {"x"}},
	"Int":          {{"42"}, {1.9}, {"x"}},
	"Str":          {{int64(5)}, {decimal.NewFromInt(4)}},
	"Len":          {{[]any{int64(1), int64(2), int64(3)}}},
	"Sprintf":      {{"%d-%s-%v", int64(1), "a", decimal.NewFromInt(2)}, {"plain"}},
	"MulDiv":       {{"10", "3", "2"}, {int64(10), int64(3), int64(0)}},
	"Div":          {{"10", "4"}},
	"GreaterThan":  {{"10", "4"}},
	"Join":         {{[]any{"a", int64(1)}, ","}},
	"Split":        {{"a,b", ","}},
	"Random":       {{int64(1), int64(100)}},
	"RandomFloat":  {{1.0, 2.0}},
	"Substr":       {{"abcdef", int64(1), int64(3)}},
	"AddressToId":  {{"0666-0819-7230-7286-0036"}},
	"IdToAddress":  {{int64(-1)}},
	"JSONDecode":   {{`{"a":[1,"b"]}`}},
	"JSONEncode":   {{types.LoadMap(map[string]any{"a": int64(1)})}},
	"HexToBytes":   {{"0a0b"}},
	"DateTime":     {{int64(1700000000)}},
	"AddDate":      {{int64(1700000000), int64(1), int64(2), int64(3)}},
	"Append":       {{[]any{int64(1)}, "x"}},
	"Replace":      {{"aXbX", "X", "-"}},
	"ContractName": {{"contract Abc {}"}},
	"Pow":          {{int64(2), int64(10)}},
	"Floor":        {{"2.7"}},
	"CreateCLB":    {{"1clb", "u", "p", int64(1)}},
}

// restricted are the functions that act on the processes or the roles of the node: they are
// called with the values the test names only
var restricted = map[string]bool{"CreateCLB": true}

// outcome is what a call did
type outcome struct {
	stack    []any
	cost     int64
	err      string
	errType  string
	rollback []*types.RollbackTx
	flush    int
}

func extendTx(t *testing.T) func() *sqldb.DbTransaction {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Log("IBAX_TEST_DSN names no database: the functions run without one")
		return func() *sqldb.DbTransaction { return nil }
	}
	pc, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	saved := sqldb.DBConn
	require.NoError(t, sqldb.GormInit(conf.DBConfig{Host: pc.Host, Port: int(pc.Port), User: pc.User, Name: pc.Database,
		Password: pc.Password, LockTimeout: 5000, IdleInTxTimeout: 5000, MaxIdleConns: 1, MaxOpenConns: 2}))
	t.Cleanup(func() {
		sqldb.GormClose()
		sqldb.DBConn = saved
	})
	return func() *sqldb.DbTransaction {
		tr, err := sqldb.StartTransaction()
		require.NoError(t, err)
		return tr
	}
}

// builtinVMs are VMs of each type, with the extended functions InitVM gives it
func builtinVMs() []*script.VM {
	smart.InitVM()
	crypto.InitHashAlgo(crypto.HashAlgo_SHA256.String())
	var vms []*script.VM
	for _, vt := range []script.VMType{script.VMType_Smart, script.VMType_CLB, script.VMType_CLBMaster} {
		vm := script.NewVM()
		vm.SetFuncCallsDB(script.GetVM().FuncCallsDB)
		vm.Extend(&script.ExtendData{
			Objects:  smart.EmbedFuncs(vt),
			AutoPars: map[string]string{`*smart.SmartContract`: `sc`, `*script.RunTime`: `rt`},
			Calls:    smart.ExtCallOf,
		})
		vms = append(vms, vm)
	}
	return vms
}

// TestExtCallCoverage: every extended function of every type of VM has a typed call
func TestExtCallCoverage(t *testing.T) {
	names := map[string]bool{}
	for _, vm := range builtinVMs() {
		covered, total, reflected := vm.ExtCallCoverage()
		require.Empty(t, reflected, "functions without a typed call: run go generate in packages/script")
		require.Equal(t, total, covered)
		for name, obj := range vm.Objects {
			if obj.Type == script.ObjectType_ExtFunc {
				names[name] = true
			}
		}
	}
	t.Logf("%d extended functions, all of them with a typed call", len(names))
}

// TestExtCallBuiltins: each extended function, called by its typed call and by reflection with
// the same arguments of its types, of other types, nil, assignable ones and missing ones of the
// VM, leaves the same stack, cost, error or panic, rollback records and flushes of the VM
func TestExtCallBuiltins(t *testing.T) {
	level := log.GetLevel()
	log.SetLevel(log.PanicLevel)
	defer log.SetLevel(level)
	newTx := extendTx(t)
	vms := builtinVMs()
	done := map[string]bool{}
	var funcs, calls, typed int
	for _, vm := range vms {
		names := make([]string, 0)
		for name, obj := range vm.Objects {
			if obj.Type == script.ObjectType_ExtFunc && !done[name] {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			done[name] = true
			funcs++
			finfo := vm.Objects[name].GetExtFuncInfo()
			require.NotNil(t, finfo.Call, name)
			for _, c := range builtinCases(t, name, finfo) {
				calls++
				run := func(useCall bool) (outcome, int64) {
					call := finfo.Call
					if !useCall {
						finfo.Call = nil
					}
					defer func() { finfo.Call = call }()
					tr := newTx()
					if tr != nil {
						defer tr.Rollback()
					}
					extend := map[string]any{}
					sc := &smart.SmartContract{
						VM:            vm,
						DbTransaction: tr,
						Key:           &sqldb.Key{},
						Rollback:      true,
						Hash:          []byte("b6-ext-call"),
						Timestamp:     1700000000,
						BlockHeader:   &types.BlockHeader{BlockId: 9999, Timestamp: 1700000000},
						TxContract:    &smart.Contract{Name: "@1Test", StackCont: []any{"@1Test"}, Extend: extend},
						TxSmart:       &types.SmartTransaction{Header: &types.Header{EcosystemID: 1, KeyID: -7331100}},
						Rand:          rand.New(rand.NewSource(1)),
						FullAccess:    c.fullAccess,
					}
					if !c.noSC {
						extend[script.Extend_sc] = sc
					}
					extend[script.Extend_ecosystem_id] = int64(1)
					extend[script.Extend_key_id] = int64(-7331100)
					before := script.ExtCallFallbacksForTest()
					stack, cost, err := script.CallExtendForTest(vm, name, extend, 1e9, c.args)
					o := outcome{stack: stack, cost: cost, rollback: sc.RollBackTx, flush: len(sc.FlushRollback)}
					if err != nil {
						o.err, o.errType = err.Error(), fmt.Sprintf("%T", err)
					}
					return o, script.ExtCallFallbacksForTest() - before
				}
				want, _ := run(false)
				got, fallbacks := run(true)
				desc := fmt.Sprintf("%s %s %#v", name, c.kind, c.args)
				require.Equal(t, want.err, got.err, desc)
				require.Equal(t, want.errType, got.errType, desc)
				require.Equal(t, want.cost, got.cost, desc)
				require.True(t, reflect.DeepEqual(want.stack, got.stack), "%s: %#v != %#v", desc, want.stack, got.stack)
				require.True(t, reflect.DeepEqual(want.rollback, got.rollback), desc)
				require.Equal(t, want.flush, got.flush, desc)
				if c.typed {
					typed++
					require.Zero(t, fallbacks, "%s: the typed call refused its arguments", desc)
				}
				if c.assignable {
					require.Equal(t, int64(1), fallbacks, "%s: the typed call took an assignable argument", desc)
				}
			}
		}
	}
	t.Logf("%d functions, %d calls, %d of them of the types of the parameters", funcs, calls, typed)
}

// builtinCase is a call of an extended function
type builtinCase struct {
	kind       string
	args       []any
	typed      bool // the arguments are of the types of the parameters
	assignable bool // an argument is assignable to its parameter, not of its type
	noSC       bool // the VM has no contract to pass
	fullAccess bool // the contract may write any table
}

func builtinCases(t *testing.T, name string, finfo *script.ExtFuncInfo) []builtinCase {
	var (
		cases  []builtinCase
		params []reflect.Type // the parameters the call passes
		hasSC  bool
	)
	fixed := len(finfo.Params)
	if finfo.Variadic {
		fixed--
	}
	for i := 0; i < fixed; i++ {
		switch finfo.Auto[i] {
		case "":
			params = append(params, finfo.Params[i])
		case script.Extend_sc:
			hasSC = true
		}
	}
	for _, args := range curated[name] {
		cases = append(cases, builtinCase{kind: "curated", args: args},
			builtinCase{kind: "curated full access", args: args, fullAccess: true})
	}
	if restricted[name] {
		return cases
	}
	tails := [][]any{nil}
	if finfo.Variadic {
		tails = [][]any{{}, {"a"}, {int64(1), "b", 2.5}, {namedSlice{"c"}, nil}}
	}
	// the values of the types of the parameters, the k-th of each
	typedArgs := func(k int) []any {
		args := make([]any, len(params))
		for i, p := range params {
			vals := typedValues(t, p)
			args[i] = vals[k%len(vals)]
		}
		return args
	}
	withTail := func(args, tail []any) []any {
		return append(append([]any{}, args...), tail...)
	}
	for k := 0; k < 3; k++ {
		for _, tail := range tails {
			cases = append(cases, builtinCase{kind: "typed", args: withTail(typedArgs(k), tail), typed: true})
		}
	}
	for i, p := range params {
		args := typedArgs(0)
		args[i] = nil
		cases = append(cases, builtinCase{kind: fmt.Sprintf("nil %d", i), args: withTail(args, tails[0]),
			typed: p == typeString || p == typeAny})
		for _, v := range otherValues(p) {
			args := typedArgs(0)
			args[i] = v
			_, named := v.(namedSlice)
			cases = append(cases, builtinCase{kind: fmt.Sprintf("other %d", i), args: withTail(args, tails[0]),
				typed: p == typeAny, assignable: named && p == typeSlice})
		}
	}
	if hasSC {
		cases = append(cases, builtinCase{kind: "no contract", args: withTail(typedArgs(0), tails[0]), noSC: true})
	}
	if !finfo.Variadic && len(params) > 0 {
		cases = append(cases, builtinCase{kind: "missing", args: typedArgs(0)[1:]})
	}
	return cases
}

// BenchmarkExtCallBuiltins are calls of extended functions a transfer runs, by their typed calls
// and by reflection
func BenchmarkExtCallBuiltins(b *testing.B) {
	level := log.GetLevel()
	log.SetLevel(log.PanicLevel)
	defer log.SetLevel(level)
	vm := builtinVMs()[0]
	sc := &smart.SmartContract{VM: vm, TxContract: &smart.Contract{Name: "@1Test", StackCont: []any{"@1Test"}},
		TxSmart: &types.SmartTransaction{Header: &types.Header{EcosystemID: 1}}}
	calls := []struct {
		name string
		args []any
	}{
		{"Len", []any{[]any{int64(1), int64(2), int64(3)}}},
		{"Int", []any{"42"}},
		{"Money", []any{"12.5"}},
		{"Str", []any{int64(5)}},
		{"Sprintf", []any{"%s-%d", "a", int64(1)}},
		{"Contains", []any{"abc", "b"}},
		{"IdToAddress", []any{int64(-1)}},
		{"ContractAccess", []any{"@1Test"}},
	}
	for _, c := range calls {
		finfo := vm.Objects[c.name].GetExtFuncInfo()
		call := finfo.Call
		for _, typed := range []bool{true, false} {
			name := c.name + "/reflect"
			if typed {
				name = c.name + "/typed"
			}
			b.Run(name, func(b *testing.B) {
				finfo.Call = nil
				if typed {
					finfo.Call = call
				}
				defer func() { finfo.Call = call }()
				run := script.ExtendCallerForTest(vm, c.name, map[string]any{script.Extend_sc: sc}, c.args)
				b.ReportAllocs()
				for b.Loop() {
					if err := run(); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
