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

	"github.com/IBAX-io/go-ibax/packages/bft"
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
	tx, keys := validatorsDB(t, "validators_hash", 3)
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

// validatorsDB is a transaction it rolls back on the database IBAX_TEST_DSN, DBConn meanwhile, in a
// schema of its own with honor_nodes of n nodes, whose keys it returns, the reviews of the evidence
// and the validators sets; the chain is not one of the BFT consensus
func validatorsDB(t *testing.T, schema string, n int) (*gorm.DB, [][]byte) {
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
	require.NoError(t, tx.Exec(`CREATE SCHEMA `+schema+`; SET LOCAL search_path TO `+schema).Error)
	require.NoError(t, tx.AutoMigrate(&sqldb.PlatformParameter{}, &sqldb.BftReview{}, &sqldb.BftValidatorSet{}))
	useSuite("ECC_P256", "SHA256")
	keys := make([][]byte, n)
	for i := range keys {
		_, pub, err := crypto.GenNodeKeyPair()
		require.NoError(t, err)
		keys[i] = pub
	}
	require.NoError(t, tx.Create(&sqldb.PlatformParameter{ID: 1, Name: syspar.HonorNodes, Value: honorNodes(t, keys)}).Error)
	return tx, keys
}

// honorNodes is the value of honor_nodes of the nodes of the keys
func honorNodes(t *testing.T, keys [][]byte) string {
	var nodes []map[string]any
	for i, k := range keys {
		nodes = append(nodes, map[string]any{"tcp_address": fmt.Sprintf("127.0.0.1:%d", 7078+2*i),
			"api_address": fmt.Sprintf("http://127.0.0.1:%d", 7079+2*i), "public_key": crypto.PubToHex(k), "unban_time": "0"})
	}
	value, err := json.Marshal(nodes)
	require.NoError(t, err)
	return string(value)
}

func validatorsData(t *testing.T, dbTx *sqldb.DbTransaction, height int64) []byte {
	set, err := smart.BftValidatorsOf(dbTx, height)
	require.NoError(t, err)
	return set.Marshal()
}

// A change of the validators set takes effect BftSetDelay heights after the block whose execution
// makes it, on every node, as the execution of each block stores the set it leaves: honor_nodes a
// vote changes in block 6 is the set of height 9, the stop of an evidence in block 7 that of 10, the
// restore a review concludes in block 11 that of 14. The heights between keep the set before, and
// the signer of the header of a block is resolved in the set of its height.
func TestSetChangeAtDelay(t *testing.T) {
	tx, keys := validatorsDB(t, "set_change_delay", 3)
	dbTx := sqldb.NewDbTransaction(tx)
	syspar.SetFirstBlockData(&types.FirstBlock{BftConsensus: 1})
	_, added, err := crypto.GenNodeKeyPair()
	require.NoError(t, err)

	execute := func(id int64, change func()) {
		if change != nil {
			change()
		}
		b := &Block{BlockData: &types.BlockData{Header: &types.BlockHeader{BlockId: id, Version: consts.BvOrdered}}}
		require.NoError(t, b.validatorsHash(dbTx), "block %d", id)
		require.NoError(t, b.storeValidators(dbTx), "block %d", id)
	}
	for id := int64(1); id <= 12; id++ {
		switch id {
		case 6:
			execute(id, func() {
				require.NoError(t, tx.Model(&sqldb.PlatformParameter{}).Where("name = ?", syspar.HonorNodes).
					Update("value", honorNodes(t, append(append([][]byte{}, keys...), added))).Error)
			})
		case 7:
			execute(id, func() {
				require.NoError(t, tx.Create(&sqldb.BftReview{ID: 1, EvidenceID: 1, PublicKey: keys[0],
					StopHeight: id + consts.BftSetDelay}).Error)
			})
		case 11:
			execute(id, func() {
				require.NoError(t, tx.Model(&sqldb.BftReview{}).Where("id = 1").
					Update("restore_height", id+consts.BftSetDelay).Error)
			})
		default:
			execute(id, nil)
		}
	}

	all := append(append([][]byte{}, keys...), added)
	want := func(height int64) [][]byte {
		switch {
		case height < 9:
			return keys
		case height < 10, height >= 14:
			return all
		}
		return all[1:]
	}
	for height := int64(2); height <= 15; height++ {
		s := &sqldb.BftValidatorSet{}
		found, err := s.GetAt(dbTx, height)
		require.NoError(t, err)
		require.True(t, found, "height %d", height)
		set, err := bft.UnmarshalValidators(s.Data)
		require.NoError(t, err)
		require.Equal(t, want(height), set.Keys, "the set of height %d", height)
		for i, k := range want(height) {
			got, err := validatorAt(dbTx, height, int64(i))
			require.NoError(t, err)
			require.Equal(t, k, got, "the signer at %d of height %d", i, height)
		}
	}
	var changes int64
	require.NoError(t, tx.Model(&sqldb.BftValidatorSet{}).Count(&changes).Error)
	require.EqualValues(t, 4, changes, "a set is stored at 2 and at each change: 9, 10, 14")
}
