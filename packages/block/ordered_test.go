/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/jackc/pgx/v5/pgconn"
)

// An error of the transaction and the state discards the transaction, for the reason of its kind;
// an error of the node, or the stop of the network, fails the execution of the block, which the
// node executes again
func TestDiscardOf(t *testing.T) {
	for _, c := range []struct {
		what   string
		err    error
		reason int32
		ok     bool
	}{
		{"no error", nil, 0, false},
		{"the stop of the network", transaction.ErrNetworkStopping, 0, false},
		{"an error of PostgreSQL", fmt.Errorf("query: %w", &pgconn.PgError{Code: "40001"}), 0, false},
		{"an error of the network", &net.OpError{Op: "read", Err: errors.New("reset")}, 0, false},
		{"a closed connection", fmt.Errorf("x: %w", sql.ErrConnDone), 0, false},
		{"a done transaction", sql.ErrTxDone, 0, false},
		{"a canceled context", fmt.Errorf("x: %w", context.Canceled), 0, false},
		{"the end of a stream", io.ErrUnexpectedEOF, 0, false},
		{"the fuel", fmt.Errorf("x: %w", transaction.ErrFuelLimit), discardFuel, true},
		{"the signer", script.SetVMError("panic", "unknown key"), discardCheck, true},
		{"the funds", errors.New("not enough money"), discardCheck, true},
	} {
		reason, ok := discardOf(c.err)
		if reason != c.reason || ok != c.ok {
			t.Errorf("%s: reason %d, discarded %v; want %d, %v", c.what, reason, ok, c.reason, c.ok)
		}
	}
}

// Only the execution of an ordered block but the first discards: a block before
// consts.BvOrdered, or one the node generates, fails as it did
func TestDiscardableBlocks(t *testing.T) {
	err := errors.New("not enough money")
	for _, c := range []struct {
		what     string
		version  int32
		height   int64
		generate bool
		ok       bool
	}{
		{"an ordered block", consts.BvOrdered, 7, false, true},
		{"the first ordered block", consts.BvOrdered, 1, false, false},
		{"a generated block", consts.BvOrdered, 7, true, false},
		{"a block of version 4", consts.BvValidatorsHash, 7, false, false},
	} {
		b := &Block{BlockData: &types.BlockData{Header: &types.BlockHeader{BlockId: c.height, Version: c.version}}, GenBlock: c.generate}
		if _, ok := b.discardable(err); ok != c.ok {
			t.Errorf("%s: discarded %v, want %v", c.what, ok, c.ok)
		}
	}
}

// The validators set the execution of a block leaves is of BftSetDelay heights after an ordered
// block, of the next height after another; block 1 leaves the set of height 2
func TestSetHeight(t *testing.T) {
	for _, c := range []struct {
		version     int32
		height, set int64
	}{
		{consts.BvOrdered, 1, 2}, {consts.BvOrdered, 2, 5}, {consts.BvOrdered, 10, 13},
		{consts.BvValidatorsHash, 1, 2}, {consts.BvValidatorsHash, 10, 11},
	} {
		b := &Block{BlockData: &types.BlockData{Header: &types.BlockHeader{BlockId: c.height, Version: c.version}}}
		if got := b.setHeight(); got != c.set {
			t.Errorf("block %d of version %d leaves the set of %d, want %d", c.height, c.version, got, c.set)
		}
	}
}

