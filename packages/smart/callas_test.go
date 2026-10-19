package smart

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// The conditions of an account name contracts by commas, with spaces, the ones without the @ of an
// ecosystem of the account's
func TestControllingContracts(t *testing.T) {
	require.Equal(t, []string{"@1MultiSignConfirm", "@2Swap", "@3Local"},
		ControllingContracts(" @1MultiSignConfirm, @2Swap,,Local ", 3))
	require.Empty(t, ControllingContracts("", 1))
}

// Within CallContractAs the account is the one that writes the parameters and binds the wallets of
// contracts; the key of the transaction outside it
func TestActingKeyID(t *testing.T) {
	sc := &SmartContract{TxSmart: &types.SmartTransaction{Header: &types.Header{KeyID: 7}}}
	require.Equal(t, int64(7), sc.actingKeyID())
	sc.callAs = 5
	require.Equal(t, int64(5), sc.actingKeyID())
}

// The JSON object of the parameters gives each field a value of its type, and nothing else
func TestContractParams(t *testing.T) {
	fields := &[]*script.FieldInfo{
		{Name: "Amount", Original: script.DtMoney},
		{Name: "Id", Original: script.DtInt},
		{Name: "Name", Original: script.DtString},
		{Name: "Flag", Original: script.DtBool},
		{Name: "List", Original: script.DtArray},
		{Name: "Data", Original: script.DtMap},
		{Name: "Raw", Original: script.DtBytes},
	}
	values, err := ContractParams(fields, `{"Amount": "1000000000000000000000", "Id": 12, "Name": "x",
		"Flag": true, "List": [1, "a"], "Data": {"k": 2}, "Raw": "0a0b"}`)
	require.NoError(t, err)
	amount, _ := values.Get("Amount")
	require.Equal(t, "1000000000000000000000", amount.(decimal.Decimal).String())
	id, _ := values.Get("Id")
	require.Equal(t, int64(12), id)
	list, _ := values.Get("List")
	require.Equal(t, []any{int64(1), "a"}, list)
	data, _ := values.Get("Data")
	k, _ := data.(*types.Map).Get("k")
	require.Equal(t, int64(2), k)
	raw, _ := values.Get("Raw")
	require.Equal(t, []byte{10, 11}, raw)

	values, err = ContractParams(fields, ``)
	require.NoError(t, err)
	require.Zero(t, values.Size())

	for _, params := range []string{
		`{"Other": 1}`,        // no field of the contract
		`{"Amount": "1.5"}`,   // money is an integer
		`{"Id": "x"}`,         // not a number
		`{"Id": 1.5}`,         // not an integer
		`{"Name": 1}`,         // not a string
		`{"Flag": "true"}`,    // not a boolean
		`{"Data": [1]}`,       // not an object
		`{"Raw": "zz"}`,       // not hex
		`[1]`,                 // not an object
		`{"Id": 1} {"Id": 2}`, // two values
		`{"Id": 99999999999999999999}`,
	} {
		_, err := ContractParams(fields, params)
		require.Error(t, err, params)
	}
}

// A contract calls CallContractAs with the account, the contract and the parameters: the VM
// passes the SmartContract and the RunTime itself
func TestCallContractAsCompiles(t *testing.T) {
	vm := script.NewVM()
	vm.Extend(extendData(script.VMType_Smart))
	require.NoError(t, vm.Compile([]rune(`contract AsCaller {
		action {
			CallContractAs("0000-0000-0000-0000-1009", "@1AsCallee", "{}")
		}
	}`), &script.OwnerInfo{StateID: 1}))
}
