/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"errors"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// signedTx is a transaction of a contract signed at signed, in seconds, that a node received at
// received, in milliseconds
func signedTx(signed, received int64) *Transaction {
	return &Transaction{Inner: &SmartTransactionParser{SmartContract: &smart.SmartContract{
		TxSmart:   &types.SmartTransaction{Header: &types.Header{ID: 1, KeyID: 1, Time: signed}},
		Hash:      []byte{1},
		Timestamp: received,
	}}}
}

// The window is of the signed time: the time a node received the transaction at moves nothing
func TestCheckTimeSigned(t *testing.T) {
	const block = int64(1) << 32
	now := time.Unix(block, 0).UnixMilli()
	for _, c := range []struct {
		name   string
		signed int64
		want   error
	}{
		{"at the block time", block, nil},
		{"MaxTxBack before", block - consts.MaxTxBack, nil},
		{"MaxTxForw after", block + consts.MaxTxForw, nil},
		{"older than MaxTxBack, received now", block - consts.MaxTxBack - 1, ErrExpiredTime},
		{"later than MaxTxForw", block + consts.MaxTxForw + 1, ErrEarlyTime},
	} {
		if err := signedTx(c.signed, now).checkTime(block); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	// received long before or after, the time of the node is not checked
	if err := signedTx(block, 0).checkTime(block); err != nil {
		t.Errorf("received at 0: %v", err)
	}
	if err := signedTx(block, now+int64(2*consts.MaxTxBack)*1000).checkTime(block); err != nil {
		t.Errorf("received later: %v", err)
	}
}

// A transaction of a block sent again once the window of the duplicates can let it go (the time of
// its block, plus MaxTxForw, is earlier than MaxTxBack before the block time; sqldb.TxWindow) is
// refused, whatever time the node receiving it gives it
func TestCheckTimeReplayPastWindow(t *testing.T) {
	const last = int64(1) << 32
	leaves := last + consts.MaxTxForw + consts.MaxTxBack + 1
	// the latest a block of the range takes: signed MaxTxForw after the last block
	for _, signed := range []int64{last + consts.MaxTxForw, last, last - consts.MaxTxBack} {
		replay := signedTx(signed, time.Unix(leaves, 0).UnixMilli())
		if err := replay.checkTime(leaves); !errors.Is(err, ErrExpiredTime) {
			t.Errorf("signed at %d, sent again at %d: %v", signed, leaves, err)
		}
	}
	// one second before, the latest is still in the window, and its hash in the window refuses it
	if err := signedTx(last+consts.MaxTxForw, 0).checkTime(leaves - 1); err != nil {
		t.Errorf("before the range leaves: %v", err)
	}
}