// The execution hash takes the height, the rollbacks hash, of each transaction in its order its
// hash, its status and the reason of its discard, and the set the block leaves; not the text of
// the error of a discard
func TestExecHash(t *testing.T) {
	useSuite("ECC_P256", "SHA256")
	make := func(change func(b *Block)) []byte {
		b := &Block{BlockData: &types.BlockData{Header: &types.BlockHeader{BlockId: 9, Version: consts.BvOrdered}}}
		b.rollbacksHash = []byte{1, 2}
		b.nextValidators = []byte("set")
		b.AfterTxs = &types.AfterTxs{Txs: []*types.AfterTx{
			{Lts: &types.LogTransaction{Hash: []byte{0xa}, InvokeStatus: pbgo.TxInvokeStatusCode_SUCCESS}},
			{Lts: &types.LogTransaction{Hash: []byte{0xb}, InvokeStatus: pbgo.TxInvokeStatusCode_DISCARDED},
				UpdTxStatus: &pbgo.TxResult{Result: "not enough money"}},
		}}
		b.discards = map[string]int32{string([]byte{0xb}): discardCheck}
		if change != nil {
			change(b)
		}
		return b.execHash()
	}
	base := make(nil)
	if !bytes.Equal(base, make(nil)) {
		t.Fatal("the execution hash of the same execution differs")
	}
	if !bytes.Equal(base, make(func(b *Block) { b.AfterTxs.Txs[1].UpdTxStatus.Result = "other text" })) {
		t.Fatal("the execution hash takes the text of the error of a discard")
	}
	for what, change := range map[string]func(b *Block){
		"the height":         func(b *Block) { b.Header.BlockId = 10 },
		"the rollbacks hash": func(b *Block) { b.rollbacksHash = []byte{1, 3} },
		"the set":            func(b *Block) { b.nextValidators = []byte("other set") },
		"a status":           func(b *Block) { b.AfterTxs.Txs[0].Lts.InvokeStatus = pbgo.TxInvokeStatusCode_PENALTY },
		"a reason":           func(b *Block) { b.discards[string([]byte{0xb})] = discardFuel },
		"the order": func(b *Block) {
			b.AfterTxs.Txs[0], b.AfterTxs.Txs[1] = b.AfterTxs.Txs[1], b.AfterTxs.Txs[0]
		},
		"a transaction": func(b *Block) { b.AfterTxs.Txs = b.AfterTxs.Txs[:1] },
		"the boundaries": func(b *Block) {
			b.rollbacksHash, b.nextValidators = []byte{1}, append([]byte{2}, b.nextValidators...)
		},
	} {
		if bytes.Equal(base, make(change)) {
			t.Errorf("the execution hash does not take %s", what)
		}
	}
}

// A discarded transaction leaves a log of status DISCARDED with the text of its error, bounded,
// and no rollback record
func TestDiscardRecord(t *testing.T) {
	b := &Block{BlockData: &types.BlockData{Header: &types.BlockHeader{BlockId: 9, Version: consts.BvOrdered}}}
	tx := &transaction.Transaction{Inner: &transaction.SmartTransactionParser{SmartContract: &smart.SmartContract{
		Hash: []byte{7}, TxSmart: &types.SmartTransaction{Header: &types.Header{EcosystemID: 3, KeyID: 5}},
		TxContract: &smart.Contract{Name: "@1TokensSend"}}}}
	afters := &types.AfterTxs{}
	long := bytes.Repeat([]byte("x"), 1000)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("discarding a transaction without its contract: %v", r)
		}
	}()
	b.discard(afters, tx, errors.New(string(long)), discardFuel)
	if len(afters.Txs) != 1 || len(afters.Rts) != 0 {
		t.Fatalf("%d logs and %d rollback records, want 1 and 0", len(afters.Txs), len(afters.Rts))
	}
	a := afters.Txs[0]
	if a.Lts.InvokeStatus != pbgo.TxInvokeStatusCode_DISCARDED || a.UpdTxStatus.Code != pbgo.TxInvokeStatusCode_DISCARDED ||
		a.Lts.Block != 9 || a.UpdTxStatus.BlockId != 9 || a.Lts.EcosystemId != 3 || a.Lts.ContractName != "@1TokensSend" ||
		!bytes.Equal(a.UsedTx, []byte{7}) || len(a.UpdTxStatus.Result) != maxDiscardText {
		t.Fatalf("the log of the discard: %+v %+v", a.Lts, a.UpdTxStatus)
	}
	if b.discards[string(tx.Hash())] != discardFuel {
		t.Fatalf("the reason of the discard: %d", b.discards[string(tx.Hash())])
	}
}
