/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// A generated block of a chain of the BFT consensus puts in its header the hash of the validators
// set of the next height, and the block of another node is valid only with the hash of the set
// its execution leaves; it needs only a database, IBAX_TEST_DSN, where it makes a schema of its
// own that it rolls back
func TestValidatorsHash(t *testing.T) {
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
		syspar.SetFirstBlockData(nil)
	})
	require.NoError(t, tx.Exec(`CREATE SCHEMA validators_hash; SET LOCAL search_path TO validators_hash`).Error)
	require.NoError(t, tx.AutoMigrate(&sqldb.PlatformParameter{}, &sqldb.BftReview{}))
	useSuite("ECC_P256", "SHA256")
	var nodes []map[string]any
	keys := make([][]byte, 3)
	for i := range keys {
		_, pub, err := crypto.GenNodeKeyPair()
		require.NoError(t, err)
		keys[i] = pub
		nodes = append(nodes, map[string]any{"tcp_address": fmt.Sprintf("127.0.0.1:%d", 7078+i),
			"api_address": fmt.Sprintf("http://127.0.0.1:%d", 7079+i), "public_key": crypto.PubToHex(pub), "unban_time": "0"})
	}
	value, err := json.Marshal(nodes)
	require.NoError(t, err)
	require.NoError(t, tx.Create(&sqldb.PlatformParameter{ID: 1, Name: syspar.HonorNodes, Value: string(value)}).Error)
	dbTx := sqldb.NewDbTransaction(tx)

	block := func(id int64, gen bool) *Block {
		return &Block{BlockData: &types.BlockData{Header: &types.BlockHeader{BlockId: id}}, GenBlock: gen}
	}
	// a chain without the BFT consensus has no validators set
	b := block(5, true)
	require.NoError(t, b.validatorsHash(dbTx))
	require.Nil(t, b.Header.NextValidatorsHash)
	require.Nil(t, b.nextValidators)

	syspar.SetFirstBlockData(&types.FirstBlock{BftConsensus: 1})
	all := crypto.Hash(validatorsData(t, dbTx, 6))
	gen := block(5, true)
	gen.Header.StateHash = []byte{1}
	require.NoError(t, gen.validatorsHash(dbTx))
	require.Equal(t, all, gen.Header.NextValidatorsHash)
	require.Nil(t, gen.Header.StateHash)

	other := block(5, false)
	other.Header.NextValidatorsHash = gen.Header.NextValidatorsHash
	require.NoError(t, other.validatorsHash(dbTx))
	other.Header.NextValidatorsHash = crypto.Hash([]byte("another set"))
	require.ErrorContains(t, other.validatorsHash(dbTx), "the validators hash")
	other.Header.NextValidatorsHash = nil
	require.ErrorContains(t, other.validatorsHash(dbTx), "the validators hash")
	other.Header.NextValidatorsHash, other.Header.StateHash = all, []byte{1}
	require.ErrorContains(t, other.validatorsHash(dbTx), "state hash")

	// an evidence stops the second node from height 7: block 6 leaves a set without it
	require.NoError(t, tx.Create(&sqldb.BftReview{ID: 1, EvidenceID: 1, PublicKey: keys[1], StopHeight: 7}).Error)
	before, after := block(5, true), block(6, true)
	require.NoError(t, before.validatorsHash(dbTx))
	require.NoError(t, after.validatorsHash(dbTx))
	require.Equal(t, all, before.Header.NextValidatorsHash)
	require.NotEqual(t, all, after.Header.NextValidatorsHash)
	require.Equal(t, crypto.Hash(validatorsData(t, dbTx, 7)), after.Header.NextValidatorsHash)

	// block 1 carries no hash, but keeps the set for storeValidators
	genesis := block(1, false)
	require.NoError(t, genesis.validatorsHash(dbTx))
	require.Nil(t, genesis.Header.NextValidatorsHash)
	require.Equal(t, validatorsData(t, dbTx, 2), genesis.nextValidators)
}

func validatorsData(t *testing.T, dbTx *sqldb.DbTransaction, height int64) []byte {
	set, err := smart.BftValidatorsOf(dbTx, height)
	require.NoError(t, err)
	return set.Marshal()
}

// The shell of a streaming proposal is without the hashes its trial sets: the validators hash and
// the state hash
func TestShellWithoutValidatorsHash(t *testing.T) {
	b := &Block{BlockData: &types.BlockData{Header: &types.BlockHeader{BlockId: 5, Version: 4}}}
	bare, err := b.shell()
	require.NoError(t, err)
	b.Header.NextValidatorsHash, b.Header.StateHash = []byte{1}, []byte{2}
	shell, err := b.shell()
	require.NoError(t, err)
	require.Equal(t, bare, shell)
	require.Equal(t, []byte{1}, b.Header.NextValidatorsHash)
}
