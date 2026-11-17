/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/hex"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestGetTxStatus: /txstatus answers for a transaction of a block from log_transactions, its
// result, or its penalty with the error of it; a refusal of the pool after the block does not hide
// the block; a transaction no block took has the refusal of the pool
func TestGetTxStatus(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	db := conn.Begin()
	require.NoError(t, db.Error)
	savedDB, savedPool := sqldb.DBConn, transaction.TxPool
	sqldb.DBConn, transaction.TxPool = db, transaction.NewPool()
	t.Cleanup(func() {
		sqldb.DBConn, transaction.TxPool = savedDB, savedPool
		db.Rollback()
	})
	conf.Config.Storage.Archive = false
	require.NoError(t, db.Exec(`DELETE FROM "log_transactions"`).Error)
	require.NoError(t, db.Create(&sqldb.LogTransaction{Hash: []byte("ok"), Block: 7, Result: "42"}).Error)
	require.NoError(t, db.Create(&sqldb.LogTransaction{Hash: []byte("penalty"), Block: 7, Status: 1, Penalty: 1,
		Result: `{"type":"panic","error":"no funds"}`}).Error)
	require.NoError(t, db.Create(&sqldb.LogTransaction{Hash: []byte("text"), Block: 8, Status: 1, Penalty: 1,
		Result: "not json"}).Error)
	transaction.TxPool.Bad([]byte("ok"), "duplicate")
	transaction.TxPool.Bad([]byte("refused"), `{"type":"error","error":"bad"}`)

	r := setLogger(httptest.NewRequest("GET", "/txstatus", nil), log.NewEntry(log.StandardLogger()))
	for hash, want := range map[string]txstatusResult{
		"ok":      {BlockID: "7", Result: "42"},
		"penalty": {BlockID: "7", Penalty: 1, Message: &txstatusError{Type: "panic", Error: "no funds"}},
		"text":    {BlockID: "8", Penalty: 1, Message: &txstatusError{Type: "txError", Error: "not json"}},
		"refused": {Message: &txstatusError{Type: "error", Error: "bad"}},
	} {
		got, err := getTxStatus(r, hex.EncodeToString([]byte(hash)))
		require.NoError(t, err, hash)
		require.Equal(t, want, *got, hash)
	}
	_, err = getTxStatus(r, hex.EncodeToString([]byte("none")))
	require.Error(t, err)
}
