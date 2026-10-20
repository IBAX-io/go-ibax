/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/rollback"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/gogo/protobuf/proto"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// tamper is the block of binData with a change that keeps the hash it carries
func tamper(t *testing.T, binData []byte, change func(*types.BlockData)) []byte {
	t.Helper()
	raw := &types.BlockData{}
	require.NoError(t, proto.Unmarshal(binData, raw))
	change(raw)
	b, err := proto.Marshal(raw)
	require.NoError(t, err)
	return b
}

// TestValidateAndCommitOnBftChain runs with the config of a validator of a chain of the BFT
// consensus, IBAX_TEST_BFT_CONFIG, on a copy of its database named IBAX_TEST_BFT_DB, and changes
// the copy: it rolls the last IBAX_TEST_BFT_BLOCKS blocks (20 by default) back with their
// certificates and validators sets, then for each of them in turn its certificate verifies by the
// validators set of its height; Validate takes the block at its height and hash, and refuses it at
// another height, with another hash, with a header or transactions that are not those of its
// hash; Timely takes it within the bounds of syspar.GetBftTimeliness and refuses it out of them;
// the commit applies it with the hash and the rollbacks hash the chain has, and writes the
// validators set of the next height the chain has. Before that, no contract changes
// bft_consensus, and a change of its row changes no consensus.
func TestValidateAndCommitOnBftChain(t *testing.T) {
	config, name := os.Getenv("IBAX_TEST_BFT_CONFIG"), os.Getenv("IBAX_TEST_BFT_DB")
	if config == "" || name == "" {
		t.Skip("IBAX_TEST_BFT_CONFIG and IBAX_TEST_BFT_DB name no validator and no copy of its database")
	}
	count := int64(20)
	if v := os.Getenv("IBAX_TEST_BFT_BLOCKS"); v != "" {
		var err error
		count, err = strconv.ParseInt(v, 10, 64)
		require.NoError(t, err)
	}
	require.NoError(t, conf.LoadConfig(config))
	conf.Config.DB.Name = name
	require.NoError(t, sqldb.GormInit(conf.Config.DB))
	require.NoError(t, syspar.ReadNodeKeys())
	log.SetLevel(log.FatalLevel)

	data, ok := block.GetDataFromFirstBlock()
	require.True(t, ok)
	syspar.SetFirstBlockData(data)
	syspar.SetRunModel(consts.HonorNodeMode)
	load := func() {
		require.NoError(t, syspar.SysUpdate(nil))
		require.NoError(t, syspar.SysTableColType(nil))
		smart.InitVM()
		require.NoError(t, smart.LoadContracts())
	}
	load()
	require.True(t, syspar.IsBftConsensus(), "the chain of %s is not of the BFT consensus", name)

	// the first block fixes the consensus: no contract changes the parameter, and a change of the
	// row changes nothing the node reads
	dbTx, err := sqldb.StartTransaction()
	require.NoError(t, err)
	_, err = smart.UpdatePlatformParam(&smart.SmartContract{DbTransaction: dbTx}, syspar.BftConsensus, "0", "")
	require.ErrorContains(t, err, "fixed by the first block")
	require.NoError(t, sqldb.GetDB(dbTx).Exec(`UPDATE "1_platform_parameters" SET value = '0' WHERE name = ?`, syspar.BftConsensus).Error)
	require.NoError(t, syspar.SysUpdate(dbTx))
	require.Equal(t, "0", syspar.SysString(syspar.BftConsensus))
	require.True(t, syspar.IsBftConsensus(), "a change of the row of bft_consensus changes the consensus")
	require.NoError(t, dbTx.Rollback())
	load()
	require.Equal(t, "1", syspar.SysString(syspar.BftConsensus))

	last := &sqldb.BlockChain{}
	found, err := last.GetMaxBlock()
	require.NoError(t, err)
	require.True(t, found)
	from := last.ID - count + 1
	require.Greater(t, from, int64(1), "the chain has %d blocks", last.ID)
	var blocks []sqldb.BlockChain
	require.NoError(t, sqldb.DBConn.Where("id >= ?", from).Order("id").Find(&blocks).Error)
	require.Len(t, blocks, int(count))
	certs := map[int64]sqldb.BftCertificate{}
	for _, b := range blocks {
		c := sqldb.BftCertificate{}
		found, err := c.Get(nil, b.ID)
		require.NoError(t, err)
		require.Truef(t, found, "no certificate of block %d", b.ID)
		certs[b.ID] = c
	}
	sets := map[int64][]byte{}
	for h := from; h <= last.ID+1; h++ {
		s := &sqldb.BftValidatorSet{}
		found, err := s.GetAt(nil, h)
		require.NoError(t, err)
		require.Truef(t, found, "no validators set of height %d", h)
		sets[h] = s.Data
	}

	// a committed block is final: the copy goes back by the rollbacks, then loses the
	// certificates and the sets the blocks rolled back wrote
	require.NoError(t, rollback.ToBlockID(from-1, nil, log.WithFields(log.Fields{})))
	require.NoError(t, sqldb.DBConn.Exec(`DELETE FROM bft_certificates WHERE height >= ?`, from).Error)
	require.NoError(t, sqldb.DBConn.Exec(`DELETE FROM bft_validator_sets WHERE from_height > ?`, from).Error)
	load()

	network := uint64(conf.Config.LocalConf.NetworkID)
	var txs int32
	for _, want := range blocks {
		txs += want.Tx
		cert, err := bft.UnmarshalCertificate(certs[want.ID].Data)
		require.NoError(t, err)
		set, err := bft.UnmarshalValidators(sets[want.ID])
		require.NoError(t, err)
		require.NoErrorf(t, cert.Verify(set, network, uint64(want.ID), cert.Hash), "the certificate of block %d", want.ID)
		hash, err := bft.HashOf(want.Hash)
		require.NoError(t, err)
		require.Equal(t, cert.Hash, hash)

		b, err := block.ProcessBlockByBinData(want.Data, true)
		require.NoError(t, err)
		at := time.Unix(b.Header.Timestamp, 0)
		what := func(s string) string { return fmt.Sprintf("block %d %s", want.ID, s) }
		require.ErrorContainsf(t, block.Validate(want.Data, want.ID+1, hash), "proposed at height", what("at another height"))
		other := hash
		other[0] ^= 1
		require.ErrorContainsf(t, block.Validate(want.Data, want.ID, other), "is not the hash proposed", what("with another hash"))
		header := tamper(t, want.Data, func(r *types.BlockData) { r.Header.Timestamp++ })
		require.ErrorContainsf(t, block.Validate(header, want.ID, hash), "not that of its content", what("with another header"))
		if want.Tx > 0 {
			body := tamper(t, want.Data, func(r *types.BlockData) { r.TxFullData = append(r.TxFullData, r.TxFullData[0]) })
			require.ErrorContainsf(t, block.Validate(body, want.ID, hash), "merkle root", what("with another transaction"))
		}
		early, late := syspar.GetBftTimeliness()
		require.NoErrorf(t, block.Timely(want.Data, at.Add(-early), early, late), what("early within the bounds"))
		require.NoErrorf(t, block.Timely(want.Data, at.Add(late), early, late), what("late within the bounds"))
		require.ErrorContainsf(t, block.Timely(want.Data, at.Add(-early-time.Millisecond), early, late), "off the clock", what("early"))
		require.ErrorContainsf(t, block.Timely(want.Data, at.Add(late+time.Millisecond), early, late), "off the clock", what("late"))
		require.NoErrorf(t, block.Validate(want.Data, want.ID, hash), what("at its height, hash and time"))

		_, err = block.Commit(want.Data, cert)
		require.NoErrorf(t, err, "the commit of block %d", want.ID)
		got := &sqldb.BlockChain{}
		found, err := got.Get(want.ID)
		require.NoError(t, err)
		require.True(t, found)
		require.Equalf(t, want.Hash, got.Hash, "the hash of block %d", want.ID)
		require.Equalf(t, want.RollbacksHash, got.RollbacksHash, "the rollbacks hash of block %d", want.ID)
		next := &sqldb.BftValidatorSet{}
		found, err = next.GetAt(nil, want.ID+1)
		require.NoError(t, err)
		require.True(t, found)
		require.Equalf(t, sets[want.ID+1], next.Data, "the validators set of height %d", want.ID+1)
	}
	t.Logf("blocks %d-%d, %d transactions: each certificate verifies, Validate takes each block and refuses each change of it, each commit has the hash, the rollbacks hash and the next validators set of the chain", from, last.ID, txs)
}
