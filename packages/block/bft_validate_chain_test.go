/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block_test

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/rollback"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/gogo/protobuf/proto"
	dto "github.com/prometheus/client_model/go"
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

// countedLock is the lock of the writes of blocks, which counts how often it was taken
type countedLock struct {
	sync.Mutex
	taken int
}

func (l *countedLock) Lock() {
	l.Mutex.Lock()
	l.taken++
}

// verifyCount is the number of trials that verified in parallel the signatures of their block
func verifyCount(t *testing.T) uint64 {
	var m dto.Metric
	h := metrics.BlockStage.WithLabelValues(metrics.KindTrial, metrics.StageVerify).(interface{ Write(*dto.Metric) error })
	require.NoError(t, h.Write(&m))
	return m.GetHistogram().GetSampleCount()
}

// TestValidateAndCommitOnBftChain runs with the config of a validator of a chain of the BFT
// consensus, IBAX_TEST_BFT_CONFIG, on a copy of its database named IBAX_TEST_BFT_DB, and changes
// the copy: it rolls the last IBAX_TEST_BFT_BLOCKS blocks (20 by default) back with their
// certificates and validators sets, then for each of them in turn its certificate verifies by the
// validators set of its height; Validate takes the block at its height and hash, and refuses it at
// another height, with another hash, with a header or transactions that are not those of its
// hash, the last ones before it locks the writes of blocks, and it verifies the signatures of a
// block with transactions in parallel and tries it under that lock; Timely takes it within the bounds of syspar.GetBftTimeliness and refuses it out of them;
// the commit applies it with the hash and the rollbacks hash the chain has, and writes the
// validators set of the next height the chain has. Before that, no contract changes
// bft_consensus or honor_nodes_only, and a change of their rows changes no consensus.
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
	require.True(t, syspar.IsHonorNodesOnly(), "the chain of %s is not of the honor nodes only", name)

	// the first block fixes the consensus: no contract changes the parameters, and a change of
	// their rows changes nothing the node reads
	dbTx, err := sqldb.StartTransaction()
	require.NoError(t, err)
	for _, par := range []string{syspar.BftConsensus, syspar.HonorNodesOnly} {
		_, err = smart.UpdatePlatformParam(&smart.SmartContract{DbTransaction: dbTx}, par, "0", "")
		require.ErrorContains(t, err, par+" is fixed by the first block")
		require.NoError(t, sqldb.GetDB(dbTx).Exec(`UPDATE "1_platform_parameters" SET value = '0' WHERE name = ?`, par).Error)
	}
	require.NoError(t, syspar.SysUpdate(dbTx))
	require.Equal(t, "0", syspar.SysString(syspar.BftConsensus))
	require.Equal(t, "0", syspar.SysString(syspar.HonorNodesOnly))
	require.True(t, syspar.IsBftConsensus(), "a change of the row of bft_consensus changes the consensus")
	require.True(t, syspar.IsHonorNodesOnly(), "a change of the row of honor_nodes_only changes the consensus")
	require.NoError(t, dbTx.Rollback())
	load()
	require.Equal(t, "1", syspar.SysString(syspar.BftConsensus))
	require.Equal(t, "1", syspar.SysString(syspar.HonorNodesOnly))

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
	for i, want := range blocks {
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
		writes := &countedLock{}
		require.ErrorContainsf(t, block.Validate(want.Data, want.ID+1, hash, writes), "proposed at height", what("at another height"))
		other := hash
		other[0] ^= 1
		require.ErrorContainsf(t, block.Validate(want.Data, want.ID, other, writes), "is not the hash proposed", what("with another hash"))
		header := tamper(t, want.Data, func(r *types.BlockData) { r.Header.Timestamp++ })
		require.ErrorContainsf(t, block.Validate(header, want.ID, hash, writes), "not that of its content", what("with another header"))
		if want.Tx > 0 {
			body := tamper(t, want.Data, func(r *types.BlockData) { r.TxFullData = append(r.TxFullData, r.TxFullData[0]) })
			require.ErrorContainsf(t, block.Validate(body, want.ID, hash, writes), "merkle root", what("with another transaction"))
		}
		// taken at a time t, the time of the block is ahead of t, or behind it
		ahead, behind := syspar.GetBftTimeliness(0)
		require.NoErrorf(t, block.Timely(want.Data, at.Add(-ahead), ahead, behind), what("ahead within the bound"))
		require.NoErrorf(t, block.Timely(want.Data, at.Add(behind), ahead, behind), what("behind within the bound"))
		require.ErrorContainsf(t, block.Timely(want.Data, at.Add(-ahead-time.Millisecond), ahead, behind), "off the clock", what("ahead"))
		require.ErrorContainsf(t, block.Timely(want.Data, at.Add(behind+time.Millisecond), ahead, behind), "off the clock", what("behind"))
		require.Zerof(t, writes.taken, what("refused before the writes are locked"))
		verified := verifyCount(t)
		require.NoErrorf(t, block.Validate(want.Data, want.ID, hash, writes), what("at its height, hash and time"))
		require.Equalf(t, 1, writes.taken, what("tried under the lock of the writes"))
		require.Truef(t, writes.TryLock(), what("validated, the lock of the writes is free"))
		writes.Unlock()
		if want.Tx > 0 {
			require.Equalf(t, verified+1, verifyCount(t), what("its signatures verified in parallel"))
		}

		if i%2 == 0 {
			// the commit goes on from the trial of the speculation, which runs the block as
			// Validate does: all its batches, or the first and the proposal the rest
			speculate(t, want.Data, want.ID, hash, i%4 == 2, what)
		}

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
	t.Logf("blocks %d-%d, %d transactions: each certificate verifies, Validate takes each block and refuses each change of it, a speculation of every other block takes it from its batches and refuses another order, each commit has the hash, the rollbacks hash and the next validators set of the chain", from, last.ID, txs)
}

