/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package rollback

import (
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestFeeSettlement: the settlement of a block credits every account once with the sum of its
// fees, in the order of the ecosystem and the account, makes the accounts that are missing, and
// the rollback of the block takes all of it back
func TestFeeSettlement(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	tx := conn.Begin()
	require.NoError(t, tx.Error)
	saved := sqldb.DBConn
	sqldb.DBConn = tx
	t.Cleanup(func() {
		sqldb.DBConn = saved
		tx.Rollback()
	})
	dbTx := sqldb.NewDbTransaction(tx)
	crypto.InitHashAlgo("SHA512")
	require.NoError(t, syspar.SysTableColType(dbTx))

	const (
		blockID  = int64(19998)
		producer = int64(-1000001)
		taxes    = int64(-1000002)
	)
	for _, id := range []int64{producer, taxes} {
		require.NoError(t, tx.Exec(`DELETE FROM "1_keys" WHERE id = ?`, id).Error)
	}
	require.NoError(t, tx.Exec(`INSERT INTO "1_keys" (id, account, ecosystem, amount) VALUES (?, 'p', 1, 100)`, producer).Error)
	balance := func(eco, id int64) (string, bool) {
		k := &sqldb.Key{}
		found, err := k.SetTablePrefix(eco).Get(dbTx, id)
		require.NoError(t, err)
		return k.Amount, found
	}

	fees := smart.NewFeeSettlement()
	first := []smart.FeeCredit{
		{Ecosystem: 2, Recipient: producer, Amount: decimal.NewFromInt(7)},
		{Ecosystem: 1, Recipient: producer, Amount: decimal.NewFromInt(10)},
		{Ecosystem: 1, Recipient: taxes, Amount: decimal.NewFromInt(3)},
	}
	require.Equal(t, 3, fees.NewAccounts(first))
	fees.Add(first)
	second := []smart.FeeCredit{{Ecosystem: 1, Recipient: producer, Amount: decimal.NewFromInt(5)}}
	require.Equal(t, 0, fees.NewAccounts(second))
	fees.Add(second)

	rts, err := fees.Settle(dbTx, &types.BlockHeader{BlockId: blockID, Timestamp: 1700000000})
	require.NoError(t, err)

	// (1, taxes) and (2, producer) make the key; every account has its credit and history row
	require.Len(t, rts, 8)
	max := smart.SettlementRowMax().Size()
	for _, r := range rts {
		require.Equal(t, smart.SettlementHash, r.TxHash)
		require.LessOrEqual(t, r.Size(), max, "%+v", r)
	}
	for _, c := range []struct {
		eco, id int64
		amount  string
	}{{1, producer, "115"}, {1, taxes, "3"}, {2, producer, "7"}} {
		amount, found := balance(c.eco, c.id)
		require.True(t, found)
		require.Equal(t, c.amount, amount, "ecosystem %d account %d", c.eco, c.id)
	}
	_, found := balance(1, taxes)
	require.True(t, found, "the account in ecosystem 1")

	var history []struct {
		Ecosystem        int64
		RecipientID      int64
		RecipientBalance string
		Amount           string
		Type             int64
	}
	require.NoError(t, tx.Raw(`SELECT ecosystem, recipient_id, recipient_balance, amount, type FROM "1_history"
		WHERE block_id = ? AND txhash = ? ORDER BY id`, blockID, smart.SettlementHash).Scan(&history).Error)
	require.Len(t, history, 3)
	for i, want := range []struct {
		eco, id         int64
		balance, amount string
	}{{1, taxes, "3", "3"}, {1, producer, "115", "15"}, {2, producer, "7", "7"}} {
		h := history[i]
		require.Equal(t, want.eco, h.Ecosystem)
		require.Equal(t, want.id, h.RecipientID)
		require.Equal(t, want.balance, h.RecipientBalance)
		require.Equal(t, want.amount, h.Amount)
		require.EqualValues(t, smart.GasScenesType_Settlement, h.Type)
	}

	// the rollback of the block
	rows := make([]*sqldb.RollbackTx, len(rts))
	for i, r := range rts {
		rows[i] = &sqldb.RollbackTx{BlockID: r.BlockId, TxHash: r.TxHash, NameTable: r.NameTable, TableID: r.TableId,
			Data: r.Data, DataHash: r.DataHash}
	}
	require.NoError(t, sqldb.CreateBatchesRollbackTx(tx, rows))
	require.NoError(t, rollbackTransaction(blockID, smart.SettlementHash, dbTx, log.NewEntry(log.StandardLogger())))
	amount, found := balance(1, producer)
	require.True(t, found)
	require.Equal(t, "100", amount)
	_, found = balance(1, taxes)
	require.False(t, found)
	_, found = balance(2, producer)
	require.False(t, found)
	var n int64
	require.NoError(t, tx.Raw(`SELECT count(*) FROM "1_history" WHERE block_id = ? AND txhash = ?`, blockID,
		smart.SettlementHash).Scan(&n).Error)
	require.Zero(t, n)
	require.NoError(t, tx.Raw(`SELECT count(*) FROM rollback_tx WHERE block_id = ? AND tx_hash = ?`, blockID,
		smart.SettlementHash).Scan(&n).Error)
	require.Zero(t, n)
}
