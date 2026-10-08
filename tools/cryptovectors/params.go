/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/vmihailenco/msgpack/v5"
)

const paramsSource = "go-ibax smart.FillTxData per declared type (types), after decoding the client transaction " +
	"like the node (0x80 envelope, msgpack v5) via tools/cryptovectors; data is written by the client. Test key only."

type paramsFile struct {
	Source string `json:"source"`
	// client side
	PrivateKey  string            `json:"privateKey"`
	ContractID  int64             `json:"contractID"`
	EcosystemID int64             `json:"ecosystemID"`
	NetworkID   int64             `json:"networkID"`
	Time        int64             `json:"time"`
	Types       map[string]string `json:"types"`
	Data        string            `json:"data"`
	// node side
	Header string            `json:"header"`
	Node   map[string]string `json:"node"`
}

// Contract field types as declared in a contract's data section
var declaredTypes = map[string]uint32{
	"bool":    script.DtBool,
	"int":     script.DtInt,
	"float":   script.DtFloat,
	"money":   script.DtMoney,
	"string":  script.DtString,
	"bytes":   script.DtBytes,
	"array":   script.DtArray,
	"map":     script.DtMap,
	"address": script.DtAddress,
	"file":    script.DtFile,
}

func updateContractParams(raw []byte) (any, []string, error) {
	var f paramsFile
	if err := decode(raw, &f); err != nil {
		return nil, nil, err
	}
	f.Source = paramsSource
	// Contract calls are signed with the client's default suite; FillTxData does not depend on it
	if err := useSuite("ECC_Secp256k1", "KECCAK256"); err != nil {
		return nil, nil, err
	}
	data, err := hex.DecodeString(f.Data)
	if err != nil {
		return nil, nil, fmt.Errorf("data: %w", err)
	}

	buf := bytes.NewBuffer(data)
	if kind, err := buf.ReadByte(); err != nil || kind != 0x80 {
		return nil, nil, fmt.Errorf("not a client transaction")
	}
	var payload []byte
	if err := converter.BinUnmarshalBuff(buf, &payload); err != nil {
		return nil, nil, fmt.Errorf("payload: %w", err)
	}
	var smartTx types.SmartTransaction
	if err := msgpack.Unmarshal(payload, &smartTx); err != nil {
		return nil, nil, fmt.Errorf("body: %w", err)
	}
	f.Header = fmt.Sprintf("ID=%d Time=%d Eco=%d Net=%d KeyID=%d",
		smartTx.ID, smartTx.Time, smartTx.EcosystemID, smartTx.NetworkID, smartTx.KeyID)

	names := make([]string, 0, len(f.Types))
	for name := range f.Types {
		names = append(names, name)
	}
	sort.Strings(names)
	f.Node = make(map[string]string, len(names))
	for _, name := range names {
		dt, ok := declaredTypes[f.Types[name]]
		if !ok {
			return nil, nil, fmt.Errorf("%s: unknown type %q", name, f.Types[name])
		}
		value, present := smartTx.Params[name]
		if !present {
			f.Node[name] = "ERR missing"
			continue
		}
		// One field at a time, so each parameter shows its own conversion or refusal
		filled, err := smart.FillTxData([]*script.FieldInfo{{Name: name, Original: dt}}, map[string]any{name: value})
		if err != nil {
			f.Node[name] = "ERR " + err.Error()
			continue
		}
		f.Node[name] = fmt.Sprintf("%T %v", filled[name], filled[name])
	}
	return f, nil, nil
}
