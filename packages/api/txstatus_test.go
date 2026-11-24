/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// orderedChainDB is a database transaction of IBAX_TEST_DSN, rolled back at the end of the test, as
// sqldb.DBConn, with an empty pool and window, and an ordered chain: blocks 7 and 8 executed, with
// the logs of their transactions, and block 9 committed and not executed, with the transaction
// "pending"
func orderedChainDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	db := conn.Begin()
	require.NoError(t, db.Error)
	savedDB, savedPool, savedParse := sqldb.DBConn, transaction.TxPool, sqldb.ParseWindowTxs
	sqldb.DBConn, transaction.TxPool = db, transaction.NewPool()
	sqldb.ParseWindowTxs = func(data []byte) ([]sqldb.WindowTx, error) {
		return []sqldb.WindowTx{{Hash: data, Signed: 109}}, nil
	}
	sqldb.ResetTxWindow()
	t.Cleanup(func() {
		sqldb.DBConn, transaction.TxPool, sqldb.ParseWindowTxs = savedDB, savedPool, savedParse
		sqldb.ResetTxWindow()
		db.Rollback()
	})
	conf.Config.Storage.Archive = false
	for _, table := range []string{"log_transactions", "block_chain", "info_block"} {
		require.NoError(t, db.Exec(`DELETE FROM "`+table+`"`).Error)
	}
	for _, id := range []int64{7, 8, 9} {
		require.NoError(t, db.Create(&sqldb.BlockChain{ID: id, Hash: []byte{byte(id)}, Data: []byte("pending"),
			Time: 100 + id, RollbacksHash: []byte{}}).Error)
	}
	require.NoError(t, db.Create(&sqldb.InfoBlock{BlockID: 8, Hash: []byte{8}, NodePosition: "0", RollbacksHash: []byte{}, CandidateNodes: []byte{}}).Error)
	return db
}

// TestOfflineGetTxStatus: /txstatus answers for a transaction of an executed block from log_transactions,
// its result, its penalty with the error of it, or its discard with the reason; for one of a block
// committed and not executed, pending; a refusal of the pool after the block does not hide the
// block; a transaction no block took has the refusal of the pool
func TestOfflineGetTxStatus(t *testing.T) {
	db := orderedChainDB(t)
	require.NoError(t, db.Create(&sqldb.LogTransaction{Hash: []byte("ok"), Block: 7, Result: "42"}).Error)
	require.NoError(t, db.Create(&sqldb.LogTransaction{Hash: []byte("penalty"), Block: 7, Status: 1, Penalty: 1,
		Result: `{"type":"panic","error":"no funds"}`}).Error)
	require.NoError(t, db.Create(&sqldb.LogTransaction{Hash: []byte("text"), Block: 8, Status: 1, Penalty: 1,
		Result: "not json"}).Error)
	require.NoError(t, db.Create(&sqldb.LogTransaction{Hash: []byte("discarded"), Block: 8,
		Status: int64(pbgo.TxInvokeStatusCode_DISCARDED), Result: "the fuel of the block"}).Error)
	transaction.TxPool.Bad([]byte("ok"), "duplicate")
	transaction.TxPool.Bad([]byte("pending"), "duplicate")
	transaction.TxPool.Bad([]byte("refused"), `{"type":"error","error":"bad"}`)

	r := setLogger(httptest.NewRequest("GET", "/txstatus", nil), log.NewEntry(log.StandardLogger()))
	for hash, want := range map[string]txstatusResult{
		"ok":        {Status: "executed", BlockID: "7", Result: "42"},
		"penalty":   {Status: "penalty", BlockID: "7", Penalty: 1, Message: &txstatusError{Type: "panic", Error: "no funds"}},
		"text":      {Status: "penalty", BlockID: "8", Penalty: 1, Message: &txstatusError{Type: "txError", Error: "not json"}},
		"discarded": {Status: "discarded", BlockID: "8", Message: &txstatusError{Type: "discarded", Error: "the fuel of the block"}},
		"pending":   {Status: "pending"},
		"refused":   {Status: "refused", Message: &txstatusError{Type: "error", Error: "bad"}},
	} {
		got, err := getTxStatus(r, hex.EncodeToString([]byte(hash)))
		require.NoError(t, err, hash)
		require.Equal(t, want, *got, hash)
	}
	_, err := getTxStatus(r, hex.EncodeToString([]byte("none")))
	require.Error(t, err)
}

// TestOfflineGetMaxBlock: /maxblockid answers the executed height, whose state the reads see, and the
// committed height ahead of it
func TestOfflineGetMaxBlock(t *testing.T) {
	orderedChainDB(t)
	w := httptest.NewRecorder()
	getMaxBlockHandler(w, setLogger(httptest.NewRequest("GET", "/maxblockid", nil), log.NewEntry(log.StandardLogger())))
	require.Equal(t, 200, w.Code, w.Body.String())
	var got maxBlockResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, maxBlockResult{MaxBlockID: 8, CommittedBlockID: 9}, got)
}
