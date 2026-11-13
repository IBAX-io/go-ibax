package script

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
)

// costInt is a named type of a cost
type costInt int64

// extStacker records the calls of the stack of a contract
type extStacker struct {
	log  []string
	fail string
}

func (s *extStacker) AppendStack(fn string) error {
	s.log = append(s.log, "+"+fn)
	if fn == s.fail {
		return fmt.Errorf("no %s", fn)
	}
	return nil
}

func (s *extStacker) PopStack(fn string) { s.log = append(s.log, "-"+fn) }

// extFakes are extended functions of the parameters and results the VM treats each its way
var extFakes = map[string]any{
	// a function querying the database, its first result the cost
	"FakeDB": func(rt *RunTime, table string, cost int64) (int64, string, error) {
		if table == "fail" {
			return cost, "", errors.New("fake failure")
		}
		return cost, table + "!", nil
	},
	"FakeDBNamed": func(rt *RunTime, table string, cost int64) (costInt, string, error) {
		return costInt(cost), table, nil
	},
	"FakeDBAny": func(rt *RunTime, table string, cost int64) (any, string, error) {
		return cost, table, nil
	},
	"FakeVariadic": func(rt *RunTime, a, b string, va ...any) (any, error) {
		if a == "panic" {
			panic("fake panic")
		}
		return fmt.Sprintf("%s|%s|%d|%v|%T", a, b, len(va), va, va), nil
	},
	"FakeAny": func(v any) string {
		return fmt.Sprintf("%#v", v)
	},
	"FakeSlice": func(v []any) int64 {
		return int64(len(v))
	},
	"FakeMap": func(m *types.Map, s string) (string, error) {
		if m == nil {
			return "", errors.New("nil map")
		}
		return s + fmt.Sprint(m.Size()), nil
	},
	"FakeNoResult": func(rt *RunTime, a any, va ...any) {
		rt.extend["fake_noresult"] = fmt.Sprint(a, va)
	},
}

var extFakeCalls = map[string]ExtCall{
	"FakeDB":       ExtCall3R3(extFakes["FakeDB"].(func(*RunTime, string, int64) (int64, string, error))),
	"FakeDBNamed":  ExtCall3R3(extFakes["FakeDBNamed"].(func(*RunTime, string, int64) (costInt, string, error))),
	"FakeDBAny":    ExtCall3R3(extFakes["FakeDBAny"].(func(*RunTime, string, int64) (any, string, error))),
	"FakeVariadic": ExtCall3VR2(extFakes["FakeVariadic"].(func(*RunTime, string, string, ...any) (any, error))),
	"FakeAny":      ExtCall1R1(extFakes["FakeAny"].(func(any) string)),
	"FakeSlice":    ExtCall1R1(extFakes["FakeSlice"].(func([]any) int64)),
	"FakeMap":      ExtCall2R2(extFakes["FakeMap"].(func(*types.Map, string) (string, error))),
	"FakeNoResult": ExtCall2VR0(extFakes["FakeNoResult"].(func(*RunTime, any, ...any))),
}

func extFakeVM() *VM {
	vm := NewVM()
	vm.SetFuncCallsDB(map[string]struct{}{"FakeDB": {}, "FakeDBNamed": {}, "FakeDBAny": {}})
	vm.Extend(&ExtendData{
		Objects:  extFakes,
		AutoPars: map[string]string{`*script.RunTime`: `rt`},
		Calls: func(fn any) ExtCall {
			for name, f := range extFakes {
				if reflect.ValueOf(f).Pointer() == reflect.ValueOf(fn).Pointer() {
					return extFakeCalls[name]
				}
			}
			return nil
		},
	})
	return vm
}

// extCase is a call of an extended function with the values of the stack before it
type extCase struct {
	name   string
	args   []any
	unwrap []any // an array the call spreads into its last arguments
	cost   int64
	stack  *extStacker
	// count is the count of arguments the stack holds for a variadic call, len(args) when 0
	count int
}

type extOutcome struct {
	stack   []any
	cost    int64
	err     string
	errInfo ErrInfo
	log     []string
	extend  any
}

