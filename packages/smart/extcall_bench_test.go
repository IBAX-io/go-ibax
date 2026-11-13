package smart

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// BenchmarkContractExtCalls runs a function of a contract making 800 calls of extended functions
// a transfer makes: Len, Int, Str, Sprintf, Money, Contains, IdToAddress and ContractAccess
func BenchmarkContractExtCalls(b *testing.B) {
	vm := script.NewVM()
	vm.SetFuncCallsDB(funcCallsDBP)
	vm.Extend(extendData(script.VMType_Smart))
	if err := vm.Compile([]rune(`func benchCalls() int {
		var a array
		var i, n int
		var s string
		var m money
		a = [1, 2, 3]
		while i < 100 {
			n = n + Len(a) + Int("4")
			s = Str(i) + Sprintf("%d", i) + IdToAddress(i)
			m = Money("12.5")
			if Contains(s, "1") && ContractAccess("Test") {
				n = n + 1
			}
			i = i + 1
		}
		return n
	}`), &script.OwnerInfo{StateID: 1, Active: true, TableID: 1}); err != nil {
		b.Fatal(err)
	}
	sc := &SmartContract{VM: vm, TxContract: &Contract{Name: "@1Test", StackCont: []any{"@1Test"}},
		TxSmart: &types.SmartTransaction{Header: &types.Header{EcosystemID: 1}}}
	b.ReportAllocs()
	for b.Loop() {
		out, err := vm.Call(`benchCalls`, nil, map[string]any{`rt_state`: uint32(1), `sc`: sc,
			script.Extend_txcost: int64(1 << 40)})
		if err != nil {
			b.Fatal(err)
		}
		if out[0].(int64) != 719 {
			b.Fatal(out[0])
		}
	}
}
