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
