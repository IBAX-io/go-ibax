/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/migration"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// blockRows is the number of rows of a block that the statements of gorm, one parameter per value,
// could not write: 7 columns of rollback_tx take more than 65535 parameters
const blockRows = 10001

func hashOf(i int) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(i))
}

// TestBlockWrites: the rows a block writes go in by one statement each, whatever their number
func TestBlockWrites(t *testing.T) {
	db := retentionDB(t)
	tx := GetDB(db)
	require.NoError(t, tx.Exec(`DELETE FROM "transactions_status"`).Error)

	rts := make([]*RollbackTx, blockRows)
	lts := make([]*LogTransaction, blockRows)
	for i := range rts {
		rts[i] = &RollbackTx{BlockID: 5, TxHash: hashOf(i), NameTable: "1_keys", TableID: "1", Data: `{"amount":"1"}`,
			DataHash: []byte{1}}
		lts[i] = &LogTransaction{Hash: hashOf(i), Block: 5, Timestamp: 1000, Address: -7, EcosystemID: 2, Status: 1,
			ContractName: "@1Transfer"}
	}
	require.NoError(t, CreateBatchesRollbackTx(tx, rts))
	require.NoError(t, CreateLogTransactionBatches(tx, lts))
	require.EqualValues(t, blockRows, countRows(t, db, `SELECT count(*) FROM "rollback_tx" WHERE "block_id" = 5`))
	require.EqualValues(t, blockRows, countRows(t, db, `SELECT count(DISTINCT "id") FROM "rollback_tx"`))
	var rt RollbackTx
	require.NoError(t, tx.Where(`"tx_hash" = ?`, hashOf(blockRows-1)).First(&rt).Error)
	require.Equal(t, RollbackTx{ID: rts[blockRows-1].ID, BlockID: 5, TxHash: hashOf(blockRows - 1), NameTable: "1_keys",
		TableID: "1", Data: `{"amount":"1"}`, DataHash: []byte{1}}, rt)
	lt := &LogTransaction{}
	found, err := lt.GetByHash(hashOf(3))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, *lts[3], *lt)

	// the outputs of block 4 are unspent; block 5 spends one of them, leaves the other as it read
	// it, and makes one it keeps and one it spends
	require.NoError(t, WriteBlockOutputs(tx, 4, []SpentInfo{
		{OutputTxHash: []byte{1}, OutputIndex: 0, OutputKeyId: 9, OutputValue: "100000000000000000000", Ecosystem: 1,
			BlockId: 4, Type: 2},
		{OutputTxHash: []byte{2}, OutputIndex: 1, OutputKeyId: 9, OutputValue: "5", Ecosystem: 1, BlockId: 4},
	}))
	// a row updated in the transaction of the test gets a new ctid
	var kept string
	require.NoError(t, tx.Raw(`SELECT ctid::text FROM "spent_info" WHERE "output_tx_hash" = '\x02'`).Row().Scan(&kept))
	require.NoError(t, WriteBlockOutputs(tx, 5, []SpentInfo{
		{InputTxHash: []byte{3}, InputIndex: 4, OutputTxHash: []byte{1}, OutputIndex: 0, OutputKeyId: 9,
			OutputValue: "100000000000000000000", Ecosystem: 1, BlockId: 4, Type: 2, SpentBlock: 5},
		{OutputTxHash: []byte{2}, OutputIndex: 1, OutputKeyId: 9, OutputValue: "5", Ecosystem: 1, BlockId: 4},
		{OutputTxHash: []byte{3}, OutputIndex: 0, OutputKeyId: 8, OutputValue: "7", Ecosystem: 1, BlockId: 5},
		{InputTxHash: []byte{4}, OutputTxHash: []byte{3}, OutputIndex: 1, OutputKeyId: 9, OutputValue: "2",
			Ecosystem: 1, BlockId: 5, SpentBlock: 5},
	}))
	var spent []SpentInfo
	require.NoError(t, tx.Order("output_tx_hash, output_index").Find(&spent).Error)
	require.Equal(t, []SpentInfo{
		{InputTxHash: []byte{3}, InputIndex: 4, OutputTxHash: []byte{1}, OutputIndex: 0, OutputKeyId: 9,
			OutputValue: "100000000000000000000", Ecosystem: 1, BlockId: 4, Type: 2, SpentBlock: 5},
		{OutputTxHash: []byte{2}, OutputIndex: 1, OutputKeyId: 9, OutputValue: "5", Ecosystem: 1, BlockId: 4},
		{OutputTxHash: []byte{3}, OutputIndex: 0, OutputKeyId: 8, OutputValue: "7", Ecosystem: 1, BlockId: 5},
		{InputTxHash: []byte{4}, OutputTxHash: []byte{3}, OutputIndex: 1, OutputKeyId: 9, OutputValue: "2",
			Ecosystem: 1, BlockId: 5, SpentBlock: 5},
	}, spent)
	require.EqualValues(t, 2, countRows(t, db, `SELECT count(*) FROM "spent_info" WHERE "input_tx_hash" IS NULL`))
	var after string
	require.NoError(t, tx.Raw(`SELECT ctid::text FROM "spent_info" WHERE "output_tx_hash" = '\x02'`).Row().Scan(&after))
	require.Equal(t, kept, after, "the output block 5 left unspent is written again")
	// the unspent outputs are in the unspent partition, the spent ones in the range of block 5
	unspentPart := fmt.Sprintf(`SELECT count(*) FROM "%s"."%s"`, migration.PartsSchema, migration.UnspentPartition)
	spentPart := fmt.Sprintf(`SELECT count(*) FROM "%s"."spent_info_p0"`, migration.PartsSchema)
	require.EqualValues(t, 2, countRows(t, db, unspentPart))
	require.EqualValues(t, 2, countRows(t, db, spentPart))
	// an output spent before, or one spent_info does not have, cannot be spent
	require.ErrorContains(t, WriteBlockOutputs(tx, 6, []SpentInfo{
		{InputTxHash: []byte{5}, OutputTxHash: []byte{1}, OutputIndex: 0, OutputKeyId: 9, OutputValue: "1",
			Ecosystem: 1, BlockId: 4, SpentBlock: 6},
		{InputTxHash: []byte{5}, OutputTxHash: []byte{2}, OutputIndex: 1, OutputKeyId: 9, OutputValue: "5",
			Ecosystem: 1, BlockId: 4, SpentBlock: 6},
	}), "block 6 spends 2 outputs, 1 of them are unspent")
	require.ErrorContains(t, WriteBlockOutputs(tx, 6, []SpentInfo{
		{InputTxHash: []byte{5}, OutputTxHash: []byte{9}, OutputIndex: 0, OutputKeyId: 9, OutputValue: "1",
			Ecosystem: 1, BlockId: 4, SpentBlock: 6},
	}), "block 6 spends 1 outputs, 0 of them are unspent")

	// the status /sendTx recorded takes the block's, and its time and type stay
	require.NoError(t, (&TransactionStatus{Hash: []byte{1}, Time: 10, Type: 3, WalletID: -5}).Create())
	require.NoError(t, UpsertBlockStatusBatches(tx, []*TransactionStatus{
		{Hash: []byte{1}, Time: 20, Type: 4, WalletID: -5, BlockID: 7, Error: "it's done", Penalty: 1},
		nil,
		{Hash: []byte{2}, Time: 11, Type: 4, WalletID: 6, BlockID: 7, Error: `{"type":"panic"}`},
	}))
	var statuses []TransactionStatus
	require.NoError(t, tx.Order("hash").Find(&statuses).Error)
	require.Equal(t, []TransactionStatus{
		{Hash: []byte{1}, Time: 10, Type: 3, WalletID: -5, BlockID: 7, Error: "it's done", Penalty: 1},
		{Hash: []byte{2}, Time: 11, Type: 4, WalletID: 6, BlockID: 7, Error: `{"type":"panic"}`},
	}, statuses)
	require.NoError(t, UpsertBlockStatusBatches(tx, []*TransactionStatus{nil}))

	// the rollback of block 5 moves the output it spent back to the unspent partition and deletes
	// those it made
	require.NoError(t, RollbackOutputs(5, db, log.NewEntry(log.StandardLogger())))
	require.EqualValues(t, 1, countRows(t, db, unspentPart+` WHERE "output_tx_hash" = '\x01'`))
	require.EqualValues(t, 0, countRows(t, db, spentPart+` WHERE "spent_block" = 5`))
	require.EqualValues(t, 0, countRows(t, db, `SELECT count(*) FROM "spent_info" WHERE "block_id" = 5`))
}

