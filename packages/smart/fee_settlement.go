/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
)

// SettlementHash is the transaction hash of the rows the settlement of the fees of a block writes:
// the rollback of the block takes them back before the rollback of its transactions
var SettlementHash = make([]byte, 32)

// FeeCredit is a fee that a transaction pays to an account of an ecosystem
type FeeCredit struct {
	Ecosystem int64
	Recipient int64
	Amount    decimal.Decimal
}

type feeAccount struct{ ecosystem, recipient int64 }

// FeeSettlement sums the fees the transactions of a block pay to every account, and credits every
// account once, after the last transaction. Within the block the transactions see the balances of
// the accounts they pay as at the start of the block.
type FeeSettlement struct {
	sums map[feeAccount]decimal.Decimal
}

func NewFeeSettlement() *FeeSettlement {
	return &FeeSettlement{sums: make(map[feeAccount]decimal.Decimal)}
}

// Add adds the credits of a transaction that the block keeps
func (s *FeeSettlement) Add(credits []FeeCredit) {
	for _, c := range credits {
		a := feeAccount{c.Ecosystem, c.Recipient}
		s.sums[a] = s.sums[a].Add(c.Amount)
	}
}

// Settle credits every account, in the order of the ecosystem and the account: it makes the
// account where the ecosystem or ecosystem 1 has none, adds the sum to its balance and writes a
// history row with the balance. It returns the rollback rows of the writes.
func (s *FeeSettlement) Settle(dbTx *sqldb.DbTransaction, header *types.BlockHeader) ([]*types.RollbackTx, error) {
	if len(s.sums) == 0 {
		return nil, nil
	}
	accounts := make([]feeAccount, 0, len(s.sums))
	for a := range s.sums {
		accounts = append(accounts, a)
	}
	slices.SortFunc(accounts, func(a, b feeAccount) int {
		return cmp.Or(cmp.Compare(a.ecosystem, b.ecosystem), cmp.Compare(a.recipient, b.recipient))
	})
	sc := &SmartContract{
		Rollback:      true,
		FullAccess:    true,
		TxSmart:       &types.SmartTransaction{Header: &types.Header{EcosystemID: 1}},
		BlockHeader:   header,
		Hash:          SettlementHash,
		Timestamp:     header.Timestamp * 1000,
		DbTransaction: dbTx,
		Key:           &sqldb.Key{},
	}
	comment := fmt.Sprintf("fees of block %d", header.BlockId)
	for _, a := range accounts {
		sum := s.sums[a]
		if err := sc.hasExistKeyID(a.ecosystem, a.recipient); err != nil {
			return nil, err
		}
		if _, _, err := sc.updateWhere([]string{"+amount"}, []any{sum}, "1_keys",
			types.LoadMap(map[string]any{"id": a.recipient, "ecosystem": a.ecosystem})); err != nil {
			return nil, err
		}
		balance, err := sc.accountBalanceSingle(a.ecosystem, a.recipient)
		if err != nil {
			return nil, err
		}
		values := types.LoadMap(map[string]any{
			"sender_id":         a.recipient,
			"sender_balance":    balance,
			"recipient_id":      a.recipient,
			"recipient_balance": balance,
			"amount":            sum,
			"comment":           comment,
			"status":            int64(pbgo.TxInvokeStatusCode_SUCCESS),
			"block_id":          header.BlockId,
			"txhash":            SettlementHash,
			"ecosystem":         a.ecosystem,
			"type":              int64(GasScenesType_Settlement),
			"created_at":        sc.Timestamp,
		})
		if _, _, err = sc.insert(values.Keys(), values.Values(), `1_history`); err != nil {
			return nil, err
		}
	}
	return sc.RollBackTx, nil
}
