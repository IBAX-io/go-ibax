/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/language"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/gogo/protobuf/proto"
	log "github.com/sirupsen/logrus"
)

// memState is the state outside the database that executing a block changes: the contracts of the
// virtual machine and the language sources. The platform parameters and the column types are
// reloaded from the committed database instead, and the parsed transactions are dropped.
type memState struct {
	vm   *script.VMSnapshot
	lang language.Snapshot
}

func takeMemState() memState {
	return memState{vm: script.GetVM().Snapshot(), lang: language.TakeSnapshot()}
}

func (m memState) restore() error {
	script.GetVM().Restore(m.vm)
	language.Restore(m.lang)
	if err := syspar.SysUpdate(nil); err != nil {
		return fmt.Errorf("reloading the platform parameters: %w", err)
	}
	if err := syspar.SysTableColType(nil); err != nil {
		return fmt.Errorf("reloading the column types: %w", err)
	}
	return nil
}

// heldTrial is the trial of a block a node holds for the decision of its height: the database
// transaction of the trial, open, and the state outside the database the trial left, which the
// node runs without until the block is committed
type heldTrial struct {
	b    *Block
	dbTx *sqldb.DbTransaction
	mem  memState
}

// held is the trial the node holds, of the last block it tried at its height. Only the driver of
// the consensus and the synchronization, both under DBLock, try and commit blocks.
var held struct {
	sync.Mutex
	t *heldTrial
}

// takeHeld takes the trial the node holds, if any
func takeHeld() *heldTrial {
	held.Lock()
	defer held.Unlock()
	t := held.t
	held.t = nil
	return t
}

// dropHeld rolls back the trial the node holds: the state outside the database is already as it
// was before it
func dropHeld() {
	t := takeHeld()
	if t == nil {
		return
	}
	start := time.Now()
	if err := t.dbTx.Rollback(); err != nil {
		t.b.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("rolling back a held trial")
	}
	metrics.BlockStage.WithLabelValues(metrics.KindTrial, metrics.StageRollback).Observe(metrics.Since(start))
}

// Trial executes the block in a database transaction as its commit would (F18 design 4.3, plan
// 5.9): a block is executed once. The trial that succeeds is held, its transaction open, until
// the decision of its height; Commit commits it if the decision is of the block, and a trial of
// another block, or the commit of another, rolls it back. After a trial the contracts of the
// virtual machine, the language sources and the platform parameters are as they were before it;
// Commit sets those the trial left. A validator prevotes a proposal only if its trial succeeds.
// The proposer makes its proposal by the trial of a generated block, which sets the transactions,
// the header and BinData of the block. A validator neither bans a key nor marks a transaction
// bad for the block of another node, the proposer marks the transactions it leaves out as it does
// when it generates a block.
func (b *Block) Trial() error {
	dbTx, mem, err := b.beginTrial()
	if err != nil {
		return err
	}
	return b.endTrial(dbTx, mem, b.ProcessTxs(dbTx))
}

// beginTrial drops the trial the node holds and begins the trial of the block: the state outside
// the database as it is, and the database transaction of the trial
func (b *Block) beginTrial() (*sqldb.DbTransaction, memState, error) {
	dropHeld()
	begin := time.Now()
	mem := takeMemState()
	dbTx, err := sqldb.StartTransaction()
	if err != nil {
		return nil, mem, err
	}
	b.trial = true
	b.beginProfile()
	b.prof.start = begin
	b.stage(metrics.StageBegin, begin)
	return dbTx, mem, nil
}

// endTrial ends the trial of the block whose transactions ran with the error err: it inserts the
// block and holds the trial, or rolls it back
func (b *Block) endTrial(dbTx *sqldb.DbTransaction, mem memState, err error) error {
	defer func() { b.prof = nil }()
	if err == nil {
		if b.GenBlock && len(b.TxFullData) == 0 {
			err = ErrEmptyBlock
		} else {
			start := time.Now()
			err = b.InsertIntoBlockchain(dbTx)
			b.stage(metrics.StageInsert, start)
		}
	}
	b.trial = false
	b.SysUpdate = false
	start := time.Now()
	if err != nil {
		b.Notifications = nil
		dbTx.Rollback()
		if errMem := mem.restore(); errMem != nil {
			return errMem
		}
		b.stage(metrics.StageRollback, start)
		return err
	}
	left := takeMemState()
	if err := mem.restore(); err != nil {
		dbTx.Rollback()
		return err
	}
	held.Lock()
	held.t = &heldTrial{b: b, dbTx: dbTx, mem: left}
	held.Unlock()
	b.stage(metrics.StageHold, start)
	observeTail(b.execEnd, b.txCount())
	b.endProfile(dbTx)
	return nil
}

