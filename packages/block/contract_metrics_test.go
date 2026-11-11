/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"errors"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/transaction"

	pkgerrors "github.com/pkg/errors"
)

// TestResultOf: a transaction the time left to its block stopped is time_long when its run took
// more than 50 ms, time_short otherwise; a contract that failed and paid is penalty
func TestResultOf(t *testing.T) {
	tx := func(code pbgo.TxInvokeStatusCode) *transaction.Transaction {
		return &transaction.Transaction{OutCtx: &transaction.OutCtx{TxResult: &pbgo.TxResult{Code: code}}}
	}
	for _, c := range []struct {
		t     *transaction.Transaction
		err   error
		spent time.Duration
		want  string
	}{
		{tx(pbgo.TxInvokeStatusCode_SUCCESS), nil, time.Millisecond, resultOK},
		{&transaction.Transaction{}, nil, time.Millisecond, resultOK},
		{tx(pbgo.TxInvokeStatusCode_PENALTY), nil, time.Millisecond, resultPenalty},
		{tx(pbgo.TxInvokeStatusCode_SUCCESS), transaction.ErrTimeLeft, 60 * time.Millisecond, resultTimeLong},
		{tx(pbgo.TxInvokeStatusCode_SUCCESS), transaction.ErrTimeLeft, 50 * time.Millisecond, resultTimeShort},
		{tx(pbgo.TxInvokeStatusCode_SUCCESS), pkgerrors.WithMessage(transaction.ErrLimitStop, "txMaxFuel"), time.Millisecond, resultLimit},
		{tx(pbgo.TxInvokeStatusCode_SUCCESS), errors.New("bad signature"), time.Millisecond, resultFailed},
	} {
		if got := resultOf(c.t, c.err, c.spent); got != c.want {
			t.Errorf("resultOf(%v, %v) = %q, want %q", c.err, c.spent, got, c.want)
		}
	}
}
