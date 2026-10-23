/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"fmt"
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

// Trial executes the block in a database transaction as its commit would, then rolls all of it
// back (F18 design 4.3): after it the database, the contracts of the virtual machine, the language
// sources and the platform parameters are as they were before. A validator prevotes a proposal only
// if its trial succeeds. The proposer makes its proposal by the trial of a generated block, which
// sets the transactions, the header and BinData of the block. Of a block that is not committed no
// notification is due and no node pauses; a validator neither bans a key nor marks a transaction
// bad for the block of another node, the proposer marks the transactions it leaves out as it does
// when it generates a block.
func (b *Block) Trial() error {
	mem := takeMemState()
	dbTx, err := sqldb.StartTransaction()
	if err != nil {
		return err
	}
	b.trial = true
	b.beginProfile()
	defer func() { b.prof = nil }()
	err = b.ProcessTxs(dbTx)
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
	b.Notifications = nil
	b.SysUpdate = false
	start := time.Now()
	if errRoll := dbTx.Rollback(); errRoll != nil && err == nil {
		err = errRoll
	}
	if errMem := mem.restore(); errMem != nil {
		return errMem
	}
	b.stage(metrics.StageRollback, start)
	if err == nil {
		b.endProfile(dbTx)
	}
	return err
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

// Commit executes the block a certificate commits, as a block another node made, and stores the
// block and the certificate in one database transaction: a node has the certificate of a height
// if and only if it has its block. The block is parsed again from BinData, so nothing a trial left
// on the parsed transactions reaches the commit.
func Commit(binData []byte, cert *bft.Certificate) (*Block, error) {
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

// Propose makes the proposal of the proposer from a block it began at start: the trial executes
// it, leaves out the transactions that fail, do not fit, or do not end within
// max_block_generation_time from start, and sets its rollbacks hash, its hash and BinData. A block
// left without transactions is ErrEmptyBlock.
func Propose(binData []byte, txs []*transaction.Transaction, start time.Time) (*Block, error) {
	b, err := ProcessBlockByBinData(binData, false)
	if err != nil {
		return nil, err
	}
	b.GenBlock = true
	b.Transactions = txs
	b.genStart = start
	if err := b.Trial(); err != nil {
		return nil, err
	}
	return b, nil
}

// Validate is valid(v) of a validator for the proposal of a block at a height (design 4.2, 4.4):
// it parses, it is of the height, its hash is the hash and the hash of its content, it is of the
// chain, its time is above the time of the last block, its checks pass and its trial succeeds.
// The clock takes no part in it: Timely checks the time of a new proposal.
func Validate(binData []byte, height int64, hash bft.Hash) error {
	b, err := ProcessBlockByBinData(binData, true)
	if err != nil {
		return err
	}
	if b.Header.BlockId != height {
		return fmt.Errorf("a block %d proposed at height %d", b.Header.BlockId, height)
	}
	got, err := bft.HashOf(b.Header.BlockHash)
	if err != nil {
		return err
	}
	if got != hash {
		return fmt.Errorf("block %d: the hash %x is not the hash proposed", height, b.Header.BlockHash)
	}
	if err := b.checkContent(); err != nil {
		return err
	}
	if b.Header.NetworkId != conf.Config.LocalConf.NetworkID || b.Header.Version != consts.BlockVersion {
		return fmt.Errorf("block %d: network %d, version %d", height, b.Header.NetworkId, b.Header.Version)
	}
	b.trial = true
	err = b.Check()
	b.trial = false
	if err != nil {
		return err
	}
	return b.Trial()
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
