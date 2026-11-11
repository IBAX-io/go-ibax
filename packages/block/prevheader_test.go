/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The header of a stored block is the one its whole data has, with the hashes of its row, whether
// the header is in the first bytes the node reads or is longer and is read again to its end
func TestHeaderFromBlockChain(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	tx := conn.Begin()
	require.NoError(t, tx.Error)
	saved := sqldb.DBConn
	sqldb.DBConn = tx
	t.Cleanup(func() {
		sqldb.DBConn = saved
		tx.Rollback()
	})
	require.NoError(t, tx.Exec(`CREATE SCHEMA prevheader_test; SET LOCAL search_path TO prevheader_test`).Error)
	require.NoError(t, tx.AutoMigrate(&sqldb.BlockChain{}))

	txs := make([][]byte, 200)
	for i := range txs {
		txs[i] = bytes.Repeat([]byte{byte(i)}, 4000)
	}
	for id, candidates := range map[int64][]byte{2: []byte("[]"), 3: bytes.Repeat([]byte{7}, headPrefix)} {
		header := &types.BlockHeader{BlockId: id, Timestamp: 3, KeyId: 4, NodePosition: 1, NetworkId: 9,
			Version: consts.BvValidatorsHash, Sign: make([]byte, 3309), BlockHash: []byte{0xaa}, RollbacksHash: []byte{0xbb},
			CandidateNodes: candidates, NextValidatorsHash: []byte{0xcc}, StateHash: []byte{0xdd}}
		data, err := (&types.BlockData{Header: header, PrevHeader: &types.BlockHeader{BlockId: id - 1}, TxFullData: txs}).Marshal()
		require.NoError(t, err)
		require.NoError(t, tx.Create(&sqldb.BlockChain{ID: id, Hash: []byte{byte(id)}, RollbacksHash: []byte{0xbe},
			Data: data, CandidateNodes: []byte{0}}).Error)

		full := &types.BlockData{}
		require.NoError(t, full.UnmarshallBlock(data))
		want := full.Header
		want.BlockHash, want.RollbacksHash = []byte{byte(id)}, []byte{0xbe}
		got, err := GetBlockHeaderFromBlockChain(id)
		require.NoError(t, err)
		require.Equal(t, want, got, "block %d", id)
	}
	_, err = GetBlockHeaderFromBlockChain(4)
	require.Error(t, err)
}
