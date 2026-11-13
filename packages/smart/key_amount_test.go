/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// keyAmountTx is a transaction of the pool of the blocks, with its cache, on the database of
// IBAX_TEST_DSN
func keyAmountTx(t *testing.T) *sqldb.DbTransaction {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	pc, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	saved := sqldb.DBConn
	require.NoError(t, sqldb.GormInit(conf.DBConfig{Host: pc.Host, Port: int(pc.Port), User: pc.User, Name: pc.Database,
		Password: pc.Password, LockTimeout: 5000, IdleInTxTimeout: 5000, MaxIdleConns: 1, MaxOpenConns: 2}))
	tr, err := sqldb.StartTransaction()
	require.NoError(t, err)
	t.Cleanup(func() {
		tr.Rollback()
		sqldb.GormClose()
		sqldb.DBConn = saved
	})
	return tr
}

// keyAmountRun is what a write of the amount of a key did
type keyAmountRun struct {
	balance  decimal.Decimal
	err      error
	rollback []*types.RollbackTx
	row      string
	history  string
	after    decimal.Decimal
}

// TestUpdateKeyAmount: the balance updateKeyAmount tells, and payTaxes writes into the history, is
// the balance a read after the update gives, and the rows, the rollback records and the history
// rows are those of the update and the read; a read after the update, or after a rollback to a
// savepoint that takes it back, gives the amount of the database, though the key was read before
func TestUpdateKeyAmount(t *testing.T) {
	tr := keyAmountTx(t)
	InitVM()
	crypto.InitHashAlgo(crypto.HashAlgo_SHA256.String())
	const (
		eco     = int64(1)
		wallet  = int64(-7331001)
		taxes   = int64(-7331002)
		blockID = int64(9999)
	)
	require.NoError(t, tr.ExecSql(`DELETE FROM "1_keys" WHERE id IN (?, ?)`, wallet, taxes))
	require.NoError(t, tr.ExecSql(`INSERT INTO "1_keys" (id, pub, amount, ecosystem, account) VALUES (?, '\x01', 1000, ?, '0000-0000-0000-0000-7331')`,
		wallet, eco))
	newSC := func() *SmartContract {
		return &SmartContract{
			DbTransaction: tr,
			Key:           &sqldb.Key{},
			Rollback:      true,
			Hash:          []byte("b1-key-amount"),
			Timestamp:     1700000000,
			BlockHeader:   &types.BlockHeader{BlockId: blockID},
			TxContract:    &Contract{Name: "@1Test"},
			TxSmart:       &types.SmartTransaction{Header: &types.Header{EcosystemID: eco}},
		}
	}
	where := func() *types.Map { return types.LoadMap(map[string]any{"id": wallet, "ecosystem": eco}) }
	// fresh is the balance as a read of the database gives it
	fresh := func(sc *SmartContract) (decimal.Decimal, error) {
		key := (&sqldb.Key{}).SetTablePrefix(eco)
		if _, err := key.Get(sc.DbTransaction, wallet); err != nil {
			return decimal.Zero, err
		}
		balance, _ := decimal.NewFromString(key.Amount)
		return balance, nil
	}
	run := func(f func(sc *SmartContract) (decimal.Decimal, error)) keyAmountRun {
		t.Helper()
		require.NoError(t, tr.Savepoint(`"b1"`))
		sc := newSC()
		// the key is read before it is written, as the checks of the fees read it
		require.NoError(t, sc.hasExistKeyID(eco, wallet))
		var r keyAmountRun
		r.balance, r.err = f(sc)
		r.rollback = sc.RollBackTx
		var err error
		if r.err != nil {
			// the statement failed: the transaction takes nothing before the rollback
			require.NoError(t, tr.RollbackSavepoint(`"b1"`))
			r.after, err = sc.accountBalanceSingle(eco, wallet)
			require.NoError(t, err)
			require.Equal(t, "1000", r.after.String(), "the read after the rollback")
			return r
		}
		r.row, err = tr.Single(`SELECT to_jsonb(t)::text FROM "1_keys" t WHERE id = ? AND ecosystem = ?`, wallet, eco).String()
		require.NoError(t, err)
		r.history, err = tr.Single(`SELECT coalesce(jsonb_agg(to_jsonb(h) ORDER BY id), '[]')::text FROM "1_history" h
			WHERE block_id = ? AND txhash = ?`, blockID, sc.Hash).String()
		require.NoError(t, err)
		r.after, err = sc.accountBalanceSingle(eco, wallet)
		require.NoError(t, err)
		require.NoError(t, tr.RollbackSavepoint(`"b1"`))
		back, err := sc.accountBalanceSingle(eco, wallet)
		require.NoError(t, err)
		require.Equal(t, "1000", back.String(), "the read after the rollback")
		return r
	}
	same := func(name string, want, got keyAmountRun) {
		t.Helper()
		require.Equal(t, want.err, got.err, name)
		if got.err != nil {
			require.Equal(t, want.rollback, got.rollback, name)
			return
		}
		require.Equal(t, want.balance.String(), got.balance.String(), name)
		require.True(t, want.balance.Equal(got.balance), name)
		require.Equal(t, want.rollback, got.rollback, name)
		require.Equal(t, want.row, got.row, name)
		require.Equal(t, want.history, got.history, name)
		require.Equal(t, want.after.String(), got.after.String(), name)
		require.Equal(t, got.balance.String(), got.after.String(), "%s: the read after the update", name)
	}
	for _, c := range []struct{ sign, sum string }{
		{"-", "7"}, {"+", "7"}, {"-", "7.000"}, {"+", "12345678901234567890"}, {"-", "1000"},
		{"-", "2.5"}, {"+", "0.4"}, {"-", "1001"},
	} {
		sum := decimal.RequireFromString(c.sum)
		want := run(func(sc *SmartContract) (decimal.Decimal, error) {
			if _, _, err := sc.updateWhere([]string{c.sign + "amount"}, []any{sum}, "1_keys", where()); err != nil {
				return decimal.Zero, err
			}
			return fresh(sc)
		})
		got := run(func(sc *SmartContract) (decimal.Decimal, error) {
			return sc.updateKeyAmount(c.sign, sum, eco, wallet, where())
		})
		same(c.sign+c.sum, want, got)
	}

	pay := &PaymentInfo{TokenEco: eco, FromID: wallet, TaxesID: taxes}
	for _, s := range []string{"7", "7.000", "2.5"} {
		sum := decimal.RequireFromString(s)
		want := run(func(sc *SmartContract) (decimal.Decimal, error) {
			// payTaxes as it read the balance after the debit
			if _, _, err := sc.updateWhere([]string{`-amount`}, []any{sum}, "1_keys", where()); err != nil {
				return decimal.Zero, err
			}
			balance, err := fresh(sc)
			if err != nil {
				return balance, err
			}
			values := types.LoadMap(map[string]any{
				"sender_id":      wallet,
				"sender_balance": balance,
				"recipient_id":   taxes,
				"amount":         sum,
				"comment":        "taxes",
				"status":         int64(pbgo.TxInvokeStatusCode_SUCCESS),
				"block_id":       blockID,
				"txhash":         sc.Hash,
				"ecosystem":      eco,
				"type":           int64(GasScenesType_Taxes),
				"created_at":     sc.Timestamp,
			})
			_, _, err = sc.insert(values.Keys(), values.Values(), `1_history`)
			return balance, err
		})
		got := run(func(sc *SmartContract) (decimal.Decimal, error) {
			err := sc.payTaxes(pay, sum, GasScenesType_Taxes, "taxes", pbgo.TxInvokeStatusCode_SUCCESS)
			return decimal.Zero, err
		})
		got.balance = want.balance
		require.NotEqual(t, "[]", got.history, s)
		same("payTaxes "+s, want, got)
	}
}
