/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block_test

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/language"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/rollback"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// dbState is the content of every table of the database, by table; the sequences are left out:
// PostgreSQL does not roll a sequence back, and the one a block uses, rollback_tx_id_seq, only
// orders the rows of rollback_tx, which the rollbacks hash reads without their ids
func dbState(t *testing.T) map[string]string {
	t.Helper()
	var tables []string
	require.NoError(t, sqldb.DBConn.Raw(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY table_name`).Scan(&tables).Error)
	state := make(map[string]string, len(tables))
	for _, name := range tables {
		var sum string
		require.NoError(t, sqldb.DBConn.Raw(fmt.Sprintf(`SELECT coalesce(md5(string_agg(r::text, '|' ORDER BY r::text)), '')
			FROM %q r`, name)).Scan(&sum).Error)
		state[name] = sum
	}
	return state
}

// vmState is every object of the virtual machine by name with its value, every child by index and
// the owner of every contract
func vmState() string {
	vm := script.GetVM()
	names := make([]string, 0, len(vm.Objects))
	for k := range vm.Objects {
		names = append(names, k)
	}
	slices.Sort(names)
	var out bytes.Buffer
	for _, k := range names {
		fmt.Fprintf(&out, "%s %p %p\n", k, vm.Objects[k], vm.Objects[k].Value)
	}
	for i, c := range vm.Children {
		fmt.Fprintf(&out, "%d %p", i, c)
		if c != nil && c.Type == script.ObjectType_Contract {
			fmt.Fprintf(&out, " %+v", *c.GetContractInfo().Owner)
		}
		out.WriteByte('\n')
	}
	return out.String()
}

type nodeState struct {
	db     map[string]string
	vm     string
	syspar map[string]string
	lang   language.Snapshot
}

func takeState(t *testing.T) nodeState {
	return nodeState{db: dbState(t), vm: vmState(), syspar: syspar.GetSysParCache(), lang: language.TakeSnapshot()}
}

func requireSameState(t *testing.T, want, got nodeState, what string) {
	t.Helper()
	for name, sum := range want.db {
		require.Equalf(t, sum, got.db[name], "%s: table %s", what, name)
	}
	require.Equalf(t, len(want.db), len(got.db), "%s: the tables", what)
	require.Equalf(t, want.vm, got.vm, "%s: the virtual machine", what)
	require.Equalf(t, want.syspar, got.syspar, "%s: the platform parameters", what)
	require.Truef(t, reflect.DeepEqual(want.lang, got.lang), "%s: the language sources", what)
}

// heldCommits is the number of the commits of a held trial
func heldCommits(t *testing.T) uint64 {
	t.Helper()
	m := &dto.Metric{}
	h := metrics.BlockStage.WithLabelValues(metrics.KindHeld, metrics.StageCommit).(prometheus.Histogram)
	require.NoError(t, h.Write(m))
	return m.GetHistogram().GetSampleCount()
}

// TestTrialAndCommitOnChain runs with the config of a node, IBAX_TEST_CHAIN_CONFIG, on a copy of
// its database named IBAX_TEST_CHAIN_DB, and changes the copy: it rolls the last IBAX_TEST_CHAIN_BLOCKS blocks (20 by
// default) back, then for each of them in turn the trial leaves the database, as the other
// connections see it, the virtual machine, the platform parameters and the language sources as
// they were; the commit applies it with the hash and the rollbacks hash the chain has, and stores
// its certificate; a second trial of a committed block, on the state after it, changes nothing
// either, whether it fails or not. The blocks take turns: the commit commits the trial it holds;
// the commit executes a block it holds no trial of; the commit executes a block whose trial the
// trial of another block dropped.
func TestTrialAndCommitOnChain(t *testing.T) {
	config, name := os.Getenv("IBAX_TEST_CHAIN_CONFIG"), os.Getenv("IBAX_TEST_CHAIN_DB")
	if config == "" || name == "" {
		t.Skip("IBAX_TEST_CHAIN_CONFIG and IBAX_TEST_CHAIN_DB name no node and no copy of its database")
	}
	count := int64(20)
	if v := os.Getenv("IBAX_TEST_CHAIN_BLOCKS"); v != "" {
		var err error
		count, err = strconv.ParseInt(v, 10, 64)
		require.NoError(t, err)
	}
	require.NoError(t, conf.LoadConfig(config))
	conf.Config.DB.Name = name
	require.NoError(t, sqldb.GormInit(conf.Config.DB))
	require.NoError(t, syspar.ReadNodeKeys())
	log.SetLevel(log.FatalLevel)

	// as a node starts: the node of the first block is the node of a chain with no honor_nodes
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

	last := &sqldb.BlockChain{}
	found, err := last.GetMaxBlock()
	require.NoError(t, err)
	require.True(t, found)
	from := last.ID - count + 1
	require.Greater(t, from, int64(1), "the chain has %d blocks", last.ID)
	var blocks []sqldb.BlockChain
	require.NoError(t, sqldb.DBConn.Where("id >= ?", from).Order("id").Find(&blocks).Error)
	require.Len(t, blocks, int(count))

	require.NoError(t, rollback.ToBlockID(from-1, nil, log.WithFields(log.Fields{})))
	load()

	var txs, failed int32
	var prev []byte
	for i, want := range blocks {
		txs += want.Tx
		mode := i % 3
		if prev == nil && mode == 2 {
			mode = 0
		}
		if mode != 1 {
			before := takeState(t)
			b, err := block.ProcessBlockByBinData(want.Data, true)
			require.NoError(t, err)
			require.NoErrorf(t, b.Trial(), "the trial of block %d", want.ID)
			requireSameState(t, before, takeState(t), fmt.Sprintf("after the trial of block %d", want.ID))
		}
		if mode == 2 {
			// the trial of the block before, committed, drops the trial of this one, held or not
			if b, err := block.ProcessBlockByBinData(prev, true); err == nil {
				b.Trial()
			}
		}
		held := heldCommits(t)

		hash, err := bft.HashOf(want.Hash)
		require.NoError(t, err)
		cert := &bft.Certificate{Height: uint64(want.ID), Round: 2, Hash: hash,
			Precommits: []bft.Signed{{Validator: 0, Signature: []byte{1}}}}
		_, err = block.Commit(want.Data, cert)
		require.NoErrorf(t, err, "the commit of block %d", want.ID)
		if mode == 0 {
			require.Equalf(t, held+1, heldCommits(t), "block %d commits the trial it holds", want.ID)
		} else {
			require.Equalf(t, held, heldCommits(t), "block %d commits a trial it does not hold", want.ID)
		}
		prev = want.Data
		got := &sqldb.BlockChain{}
		found, err := got.Get(want.ID)
		require.NoError(t, err)
		require.True(t, found)
		require.Equalf(t, want.Hash, got.Hash, "the hash of block %d", want.ID)
		require.Equalf(t, want.RollbacksHash, got.RollbacksHash, "the rollbacks hash of block %d", want.ID)
		stored := &sqldb.BftCertificate{}
		found, err = stored.Get(nil, want.ID)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, cert.Marshal(), stored.Data)
		require.Equal(t, int64(2), stored.Round)

		if want.Tx > 0 {
			after := takeState(t)
			b, err := block.ProcessBlockByBinData(want.Data, true)
			if err != nil || b.Trial() != nil {
				failed++
			}
			requireSameState(t, after, takeState(t), fmt.Sprintf("after the failed trial of block %d", want.ID))
		}
	}
	t.Logf("blocks %d-%d, %d transactions: each trial left the state as it was, each commit has the hash and the rollbacks hash of the chain, the held trials committed once; %d second trials failed", from, last.ID, txs, failed)
}
