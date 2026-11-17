/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
)

func TestRollbacksHashCoversTheRecordsOfTheBlock(t *testing.T) {
	useSuite("ECC_P256", "SHA256")
	rts := []*types.RollbackTx{
		{BlockId: 7, TxHash: []byte{1}, NameTable: "1_keys", TableId: "5", Data: `{"amount":"1"}`, DataHash: []byte{9}},
		{BlockId: 7, TxHash: []byte{2}, NameTable: "1_keys", TableId: "6", Data: `{"amount":"2"}`, DataHash: []byte{8}},
	}
	outputs := []sqldb.SpentInfo{
		{OutputTxHash: []byte{1}, OutputKeyId: 5, OutputValue: "1", Ecosystem: 1, BlockId: 7},
		{OutputTxHash: []byte{3}, OutputKeyId: 6, OutputValue: "4", Ecosystem: 1, BlockId: 3, InputTxHash: []byte{2}},
	}
	hashes := func(rts ...*types.RollbackTx) []string {
		h, err := rollbackHashes(rts)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	want, err := rollbacksHash(7, hashes(rts...), outputs)
	if err != nil {
		t.Fatal(err)
	}

	reordered, err := rollbacksHash(7, hashes(rts[1], rts[0]), []sqldb.SpentInfo{outputs[1], outputs[0]})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, reordered) {
		t.Error("the hash depends on the order of the records")
	}

	// an output of an earlier block that the block spends is not a record of the block
	own, err := rollbacksHash(7, hashes(rts...), outputs[:1])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, own) {
		t.Error("the hash covers an output of another block")
	}

	changed := *rts[0]
	changed.Data = `{"amount":"9"}`
	other, err := rollbacksHash(7, hashes(&changed, rts[1]), outputs)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(want, other) {
		t.Error("the hash does not cover the data of a rollback record")
	}
}

// The hash of a rollback record is of the JSON of the record as rollback_tx had it, whatever the
// table has now: the next block checks the rollbacks hash of the last
func TestRollbackHashForm(t *testing.T) {
	useSuite("ECC_P256", "SHA256")
	rt := &types.RollbackTx{BlockId: 7, TxHash: []byte{1}, NameTable: "1_keys", TableId: "5", Data: `{"amount":"1"}`,
		DataHash: []byte{9}}
	got, err := rollbackHash(rt)
	if err != nil {
		t.Fatal(err)
	}
	want := crypto.HashHex([]byte(`{"ID":0,"block_id":7,"tx_hash":"AQ==","table_name":"1_keys","table_id":"5",` +
		`"data":"{\"amount\":\"1\"}","data_hash":"CQ=="}`))
	if got != want {
		t.Errorf("the hash of a rollback record is %s, the form of the chain gives %s", got, want)
	}
}