// commitHeld commits the trial the node holds if it is of the block the certificate commits, with
// the certificate, and reports whether it was; one of another block it rolls back
func commitHeld(cert *bft.Certificate) (*Block, bool, error) {
	t := takeHeld()
	if t == nil {
		return nil, false, nil
	}
	b := t.b
	hash, err := bft.HashOf(b.Header.BlockHash)
	if err != nil || uint64(b.Header.BlockId) != cert.Height || hash != cert.Hash {
		start := time.Now()
		t.dbTx.Rollback()
		metrics.BlockStage.WithLabelValues(metrics.KindTrial, metrics.StageRollback).Observe(metrics.Since(start))
		return nil, false, nil
	}
	start := time.Now()
	c := &sqldb.BftCertificate{Height: b.Header.BlockId, Round: int64(cert.Round), Hash: cert.Hash[:], Data: cert.Marshal()}
	err = c.Create(t.dbTx)
	if err == nil && syspar.IsBftConsensus() {
		err = storeValidators(t.dbTx, b.Header.BlockId+1)
	}
	if err != nil {
		t.dbTx.Rollback()
		return nil, true, err
	}
	if err := t.dbTx.Commit(); err != nil {
		return nil, true, err
	}
	if err := t.mem.restore(); err != nil {
		return nil, true, err
	}
	metrics.BlockStage.WithLabelValues(metrics.KindHeld, metrics.StageCommit).Observe(metrics.Since(start))
	transaction.TxPool.Remove(b.committedHashes())
	start = time.Now()
	for _, q := range b.Notifications {
		q.Send()
	}
	metrics.BlockStage.WithLabelValues(metrics.KindHeld, metrics.StageNotify).Observe(metrics.Since(start))
	return b, true, nil
}

// committedHashes is the hashes of the transactions of the block: of a generated block, those it took
func (b *Block) committedHashes() [][]byte {
	if b.GenBlock {
		hashes := make([][]byte, 0, len(b.AfterTxs.Txs))
		for _, after := range b.AfterTxs.Txs {
			hashes = append(hashes, after.UsedTx)
		}
		return hashes
	}
	hashes := make([][]byte, len(b.Transactions))
	for i, t := range b.Transactions {
		hashes[i] = t.Hash()
	}
	return hashes
}

// Certify parses the block a certificate commits and checks that the certificate is of it: of its
// height and of its hash, the hash of its content. It checks nothing of the certificate's
// precommits, which bft.Certificate.Verify does.
func Certify(binData []byte, cert *bft.Certificate) (*Block, error) {
	b, err := ProcessBlockByBinData(binData, true)
	if err != nil {
		return nil, err
	}
	if uint64(b.Header.BlockId) != cert.Height {
		return nil, fmt.Errorf("the certificate of height %d commits no block %d", cert.Height, b.Header.BlockId)
	}
	hash, err := bft.HashOf(b.Header.BlockHash)
	if err != nil {
		return nil, err
	}
	if hash != cert.Hash {
		return nil, fmt.Errorf("the certificate of height %d commits another block than %x", cert.Height, b.Header.BlockHash)
	}
	if err := b.checkContent(); err != nil {
		return nil, err
	}
	return b, nil
}

// Commit stores the block a certificate commits and the certificate in one database transaction:
// a node has the certificate of a height if and only if it has its block. The trial the node holds
// of the block is committed; a block the node holds no trial of is executed, as a block another
// node made, parsed again from BinData.
func Commit(binData []byte, cert *bft.Certificate) (*Block, error) {
	if b, ok, err := commitHeld(cert); ok {
		return b, err
	}
	b, err := Certify(binData, cert)
	if err != nil {
		return nil, err
	}
	c := &sqldb.BftCertificate{
		Height: b.Header.BlockId,
		Round:  int64(cert.Round),
		Hash:   cert.Hash[:],
		Data:   cert.Marshal(),
	}
	if err := b.playSafe(c.Create); err != nil {
		return nil, err
	}
	return b, nil
}

// checkContent checks that the hash of the block is the hash of its content: its header, the
// last block it follows, and the merkle root of its transactions as the block carries them. A
// parsed block takes its hash and its merkle root as they are written, so a certificate, which
// commits the hash, commits the content only by this check.
func (b *Block) checkContent() error {
	raw := &types.BlockData{}
	if err := proto.Unmarshal(b.BinData, raw); err != nil {
		return fmt.Errorf("parsing the block: %w", err)
	}
	merkle := raw.GenMerkleRoot()
	if !bytes.Equal(merkle, raw.MerkleRoot) {
		return fmt.Errorf("block %d: the merkle root is not that of its transactions", b.Header.BlockId)
	}
	if !bytes.Equal(b.Header.GenHash(b.PrevHeader, merkle), b.Header.BlockHash) {
		return fmt.Errorf("block %d: the hash is not that of its content", b.Header.BlockId)
	}
	return nil
}

