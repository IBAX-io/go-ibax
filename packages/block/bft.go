/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/language"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
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

// KeepMemState takes the state outside the database that executing blocks changes and returns
// what puts it back, the platform parameters and the column types read again from the database
func KeepMemState() func() error {
	return takeMemState().restore
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

// inChain reports whether the chain has the block, by its height and its hash: a commit whose
// connection fails after PostgreSQL commits returns an error
func inChain(b *Block) (bool, error) {
	bc := &sqldb.BlockChain{}
	found, err := bc.GetMeta(nil, b.Header.BlockId)
	if err != nil {
		return false, err
	}
	return found && bytes.Equal(bc.Hash, b.Header.BlockHash), nil
}

// Certify parses the block a certificate commits and checks that the certificate is of it: of its
// height and of its hash, the hash of its content. It checks nothing of the certificate's
// precommits, which bft.Certificate.Verify does.
func Certify(binData []byte, cert *bft.Certificate) (*Block, error) {
	b, err := parseBft(binData, cert.Hash, useCommit)
	if err != nil {
		return nil, fmt.Errorf("the certificate of height %d: %w", cert.Height, err)
	}
	if uint64(b.Header.BlockId) != cert.Height {
		return nil, fmt.Errorf("the certificate of height %d commits no block %d", cert.Height, b.Header.BlockId)
	}
	return b, nil
}

// VerifyCertified checks, of a block as a peer sends it, that the certificate commits it and
// verifies by the validators set, without the database: the block carries the header of the block
// it follows, whose hashes its hash takes, so the certificate commits the header it carries too.
// It returns the block, its transactions as written.
func VerifyCertified(raw []byte, cert *bft.Certificate, set bft.Validators, networkID uint64) (*types.BlockData, error) {
	b := &types.BlockData{}
	if err := proto.Unmarshal(raw, b); err != nil {
		return nil, fmt.Errorf("parsing the block: %w", err)
	}
	if b.Header == nil || b.PrevHeader == nil {
		return nil, errors.New("a block without its header or the header of the block before")
	}
	height := b.Header.BlockId
	if uint64(height) != cert.Height || b.PrevHeader.BlockId != height-1 {
		return nil, fmt.Errorf("the certificate of height %d commits no block %d after %d", cert.Height, height, b.PrevHeader.BlockId)
	}
	if b.Header.Version != consts.BftBlockVersion {
		return nil, fmt.Errorf("block %d: version %d, not %d", height, b.Header.Version, consts.BftBlockVersion)
	}
	merkle := b.GenMerkleRoot()
	if !bytes.Equal(merkle, b.MerkleRoot) {
		return nil, fmt.Errorf("block %d: the merkle root is not that of its transactions", height)
	}
	if !bytes.Equal(b.Header.GenHash(b.PrevHeader, merkle), b.Header.BlockHash) {
		return nil, fmt.Errorf("block %d: the hash is not that of its content", height)
	}
	hash, err := bft.HashOf(b.Header.BlockHash)
	if err != nil {
		return nil, err
	}
	if hash != cert.Hash {
		return nil, fmt.Errorf("the certificate of height %d commits another block than %x", height, b.Header.BlockHash)
	}
	if err := cert.Verify(set, networkID, cert.Height, cert.Hash); err != nil {
		return nil, fmt.Errorf("the certificate of height %d: %w", height, err)
	}
	b.BinData = raw
	return b, nil
}

// parseProposal parses the block of a proposal, and checks it is of the height, its hash is the
// hash and the hash of its content, and it is of the network and of the version
func parseProposal(binData []byte, height int64, hash bft.Hash) (*Block, error) {
	b, err := parseBft(binData, hash, useProposal)
	if err != nil {
		return nil, err
	}
	if b.Header.BlockId != height {
		return nil, fmt.Errorf("a block %d proposed at height %d", b.Header.BlockId, height)
	}
	if err := checkNetwork(b.Header); err != nil {
		return nil, err
	}
	return b, nil
}

// checkNetwork checks the block is of the network of the node and of the version
func checkNetwork(h *types.BlockHeader) error {
	if h.NetworkId != conf.Config.LocalConf.NetworkID || h.Version != consts.BftBlockVersion {
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

// validatorsHash takes, on a chain of the BFT consensus, the validators set the execution of the
// block leaves (setHeight; design 4.1). Block 1 carries no hash of it, the set it leaves is that of
// the honor_nodes it writes; an ordered block carries none either, its execution hash covers the
// set.
func (b *Block) validatorsHash(dbTx *sqldb.DbTransaction) error {
	b.nextValidators = nil
	if !syspar.IsBftConsensus() {
		return nil
	}
	return b.leaveValidators(dbTx)
}

// checkStateHash checks the state hash of the block of another node (design 4.3). A block may
// carry none: a proposer without the hash, after a restart that lost its snapshot, proposes
// without it, so that the chain goes on whatever the nodes lost. A block carries one only at the
// delay after a snapshot height, the hash of the snapshot. A validator prevotes a proposal with
// the hash only if it has the hash and it is the same; it commits a block a certificate commits,
// whose hash more than two thirds of the validators prevoted, once it has hashed the snapshot,
// if it is the same, and if the node lost the snapshot, as it is. A node whose state hash is not
// the one the chain committed stops at the block.
func (b *Block) checkStateHash() error {
	if len(b.Header.StateHash) == 0 {
		return nil
	}
	s := stateHeight(b.Header.BlockId)
	if s == 0 {
		return fmt.Errorf("block %d: a state hash at a height without a snapshot", b.Header.BlockId)
	}
	get := states.wait
	if b.trial {
		get = states.get
	}
	hash, status, err := get(s)
	if err != nil {
		return err
	}
	switch {
	case status == stateDone && !bytes.Equal(hash, b.Header.StateHash):
		return fmt.Errorf("block %d: the state hash %x of height %d is not %x, the hash of the state of this node", b.Header.BlockId, b.Header.StateHash, s, hash)
	case status != stateDone && b.trial:
		return fmt.Errorf("block %d: this node has no state hash of height %d yet", b.Header.BlockId, s)
	}
	return nil
}

// storeValidators writes, in the transaction of the block, the validators set the execution of
// the block leaves (setHeight) if it is not the set before: the nodes of honor_nodes in their
// order, but the stopped ones and those an evidence keeps out of the height (design 4.5)
func (b *Block) storeValidators(dbTx *sqldb.DbTransaction) error {
	height := b.setHeight()
	data := b.nextValidators
	if data == nil {
		// block 1 makes the chain one of the BFT consensus as it inserts it
		set, err := smart.BftValidatorsOf(dbTx, height)
		if err != nil {
			return err
		}
		data = set.Marshal()
	}
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
