/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"bytes"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// The window of the duplicates reads the hash and the signed time of a transaction that calls a
// contract the node does not know, as one a committed block not yet executed creates; the parse of
// the transaction refuses it
func TestWindowEntryUnknownContract(t *testing.T) {
	const signed = 1_700_000_000
	payload, err := msgpack.Marshal(&types.SmartTransaction{
		Header: &types.Header{ID: 999_999, EcosystemID: 1, KeyID: 7, Time: signed, NetworkID: 1},
		Params: map[string]any{"Value": "1"},
	})
	require.NoError(t, err)
	body, err := msgpack.Marshal(&storedTx{Payload: payload, TxSignature: []byte{1}, Timestamp: signed * 1000})
	require.NoError(t, err)
	data := append([]byte{types.SmartContractTxType}, body...)

	_, err = UnmarshallTransaction(bytes.NewBuffer(data), true)
	require.ErrorContains(t, err, "unknown contract 999999")

	hash, at, err := WindowEntry(data)
	require.NoError(t, err)
	require.Equal(t, crypto.DoubleHash(payload), hash)
	require.EqualValues(t, signed, at)

	_, _, err = WindowEntry(nil)
	require.Error(t, err)
	_, _, err = WindowEntry(append([]byte{types.SmartContractTxType}, payload...))
	require.Error(t, err, "not the stored form")
}
