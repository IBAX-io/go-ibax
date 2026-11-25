/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// testStored is the stored form of a transaction of the smart transaction tx
func testStored(t *testing.T, tx *types.SmartTransaction) ([]byte, []byte) {
	t.Helper()
	crypto.InitHashAlgo("SHA3_256")
	payload, err := msgpack.Marshal(tx)
	require.NoError(t, err)
	body, err := msgpack.Marshal(&storedTx{Payload: payload, TxSignature: []byte{1}, Timestamp: tx.Time * 1000})
	require.NoError(t, err)
	return append([]byte{tx.TxType()}, body...), payload
}

// Decode reads a transaction without its contract; Bind binds a copy of it to the contract as the
// node has it, each time a new one sharing what Decode read, and refuses a contract the node does
// not know; a transfer of the UTXO model binds to no contract
func TestDecodeBind(t *testing.T) {
	const signed = 1_700_000_000
	data, payload := testStored(t, &types.SmartTransaction{
		Header: &types.Header{ID: 999_999, EcosystemID: 1, KeyID: 7, Time: signed, NetworkID: 1},
		Params: map[string]any{"Value": "1"},
	})
	tx, err := Decode(data)
	require.NoError(t, err)
	require.Equal(t, crypto.DoubleHash(payload), tx.Hash())
	require.EqualValues(t, signed*1000, tx.Timestamp())
	require.Equal(t, data, tx.FullData)
	require.Nil(t, tx.SmartContract().TxContract)
	_, err = tx.Bind()
	require.ErrorContains(t, err, "unknown contract 999999")

	data, _ = testStored(t, &types.SmartTransaction{
		Header:       &types.Header{EcosystemID: 1, KeyID: 7, Time: signed, NetworkID: 1},
		TransferSelf: &types.TransferSelf{Value: "1", Source: "UTXO", Target: "Account"},
	})
	tx, err = Decode(data)
	require.NoError(t, err)
	a, err := tx.Bind()
	require.NoError(t, err)
	b, err := tx.Bind()
	require.NoError(t, err)
	require.NotSame(t, a.SmartContract(), b.SmartContract())
	require.NotSame(t, tx.SmartContract(), a.SmartContract())
	require.Same(t, tx.SmartContract().TxSmart, a.SmartContract().TxSmart)
	require.Equal(t, tx.Hash(), a.Hash())
	require.Equal(t, int64(len(data)), a.Inner.txSize())
	require.Equal(t, tx.Compressed(), a.Compressed())

	_, err = Decode(append([]byte{types.SmartContractTxType}, payload...))
	require.Error(t, err, "not the stored form")
}
