/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

func TestRollbacksHashCoversTheRecordsOfTheBlock(t *testing.T) {
	useSuite("ECC_P256", "SHA256")
	rts := []*sqldb.RollbackTx{
		{BlockID: 7, TxHash: []byte{1}, NameTable: "1_keys", TableID: "5", Data: `{"amount":"1"}`},
		{BlockID: 7, TxHash: []byte{2}, NameTable: "1_keys", TableID: "6", Data: `{"amount":"2"}`},
	}
	outputs := []sqldb.SpentInfo{
		{OutputTxHash: []byte{1}, OutputKeyId: 5, OutputValue: "1", Ecosystem: 1, BlockId: 7},
		{OutputTxHash: []byte{3}, OutputKeyId: 6, OutputValue: "4", Ecosystem: 1, BlockId: 3, InputTxHash: []byte{2}},
	}
	want, err := rollbacksHash(7, rts, outputs)
	if err != nil {
		t.Fatal(err)
	}

	reordered, err := rollbacksHash(7, []*sqldb.RollbackTx{rts[1], rts[0]}, []sqldb.SpentInfo{outputs[1], outputs[0]})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, reordered) {
		t.Error("the hash depends on the order of the records")
	}

	// an output of an earlier block that the block spends is not a record of the block
	own, err := rollbacksHash(7, rts, outputs[:1])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, own) {
		t.Error("the hash covers an output of another block")
	}

	changed := *rts[0]
	changed.Data = `{"amount":"9"}`
	other, err := rollbacksHash(7, []*sqldb.RollbackTx{&changed, rts[1]}, outputs)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(want, other) {
		t.Error("the hash does not cover the data of a rollback record")
	}
}
