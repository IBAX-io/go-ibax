package smart

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
)

// ControllingContracts are the contracts that the conditions of an account name, separated by
// commas; a name without the @ of an ecosystem is of the ecosystem of the account, as
// @1NewUserWithSeed reads it
func ControllingContracts(conditions string, ecosystem int64) []string {
	var names []string
	for _, name := range strings.Split(conditions, `,`) {
		name = strings.TrimSpace(name)
		if len(name) == 0 {
			continue
		}
		if name[0] != '@' {
			name = fmt.Sprintf(`@%d`, ecosystem) + name
		}
		names = append(names, name)
	}
	return names
}

// CallContractAs runs the contract name as the account of the ecosystem of the transaction:
// $key_id, $account_id and the roles are the account's while the contract and the contracts it
// calls run. Only a contract that the conditions of the account name may call it, and only for an
// account without a key, neither deleted nor blocked: the account is controlled by these
// contracts alone, and signs no transaction (CheckTxSign refuses it). The transaction's signer
// still pays its fees. params is a JSON object of the contract's fields (ContractParams).
func CallContractAs(sc *SmartContract, rt *script.RunTime, account, name, params string) (any, error) {
	if sc.TxContract == nil || len(sc.TxContract.StackCont) == 0 {
		return nil, fmt.Errorf(eCallAsCaller, account)
	}
	caller, _ := sc.TxContract.StackCont[len(sc.TxContract.StackCont)-1].(string)
	id := converter.AddressToID(account)
	if id == 0 {
		return nil, fmt.Errorf(eCallAsAccount, account)
	}
	ecosystem := sc.TxSmart.EcosystemID
	contract := VMGetContract(sc.VM, name, uint32(ecosystem))
	if contract == nil {
		return nil, fmt.Errorf(eUnknownContract, name)
	}
	values, err := ContractParams(contract.Block.GetContractInfo().Tx, params)
	if err != nil {
		return nil, err
	}
	key := &sqldb.Key{}
	found, err := key.SetTablePrefix(ecosystem).Get(sc.DbTransaction, id)
	if err != nil {
		return nil, err
	}
	if !found || key.Disable() {
		return nil, fmt.Errorf(eCallAsAccount, account)
	}
	if len(key.PublicKey) > 0 {
		return nil, fmt.Errorf(eCallAsKey, account)
	}
	var controls bool
	for _, item := range ControllingContracts(key.Conditions, ecosystem) {
		if item == caller {
			controls = true
		}
	}
	if !controls {
		return nil, fmt.Errorf(eCallAsCaller, account)
	}

	prevKey, prevCallAs := sc.Key, sc.callAs
	key.SetTablePrefix(ecosystem)
	sc.Key, sc.callAs = key, key.ID
	defer func() { sc.Key, sc.callAs = prevKey, prevCallAs }()
	return script.ExContractAs(rt, uint32(ecosystem), contract.Name, values, key.ID, key.AccountID)
}

// ContractParams are the values of the fields of a contract that the JSON object params gives,
// each of the type of its field: a number or the text of one for int, address and money (an
// integer), a number for float, a string, a boolean, the hex of bytes, an array, an object for
// map. A name that is no field of the contract, or a value of another type, is refused.
func ContractParams(fields *[]*script.FieldInfo, params string) (*types.Map, error) {
	values := types.NewMap()
	var in map[string]any
	if len(strings.TrimSpace(params)) > 0 {
		dec := json.NewDecoder(bytes.NewReader([]byte(params)))
		dec.UseNumber()
		if err := dec.Decode(&in); err != nil {
			return nil, fmt.Errorf(eContractParams, err)
		}
		if dec.More() {
			return nil, fmt.Errorf(eContractParams, "more than one JSON value")
		}
	}
	known := make(map[string]*script.FieldInfo)
	if fields != nil {
		for _, field := range *fields {
			known[field.Name] = field
		}
	}
	for name, val := range in {
		field, ok := known[name]
		if !ok {
			return nil, fmt.Errorf(eContractParams, fmt.Sprintf("%s is no field of the contract", name))
		}
		v, err := fieldValue(field.Original, val)
		if err != nil {
			return nil, fmt.Errorf(eContractParams, fmt.Sprintf("%s: %v", name, err))
		}
		values.Set(name, v)
	}
	return values, nil
}

func fieldValue(original uint32, val any) (any, error) {
	integer := func() (decimal.Decimal, error) {
		var text string
		switch v := val.(type) {
		case json.Number:
			text = v.String()
		case string:
			text = strings.TrimSpace(v)
		default:
			return decimal.Zero, fmt.Errorf("not a number")
		}
		d, err := decimal.NewFromString(text)
		if err != nil || !d.IsInteger() {
			return decimal.Zero, fmt.Errorf("%q is not an integer", text)
		}
		return d, nil
	}
	switch original {
	case script.DtBool:
		if v, ok := val.(bool); ok {
			return v, nil
		}
	case script.DtInt, script.DtAddress:
		d, err := integer()
		if err != nil {
			return nil, err
		}
		if !d.BigInt().IsInt64() {
			return nil, fmt.Errorf("%s is out of range", d)
		}
		return d.IntPart(), nil
	case script.DtMoney:
		return integer()
	case script.DtFloat:
		if v, ok := val.(json.Number); ok {
			return v.Float64()
		}
	case script.DtString:
		if v, ok := val.(string); ok {
			return v, nil
		}
	case script.DtBytes:
		if v, ok := val.(string); ok {
			return hex.DecodeString(v)
		}
	case script.DtArray:
		if v, ok := val.([]any); ok {
			return jsonValue(v), nil
		}
	case script.DtMap:
		if v, ok := val.(map[string]any); ok {
			return jsonValue(v), nil
		}
	}
	return nil, fmt.Errorf("not a value of the type of the field")
}

// jsonValue is a decoded JSON value as the contracts have it: an object a map, a number an int
// when it is an integer and a float otherwise
func jsonValue(val any) any {
	switch v := val.(type) {
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n
		}
		f, _ := v.Float64()
		return f
	case []any:
		for i, item := range v {
			v[i] = jsonValue(item)
		}
	case map[string]any:
		for key, item := range v {
			v[key] = jsonValue(item)
		}
		return types.ConvertMap(v)
	}
	return val
}

// actingKeyID is the account the contract runs as: the account of CallContractAs within it, the
// key of the transaction otherwise
func (sc *SmartContract) actingKeyID() int64 {
	if sc.callAs != 0 {
		return sc.callAs
	}
	return sc.TxSmart.KeyID
}
