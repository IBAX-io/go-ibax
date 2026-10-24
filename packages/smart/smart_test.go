/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
)

type TestSmart struct {
	Input  string
	Output string
}

func TestNewContract(t *testing.T) {
	test := []TestSmart{
		{`contract NewCitizen {
			data {
				Public bytes
				MyVal  string
			}
			 func conditions {
				Println( "Front")
				//$tmp = "Test string"
//				Println("NewCitizen Front", $tmp, $key_id, $ecosystem_id, $PublicKey )
			}
			settings {
				abc = 123
			}
			func action {
//				Println("NewCitizen Main", $tmp, $type, $key_id )
//				DBInsert(Sprintf( "%d_citizens", $ecosystem_id), "public_key,block_id", $PublicKey, $block)
			}
}			
		`, ``},
	}
	owner := script.OwnerInfo{
		StateID:  1,
		Active:   false,
		TableID:  1,
		WalletID: 0,
		TokenID:  0,
	}
	InitVM()
	for _, item := range test {
		if err := script.GetVM().Compile([]rune(item.Input), &owner); err != nil {
			t.Error(err)
		}
	}
	cnt := GetContract(`NewCitizen`, 1)
	cfunc := cnt.GetFunc(`conditions`)
	_, err := script.VMRun(script.GetVM(), cfunc, nil, map[string]any{}, nil)
	if err != nil {
		t.Error(err)
	}
}

func TestCheckAppend(t *testing.T) {
	appendTestContract := `contract AppendTest {
		action {
			var list array
			list = Append(list, "naw_value")
			Println(list)
		}
	}`

	owner := script.OwnerInfo{
		StateID:  1,
		Active:   false,
		TableID:  1,
		WalletID: 0,
		TokenID:  0,
	}

	require.NoError(t, script.GetVM().Compile([]rune(appendTestContract), &owner))

	cnt := GetContract("AppendTest", 1)
	cfunc := cnt.GetFunc("action")

	_, err := script.VMRun(script.GetVM(), cfunc, nil, map[string]any{}, nil)
	require.NoError(t, err)
}

// The conditions ContractConditions runs see the stack of the transaction, its contract first and
// theirs last, as the code of the transaction's contract does
func TestContractConditionsStack(t *testing.T) {
	InitVM()
	owner := script.OwnerInfo{StateID: 1, TableID: 1}
	require.NoError(t, script.GetVM().Compile([]rune(`contract StackTx {
		action {
		}
	}`), &owner))
	require.NoError(t, script.GetVM().Compile([]rune(`contract StackCondition {
		conditions {
			if Join($stack, ",") != "@1StackTx,@1StackCondition" {
				error "unexpected stack"
			}
		}
	}`), &owner))

	sc := &SmartContract{
		VM:         script.GetVM(),
		TxSmart:    &types.SmartTransaction{Header: &types.Header{EcosystemID: 1}, MaxSum: "100000"},
		TxContract: &Contract{Name: "@1StackTx", Extend: map[string]any{}},
		Key:        &sqldb.Key{},
	}
	require.NoError(t, sc.AppendStack("@1StackTx"))
	ok, err := ContractConditions(sc, "@1StackCondition")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []any{"@1StackTx"}, sc.TxContract.StackCont)
}

// The founder alone writes the parameters of an ecosystem out of DAO mode; in DAO mode the
// permissions of the tables do, the founder no more than any other key
func TestParamsFounderGate(t *testing.T) {
	for _, c := range []struct {
		mode, key, founder int64
		gated, allowed     bool
	}{
		{1, 7, 7, true, true},
		{1, 8, 7, true, false},
		{1, 0, 0, true, false},
		{0, 7, 7, true, true},
		{2, 7, 7, false, false},
		{2, 8, 7, false, false},
	} {
		gated, allowed := paramsFounderGate(c.mode, c.key, c.founder)
		require.Equal(t, c.gated, gated, "mode %d key %d", c.mode, c.key)
		require.Equal(t, c.allowed, allowed, "mode %d key %d", c.mode, c.key)
	}
}

// A condition that is ContractAccess of names is, for any stack, what the virtual machine makes
// of it; any other condition is left to the virtual machine
func TestContractAccessCondition(t *testing.T) {
	InitVM()
	for cond, names := range map[string][]any{
		`ContractAccess("@1TokensTransfer")`:               {"@1TokensTransfer"},
		` ContractAccess("@1A", "B" ,"@2C_d")  `:           {"@1A", "B", "@2C_d"},
		`ContractAccess("@1A") && ContractAccess("@1B")`:   nil,
		`ContractAccess("@1A") || true`:                    nil,
		`ContractAccess("@1A","@1B\")`:                     nil,
		`ContractAccess($name)`:                            nil,
		`ContractAccess()`:                                 nil,
		`ContractAccess("")`:                               nil,
		`ContractConditions("@1MainCondition")`:            nil,
		`ContractAccess("@1A"),ContractAccess("@1B")`:      nil,
		"ContractAccess(\"@1A\")\n":                        {"@1A"},
		`ContractAccess("a b")`:                            nil,
		`ContractAccess("@1A") ContractAccess("@1B")`:      nil,
		`ContractAccess("@1A", "@1B", "@1TokensTransfer")`: {"@1A", "@1B", "@1TokensTransfer"},
	} {
		require.Equal(t, names, parseContractAccess(cond), cond)
		if names == nil {
			continue
		}
		for _, stack := range [][]any{nil, {"@1TokensTransfer"}, {"@1B"}, {"@1Other", "@1A"},
			{"@1A", "@1Other"}, {"@2C_d"}, {"@1B", "NotAt"}, {"@3B"}} {
			sc := &SmartContract{
				VM:         script.GetVM(),
				TxSmart:    &types.SmartTransaction{Header: &types.Header{EcosystemID: 1}, MaxSum: "100000"},
				TxContract: &Contract{Name: "@1Tx", Extend: map[string]any{}, StackCont: stack},
				Key:        &sqldb.Key{},
			}
			want, err := sc.VM.EvalIf(cond, 1, sc.getExtend())
			require.NoError(t, err, cond)
			got, err := sc.EvalIf(cond)
			require.NoError(t, err, cond)
			require.Equal(t, want, got, "%s %v", cond, stack)
		}
	}
}