func runExtCase(t *testing.T, vm *VM, c extCase, typed bool) extOutcome {
	obj := vm.Objects[c.name]
	finfo := obj.GetExtFuncInfo()
	call := finfo.Call
	require.NotNil(t, call, c.name)
	if !typed {
		finfo.Call = nil
	}
	defer func() { finfo.Call = call }()
	cost := c.cost
	if cost == 0 {
		cost = 1000
	}
	rt := NewRunTime(vm, cost)
	rt.extend = map[string]any{}
	var stacker *extStacker
	if c.stack != nil {
		stacker = &extStacker{fail: c.stack.fail}
		rt.extend[Extend_sc] = stacker
	}
	rt.push("below")
	for _, arg := range c.args {
		rt.push(arg)
	}
	cmd := uint16(cmdCall)
	if finfo.Variadic {
		cmd = cmdCallVariadic
		count := c.count
		if count == 0 {
			count = len(c.args)
		}
		if c.unwrap != nil {
			rt.push(c.unwrap)
			count++
			rt.unwrap = true
		}
		rt.push(count)
	}
	var out extOutcome
	func() {
		defer func() {
			if r := recover(); r != nil {
				out.err = fmt.Sprintf("panic: %v", r)
			}
		}()
		if err := rt.callFunc(cmd, obj); err != nil {
			out.err = err.Error()
		}
	}()
	out.stack = append([]any(nil), rt.stack...)
	out.cost = rt.cost
	out.errInfo = rt.errInfo
	out.extend = rt.extend["fake_noresult"]
	if stacker != nil {
		out.log = stacker.log
	}
	if !strings.HasPrefix(out.err, "panic: ") {
		require.Empty(t, rt.extArgs, "the arguments of the call stay in the buffer")
	}
	return out
}

// TestExtCallMechanics: the typed call of an extended function and the call by reflection leave
// the same stack, cost, error, panic and calls of the stack of the contract, for the costs of
// functions querying the database, error results, nil arguments, arguments of other types or
// assignable ones, variadic calls without, with one or more values or a spread array, and too
// few values on the stack
func TestExtCallMechanics(t *testing.T) {
	vm := extFakeVM()
	m := types.LoadMap(map[string]any{"a": int64(1)})
	var nilMap *types.Map
	cases := []extCase{
		{name: "FakeDB", args: []any{"t", int64(7)}},
		{name: "FakeDB", args: []any{"t", int64(-7)}},
		{name: "FakeDB", args: []any{"t", int64(7)}, cost: 5},
		{name: "FakeDB", args: []any{"fail", int64(1)}},
		{name: "FakeDB", args: []any{nil, int64(1)}},
		{name: "FakeDB", args: []any{"t", nil}},
		{name: "FakeDB", args: []any{"t", 1.5}},
		{name: "FakeDB", args: []any{int64(1), int64(1)}},
		{name: "FakeDB", args: []any{"t", int64(1)}, stack: &extStacker{}},
		{name: "FakeDB", args: []any{"t", int64(1)}, stack: &extStacker{fail: "FakeDB"}},
		{name: "FakeDB", args: []any{int64(1)}},
		{name: "FakeDBNamed", args: []any{"t", int64(3)}},
		{name: "FakeDBAny", args: []any{"t", int64(3)}},
		{name: "FakeVariadic", args: []any{"a", "b"}},
		{name: "FakeVariadic", args: []any{"a", "b", int64(1)}},
		{name: "FakeVariadic", args: []any{"a", "b", int64(1), nil, m}},
		{name: "FakeVariadic", args: []any{"a", "b"}, unwrap: []any{"x", int64(2)}},
		{name: "FakeVariadic", args: []any{"a"}, unwrap: []any{"x", int64(2)}},
		{name: "FakeVariadic", args: []any{nil, "b"}},
		{name: "FakeVariadic", args: []any{"a", int64(2)}},
		{name: "FakeVariadic", args: []any{"panic", "b"}},
		{name: "FakeVariadic", args: []any{"a"}},
		{name: "FakeVariadic", args: []any{"a", "b"}, count: 1},
		{name: "FakeVariadic", args: []any{"a", "b", "c"}, count: 0, stack: &extStacker{}},
		{name: "FakeAny", args: []any{nil}},
		{name: "FakeAny", args: []any{int64(3)}},
		{name: "FakeAny", args: []any{namedAny{"n"}}},
		{name: "FakeSlice", args: []any{[]any{int64(1), "a"}}},
		{name: "FakeSlice", args: []any{namedAny{"n", "m"}}},
		{name: "FakeSlice", args: []any{"x"}},
		{name: "FakeSlice", args: []any{nil}},
		{name: "FakeMap", args: []any{m, "s"}},
		{name: "FakeMap", args: []any{nilMap, "s"}},
		{name: "FakeMap", args: []any{map[string]any{}, "s"}},
		{name: "FakeMap", args: []any{nil, "s"}},
		{name: "FakeNoResult", args: []any{"a", int64(1)}},
		{name: "FakeNoResult", args: []any{nil, nil, "v"}},
	}
	for i, c := range cases {
		want := runExtCase(t, vm, c, false)
		got := runExtCase(t, vm, c, true)
		require.Equal(t, want, got, "%d %s %#v", i, c.name, c.args)
	}
}

