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
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The execution of a block of a chain of the BFT consensus leaves the validators set of the height
// BftSetDelay after it, which its execution hash covers: the header carries no hash of it. It
// needs only a database, IBAX_TEST_DSN, where it makes a schema of its own that it rolls back.
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

	block := func(id int64) *Block {
		return &Block{BlockData: &types.BlockData{Header: &types.BlockHeader{BlockId: id, Version: consts.BvOrdered}}}
	}
	// a chain without the BFT consensus has no validators set
	b := block(5)
	require.NoError(t, b.validatorsHash(dbTx))
	require.Nil(t, b.nextValidators)

	syspar.SetFirstBlockData(&types.FirstBlock{BftConsensus: 1})
	all := validatorsData(t, dbTx, 5+consts.BftSetDelay)
	b = block(5)
	require.NoError(t, b.validatorsHash(dbTx))
	require.Equal(t, all, b.nextValidators)
	require.Nil(t, b.Header.NextValidatorsHash, "an ordered block carries no validators hash")

	// an evidence stops the second node from height 9: block 6 leaves a set without it
	require.NoError(t, tx.Create(&sqldb.BftReview{ID: 1, EvidenceID: 1, PublicKey: keys[1], StopHeight: 6 + consts.BftSetDelay}).Error)
	before, after := block(5), block(6)
	require.NoError(t, before.validatorsHash(dbTx))
	require.NoError(t, after.validatorsHash(dbTx))
	require.Equal(t, all, before.nextValidators)
	require.NotEqual(t, all, after.nextValidators)
	require.Equal(t, validatorsData(t, dbTx, 6+consts.BftSetDelay), after.nextValidators)

	// block 1 leaves the set of height 2, that of the honor_nodes it writes
	genesis := block(1)
	require.NoError(t, genesis.validatorsHash(dbTx))
	require.Equal(t, validatorsData(t, dbTx, 2), genesis.nextValidators)
}

func validatorsData(t *testing.T, dbTx *sqldb.DbTransaction, height int64) []byte {
	set, err := smart.BftValidatorsOf(dbTx, height)
	require.NoError(t, err)
	return set.Marshal()
}