// TestBlockOutputWrites: a block inserts the outputs it made, spent or not, in the order of the
// primary key, updates the earlier ones it spent, and writes none of those it read and left
func TestBlockOutputWrites(t *testing.T) {
	made, spent := blockOutputWrites(5, []SpentInfo{
		{OutputTxHash: []byte{2}, OutputKeyId: 1, BlockId: 4},
		{OutputTxHash: []byte{9}, OutputKeyId: 1, BlockId: 5},
		{OutputTxHash: []byte{7}, OutputKeyId: 2, BlockId: 3, InputTxHash: []byte{9}, SpentBlock: 5},
		{OutputTxHash: []byte{3}, OutputKeyId: 1, BlockId: 5, InputTxHash: []byte{9}, SpentBlock: 5},
		{OutputTxHash: []byte{1}, OutputKeyId: 2, BlockId: 4, InputTxHash: []byte{9}, SpentBlock: 5},
		{OutputTxHash: []byte{3}, OutputKeyId: 1, OutputIndex: 1, BlockId: 5},
	})
	require.Equal(t, []SpentInfo{
		{OutputTxHash: []byte{3}, OutputKeyId: 1, BlockId: 5, InputTxHash: []byte{9}, SpentBlock: 5},
		{OutputTxHash: []byte{3}, OutputKeyId: 1, OutputIndex: 1, BlockId: 5},
		{OutputTxHash: []byte{9}, OutputKeyId: 1, BlockId: 5},
	}, made)
	require.Equal(t, []SpentInfo{
		{OutputTxHash: []byte{7}, OutputKeyId: 2, BlockId: 3, InputTxHash: []byte{9}, SpentBlock: 5},
		{OutputTxHash: []byte{1}, OutputKeyId: 2, BlockId: 4, InputTxHash: []byte{9}, SpentBlock: 5},
	}, spent)
}