// speculate runs the block in speculations of its streaming proposal: one of another height does
// not begin, an aborted one and those whose batches are not the start of the block in its order
// leave the writes and decide nothing, and the last one, of the batches of the block or of the
// first only, takes the block and holds its trial
func speculate(t *testing.T, binData []byte, height int64, hash bft.Hash, first bool, what func(string) string) {
	t.Helper()
	shell, err := block.ShellOf(binData, height, hash)
	require.NoError(t, err)
	raw := &types.BlockData{}
	require.NoError(t, proto.Unmarshal(binData, raw))
	// the batches carry the bodies of the queue, uncompressed; the block stores them compressed
	bodies := make([][]byte, len(raw.TxFullData))
	for i, body := range raw.TxFullData {
		bodies[i] = types.DoZlibUnCompress(body)
	}
	parse := func() []*transaction.Transaction {
		txs, err := block.ParseTransactions(bodies)
		require.NoError(t, err)
		return txs
	}
	free := func(writes *countedLock, s string) {
		require.Truef(t, writes.TryLock(), what(s+": the lock of the writes is free"))
		writes.Unlock()
	}
	writes := &countedLock{}
	_, err = block.Speculate(shell, height+1, writes)
	require.Errorf(t, err, what("a speculation at another height"))
	free(writes, "a speculation at another height")

	n := len(raw.TxFullData)
	s, err := block.Speculate(shell, height, writes)
	require.NoError(t, err)
	s.Add(parse()[:n/2])
	s.Abort()
	free(writes, "aborted")
	require.ErrorIsf(t, s.Finish(binData, height, hash), block.ErrSpeculation, what("finished after its abort"))

	if n >= 2 {
		txs := parse()
		txs[0], txs[1] = txs[1], txs[0]
		s, err = block.Speculate(shell, height, writes)
		require.NoError(t, err)
		s.Add(txs)
		require.ErrorIsf(t, s.Finish(binData, height, hash), block.ErrSpeculation, what("in another order"))
		free(writes, "in another order")
	}
	if n >= 1 {
		s, err = block.Speculate(shell, height, writes)
		require.NoError(t, err)
		s.Add(parse()[:1])
		s.Add(parse()[:1])
		require.ErrorIsf(t, s.Finish(binData, height, hash), block.ErrSpeculation, what("with a transaction twice"))
		free(writes, "with a transaction twice")
	}

	taken := writes.taken
	s, err = block.Speculate(shell, height, writes)
	require.NoError(t, err)
	txs := parse()
	if first {
		s.Add(txs[:n/3])
	} else {
		s.Add(txs[:n/3])
		s.Add(txs[n/3 : 2*n/3])
		s.Add(txs[2*n/3:])
	}
	require.NoErrorf(t, s.Finish(binData, height, hash), what("from its batches"))
	require.Equalf(t, taken+1, writes.taken, what("speculated under the lock of the writes"))
	free(writes, "speculated")
}