// Propose makes the proposal of the proposer from a block whose time it read at start: the trial
// executes it, leaves out the transactions that fail, do not fit, or leave the block no time to
// end within max_block_generation_time from start, and sets its rollbacks hash, its hash and
// BinData. A block left without transactions is ErrEmptyBlock.
//
// A stream, if not nil, takes the header of the block before the trial and the transactions the
// trial puts in the block as it puts them, in their order (design 4.2 "streaming proposals").
func Propose(binData []byte, txs []*transaction.Transaction, start time.Time, stream Stream) (*Block, error) {
	b, err := ProcessBlockByBinData(binData, false)
	if err != nil {
		return nil, err
	}
	b.GenBlock = true
	b.Transactions = txs
	b.genStart = start
	if stream != nil {
		shell, err := b.shell()
		if err != nil {
			return nil, err
		}
		stream.Header(shell)
		b.onCommit = stream.Commit
		defer func() { b.onCommit = nil }()
	}
	if err := b.Trial(); err != nil {
		return nil, err
	}
	return b, nil
}

// Validate is valid(v) of a validator for the proposal of a block at a height (design 4.2, 4.4):
// it parses, it is of the height, its hash is the hash and the hash of its content, it is of the
// chain, its time is above the time of the last block, its checks pass and its trial succeeds.
// The clock takes no part in it: Timely checks the time of a new proposal. What reads only the
// block, its signatures verified in parallel included, runs before writes is locked; the checks
// against the chain and the trial run under it (plan 5.9).
func Validate(binData []byte, height int64, hash bft.Hash, writes sync.Locker) error {
	b, err := parseProposal(binData, height, hash)
	if err != nil {
		return err
	}
	b.trial = true
	b.verifySignatures()
	writes.Lock()
	defer writes.Unlock()
	err = b.Check()
	b.trial = false
	if err != nil {
		return err
	}
	return b.Trial()
}

// parseProposal parses the block of a proposal, and checks it is of the height, its hash is the
// hash and the hash of its content, and it is of the network and of the version
func parseProposal(binData []byte, height int64, hash bft.Hash) (*Block, error) {
	b, err := ProcessBlockByBinData(binData, true)
	if err != nil {
		return nil, err
	}
	if b.Header.BlockId != height {
		return nil, fmt.Errorf("a block %d proposed at height %d", b.Header.BlockId, height)
	}
	got, err := bft.HashOf(b.Header.BlockHash)
	if err != nil {
		return nil, err
	}
	if got != hash {
		return nil, fmt.Errorf("block %d: the hash %x is not the hash proposed", height, b.Header.BlockHash)
	}
	if err := b.checkContent(); err != nil {
		return nil, err
	}
	if err := checkNetwork(b.Header); err != nil {
		return nil, err
	}
	return b, nil
}

// checkNetwork checks the block is of the network of the node and of the version
func checkNetwork(h *types.BlockHeader) error {
	if h.NetworkId != conf.Config.LocalConf.NetworkID || h.Version != consts.BlockVersion {
		return fmt.Errorf("block %d: network %d, version %d", h.BlockId, h.NetworkId, h.Version)
	}
	return nil
}

// Timely checks the time of the block of a new proposal against the time a validator took the
// proposal (design 4.4): the time of the block is ahead of it by at most ahead, and behind it by
// at most behind
func Timely(binData []byte, received time.Time, ahead, behind time.Duration) error {
	raw := &types.BlockData{}
	if err := proto.Unmarshal(binData, raw); err != nil {
		return fmt.Errorf("parsing the block: %w", err)
	}
	if raw.Header == nil {
		return fmt.Errorf("a block without a header")
	}
	off := time.Unix(raw.Header.Timestamp, 0).Sub(received)
	if off > ahead || -off > behind {
		return fmt.Errorf("block %d: its time is %v off the clock, more than %v ahead or %v behind", raw.Header.BlockId, off, ahead, behind)
	}
	return nil
}

// storeValidators writes, in the transaction of block height−1, the validators set of height if
// it is not the set before: the nodes of honor_nodes in their order, but the stopped ones and
// those an evidence keeps out of height (design 4.5)
func storeValidators(dbTx *sqldb.DbTransaction, height int64) error {
	set, err := smart.BftValidatorsOf(dbTx, height)
	if err != nil {
		return err
	}
	data := set.Marshal()
	last := &sqldb.BftValidatorSet{}
	found, err := last.GetAt(dbTx, height)
	if err != nil {
		return err
	}
	if found && bytes.Equal(last.Data, data) {
		return nil
	}
	return (&sqldb.BftValidatorSet{FromHeight: height, Data: data}).Create(dbTx)
}
