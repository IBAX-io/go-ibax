/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"errors"
	"time"

	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"

	pkgerrors "github.com/pkg/errors"
)

// longRun is the time past which a run the time left to its block stopped took long by itself,
// rather than met a block with little time left
const longRun = 50 * time.Millisecond

// Results of a transaction in ContractResults
const (
	resultOK        = "ok"
	resultPenalty   = "penalty"
	resultFailed    = "failed"
	resultTimeLong  = "time_long"
	resultTimeShort = "time_short"
	resultLimit     = "limit"
)

// observeRun records by its contract a run of the transaction t that took spent, with its
// statements those of stats
func observeRun(t *transaction.Transaction, spent time.Duration, stats sqldb.TxStats) {
	contract := contractOf(t)
	if contract == "" {
		return
	}
	state := min(stats.Time, spent)
	metrics.ContractTime.WithLabelValues(contract, "state").Observe(state.Seconds())
	metrics.ContractTime.WithLabelValues(contract, "other").Observe((spent - state).Seconds())
	metrics.ContractFuel.WithLabelValues(contract).Observe(float64(t.SmartContract().TxFuel))
	metrics.ContractStatements.WithLabelValues(contract).Observe(float64(stats.Statements))
	metrics.ContractRows.WithLabelValues(contract, "read").Add(float64(stats.Read))
	metrics.ContractRows.WithLabelValues(contract, "written").Add(float64(stats.Written))
}

// observeResult counts by its contract the result of the transaction t of the block, err the error
// of the run the block took, which took spent
func observeResult(t *transaction.Transaction, err error, spent time.Duration) {
	if contract := contractOf(t); contract != "" {
		metrics.ContractResults.WithLabelValues(contract, resultOf(t, err, spent)).Inc()
	}
}

// resultOf is the result of the transaction t of the block in ContractResults
func resultOf(t *transaction.Transaction, err error, spent time.Duration) string {
	switch {
	case err == nil && t.OutCtx != nil && t.TxResult != nil && t.TxResult.Code == pbgo.TxInvokeStatusCode_PENALTY:
		return resultPenalty
	case err == nil:
		return resultOK
	case errors.Is(err, transaction.ErrTimeLeft) && spent > longRun:
		return resultTimeLong
	case errors.Is(err, transaction.ErrTimeLeft):
		return resultTimeShort
	case pkgerrors.Cause(err) == transaction.ErrLimitStop:
		return resultLimit
	}
	return resultFailed
}