// namedAny is assignable to []any, which a type assertion to []any does not take
type namedAny []any

// TestExtCallParams: VM.Call of an extended function gives the same results, panic included, by
// the typed call and by reflection
func TestExtCallParams(t *testing.T) {
	vm := extFakeVM()
	rt := NewRunTime(vm, 100)
	cases := []struct {
		name   string
		params []any
	}{
		{"FakeDB", []any{rt, "t", int64(4)}},
		{"FakeDB", []any{rt, "fail", int64(4)}},
		{"FakeDB", []any{rt, nil, int64(4)}},
		{"FakeDB", []any{rt, "t"}},
		{"FakeDB", []any{rt, "t", int64(4), "extra"}},
		{"FakeVariadic", []any{rt, "a", "b"}},
		{"FakeVariadic", []any{rt, "a", "b", int64(1), nil}},
		{"FakeVariadic", []any{rt, "a"}},
		{"FakeAny", []any{nil}},
		{"FakeSlice", []any{namedAny{"n"}}},
		{"FakeNoResult", []any{rt, "a", "b"}},
	}
	call := func(name string, params []any, typed bool) (ret []any, err string) {
		finfo := vm.Objects[name].GetExtFuncInfo()
		saved := finfo.Call
		if !typed {
			finfo.Call = nil
		}
		defer func() {
			finfo.Call = saved
			if r := recover(); r != nil {
				err = fmt.Sprint(r)
			}
		}()
		ret, e := vm.Call(name, params, map[string]any{})
		if e != nil {
			err = e.Error()
		}
		return ret, err
	}
	for i, c := range cases {
		want, wantErr := call(c.name, c.params, false)
		got, gotErr := call(c.name, c.params, true)
		require.Equal(t, wantErr, gotErr, "%d %s", i, c.name)
		require.Equal(t, want, got, "%d %s", i, c.name)
	}
}

// TestExtCallExecContract: ExecContract refuses a name of another type than a string alike by
// both calls
func TestExtCallExecContract(t *testing.T) {
	vm := NewVM()
	for _, args := range [][]any{{"@1Missing", int64(1)}, {"@1Missing", "a"}, {"@1Missing", "a", int64(1)}} {
		want := runExtCase(t, vm, extCase{name: "ExecContract", args: args}, false)
		got := runExtCase(t, vm, extCase{name: "ExecContract", args: args}, true)
		require.Equal(t, want, got)
		require.True(t, strings.HasPrefix(got.err, "unknown "), got.err)
	}
}

// TestExtCallScriptCoverage: every extended function of the VM itself has a typed call
func TestExtCallScriptCoverage(t *testing.T) {
	covered, total, reflected := NewVM().ExtCallCoverage()
	require.Empty(t, reflected)
	require.Equal(t, total, covered)
	require.Equal(t, len(NewExtendData().Objects), total)
}

// BenchmarkExtCallShape is a call of a function of the parameters and results of DBSelect, which
// does nothing, by its typed call and by reflection
func BenchmarkExtCallShape(b *testing.B) {
	type sc struct{}
	f := func(s *sc, table string, columns any, id int64, order any, offset, limit int64, where *types.Map,
		query any, group string, all bool) (int64, []any, error) {
		return 1, nil, nil
	}
	vm := NewVM()
	vm.SetFuncCallsDB(map[string]struct{}{"Select": {}})
	vm.Extend(&ExtendData{
		Objects:  map[string]any{"Select": f},
		AutoPars: map[string]string{reflect.TypeOf(&sc{}).String(): Extend_sc},
		Calls:    func(fn any) ExtCall { return ExtCall11R3(f) },
	})
	obj := vm.Objects["Select"]
	finfo := obj.GetExtFuncInfo()
	call := finfo.Call
	args := []any{"keys", "id,amount", int64(0), "id", int64(0), int64(1), types.NewMap(), "", "", false}
	for _, typed := range []bool{true, false} {
		name := "reflect"
		if typed {
			name = "typed"
		}
		b.Run(name, func(b *testing.B) {
			finfo.Call = nil
			if typed {
				finfo.Call = call
			}
			rt := NewRunTime(vm, 1<<62)
			rt.extend = map[string]any{Extend_sc: &sc{}}
			b.ReportAllocs()
			for b.Loop() {
				for _, arg := range args {
					rt.push(arg)
				}
				if err := rt.callFunc(cmdCall, obj); err != nil {
					b.Fatal(err)
				}
				rt.resetByIdx(0)
			}
		})
	}
	finfo.Call = call
}
