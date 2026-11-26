/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
	log "github.com/sirupsen/logrus"
)

// The order of a chain of the BFT consensus (direction A): a proposal is valid by what the block
// carries and the chain committed before it, never by its execution; the commit of a block writes
// the block and its certificate, and the executor of the node executes it after (Execute).

var (
	// ErrStale is a proposal of a height the node has the block of, which it took by its
	// certificate while it checked the proposal
	ErrStale = errors.New("the node has the block of the height")
	// ErrNotExecuted is a proposal whose execution hash is of a block the node has not executed
	// yet: the validator waits for its executor
	ErrNotExecuted = errors.New("the node has not executed the block of the execution hash")
	// ErrExecHash is a committed block whose execution hash is not the one of this node: the
	// execution of this node is not that of the chain, and the node stops
	ErrExecHash = errors.New("the execution hash the chain committed is not that of this node")
)

func init() {
	sqldb.ParseWindowTxs = windowTxs
}

// windowTxs is the transactions of the data of a block as the window of the duplicates takes them
// windowTxs is the transactions of the data of a committed block as the window of the duplicates
// takes them, read without their contracts: the block may call a contract that a block the node
// has not executed yet creates
func windowTxs(data []byte) ([]sqldb.WindowTx, error) {
	raw := &types.BlockData{}
	if err := raw.UnmarshallBlock(data); err != nil {
		return nil, err
	}
	txs := make([]sqldb.WindowTx, len(raw.TxFullData))
	for i, full := range raw.TxFullData {
		hash, signed, err := transaction.WindowEntry(full)
		if err != nil {
			return nil, err
		}
		txs[i] = sqldb.WindowTx{Hash: hash, Signed: signed}
	}
	return txs, nil
}

// windowTxs is the transactions of the block as the window of the duplicates takes them
func (b *Block) windowTxs() []sqldb.WindowTx {
	txs := make([]sqldb.WindowTx, len(b.Transactions))
	for i, t := range b.Transactions {
		txs[i] = sqldb.WindowTx{Hash: t.Hash(), Signed: t.SignedTime()}
	}
	return txs
}

// hashes is the hashes of the transactions of the block
func (b *Block) hashes() [][]byte {
	hashes := make([][]byte, len(b.Transactions))
	for i, t := range b.Transactions {
		hashes[i] = t.Hash()
	}
	return hashes
}

// addToWindow adds the transactions of the block, committed, to the window of the duplicates: a
// transaction of an ordered block spends its hash whether its execution runs or discards it
func (b *Block) addToWindow() {
	if err := sqldb.TxWindow.Add(b.Header.Timestamp, b.windowTxs()); err != nil {
		b.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("loading the window of the duplicates")
	}
}

// ExecHashAt is the execution hash of the block of the height as this node executed it, which the
// block BftExecDepth heights after carries: none below block 1, ErrNotExecuted for a block the node
// has not executed
func ExecHashAt(height int64) ([]byte, error) {
	if height < 1 {
		return nil, nil
	}
	h := &sqldb.BftExecHash{}
	found, err := h.GetIn(nil, height)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w %d", ErrNotExecuted, height)
	}
	return h.Hash, nil
}

// StateHashAt is the state hash the block of the height carries, if this node has it (design 4.3):
// at the delay after a snapshot height, the hash of the snapshot, else none
func StateHashAt(height int64) ([]byte, error) {
	s := stateHeight(height)
	if s == 0 {
		return nil, nil
	}
	hash, status, err := states.get(s)
	if err != nil || status != stateDone {
		return nil, err
	}
	return hash, nil
}

// validatorAt is the key of the validator at the position in the validators set of the height:
// the validator that signed the header of an ordered block, whose position the header names
func validatorAt(dbTx *sqldb.DbTransaction, height, position int64) ([]byte, error) {
	s := &sqldb.BftValidatorSet{}
	found, err := s.GetAt(dbTx, height)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("no validators set of height %d", height)
	}
	set, err := bft.UnmarshalValidators(s.Data)
	if err != nil {
		return nil, err
	}
	if position < 0 || position > int64(^uint32(0)) {
		return nil, fmt.Errorf("block %d: no validator at position %d", height, position)
	}
	key, ok := set.Key(uint32(position))
	if !ok {
		return nil, fmt.Errorf("block %d: no validator at position %d of %d", height, position, set.N())
	}
	return key, nil
}

// checkOrderedSign checks the signature of the header of an ordered block by the key of the
// validator at its position in the set of its height, which the execution of the block
// consts.BftSetDelay before it stored
func (b *Block) checkOrderedSign() error {
	key, err := validatorAt(nil, b.Header.BlockId, b.Header.NodePosition)
	if err != nil {
		return err
	}
	if err := utils.CheckNodeSign(key, []byte(b.ForSign()), b.Header.Sign); err != nil {
		return fmt.Errorf("block %d: the signature of the header: %w", b.Header.BlockId, err)
	}
	return nil
}

// CheckProposal is valid(v) of a validator for the proposal of an ordered block at a height (F18
// design 3.2): what the block carries and the chain committed before it decide, the execution of
// the block takes no part. It parses, it is of the height, its hash is the hash and the hash of its
// content, it is of the network and of the version, it follows the last committed block in time,
// its header is signed by a validator of the height, it carries the execution hash of the block
// BftExecDepth before it as this node executed it and the state hash as this node has it, and its
// transactions verify, are within the limits of a block and are no duplicates, in the block or of
// a committed one. An empty block is valid: delayed contracts may be due at the height.
//
// The sync of the node commits the blocks of the others by their certificates while the consensus
// checks a proposal: a commit of the height puts the transactions of its block in the window of the
// duplicates, after its database transaction, and the check of the very block it committed found
// them duplicates. A proposal of a height the node committed before the check ends is stale,
// whatever the check found.
func CheckProposal(binData []byte, height int64, hash bft.Hash) error {
	if err := notCommitted(height); err != nil {
		return err
	}
	b, err := parseProposal(binData, height, hash)
	if err == nil {
		b.trial = true
		err = b.checkOrdered()
	}
	if stale := notCommitted(height); stale != nil {
		return stale
	}
	return err
}

// notCommitted is ErrStale if the node committed a block of the height
func notCommitted(height int64) error {
	committed, err := sqldb.CommittedHeight()
	if err != nil {
		return err
	}
	if committed >= height {
		return fmt.Errorf("%w %d: the last committed block is %d", ErrStale, height, committed)
	}
	return nil
}

// checkOrdered checks the header and the transactions of an ordered block (CheckProposal)
func (b *Block) checkOrdered() error {
	h := b.Header
	if h.Timestamp <= b.PrevHeader.Timestamp {
		return fmt.Errorf("%w: block %d at %d, not after block %d at %d", ErrIncorrectBlockTime,
			h.BlockId, h.Timestamp, b.PrevHeader.BlockId, b.PrevHeader.Timestamp)
	}
	if err := checkConsensusMode(h, syspar.IsHonorNodesOnly()); err != nil {
		return err
	}
	if err := b.checkOrderedSign(); err != nil {
		return err
	}
	exec, err := ExecHashAt(h.BlockId - consts.BftExecDepth)
	if err != nil {
		return err
	}
	if !bytes.Equal(h.ExecHash, exec) {
		return fmt.Errorf("block %d: the execution hash %x of block %d is not %x, that of this node", h.BlockId, h.ExecHash, h.BlockId-consts.BftExecDepth, exec)
	}
	if err := b.checkStateHash(); err != nil {
		return err
	}
	return b.checkOrderedTxs()
}

// checkOrderedTxs checks the transactions of an ordered block: at most max_tx_block of them in at
// most max_block_size, each within max_tx_size, verified, of a user at most max_block_user_transactions in the block and in
// an ecosystem, no StopNetwork transaction among others, valid at the time of the block and no
// duplicates, in the block or of a committed block
func (b *Block) checkOrderedTxs() error {
	txs := b.Transactions
	if len(txs) > syspar.GetMaxTxCount() {
		return fmt.Errorf("block %d: %d transactions, more than max_tx_block %d", b.Header.BlockId, len(txs), syspar.GetMaxTxCount())
	}
	if size := int64(len(b.BinData)); size > syspar.GetMaxBlockSize() {
		return fmt.Errorf("block %d: %d bytes, more than max_block_size %d", b.Header.BlockId, size, syspar.GetMaxBlockSize())
	}
	if err := checkStopNetworkAlone(txs); err != nil {
		return err
	}
	for i, err := range transaction.VerifyAll(txs) {
		if err != nil {
			return fmt.Errorf("block %d: transaction %x: %w", b.Header.BlockId, txs[i].Hash(), err)
		}
	}
	limits := transaction.NewLimits(transaction.GetLetOrder())
	for _, t := range txs {
		if err := limits.CheckLimit(t.Inner); err != nil {
			return fmt.Errorf("block %d: transaction %x: %w", b.Header.BlockId, t.Hash(), err)
		}
	}
	return newTxChecks().check(b, txs)
}

// Commit stores the block a certificate commits and the certificate in one database transaction:
// a node has the certificate of a height if and only if it has its block. An ordered block is not
// executed: the executor of the node executes it after (Execute); its transactions are spent at
// once, in the window of the duplicates, and out of the pool. Block 1 is executed as it is
// committed.
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
	if b.IsGenesis() {
		return b, b.playSafe(c.Create)
	}
	if !b.ordered() {
		return nil, fmt.Errorf("block %d: version %d, not %d", b.Header.BlockId, b.Header.Version, consts.BftBlockVersion)
	}
	if err := b.commitOrdered(c); err != nil {
		return nil, err
	}
	decoded.commit(cert.Hash)
	return b, nil
}

// commitOrdered writes the ordered block and its certificate, and spends its transactions
func (b *Block) commitOrdered(c *sqldb.BftCertificate) error {
	dbTx, err := sqldb.StartTransaction()
	if err != nil {
		return err
	}
	bc := &sqldb.BlockChain{
		ID:           b.Header.BlockId,
		Hash:         b.Header.BlockHash,
		Data:         b.BinData,
		EcosystemID:  b.Header.EcosystemId,
		KeyID:        b.Header.KeyId,
		NodePosition: b.Header.NodePosition,
		Time:         b.Header.Timestamp,
		Tx:           int32(len(b.TxFullData)),
		// the execution of the block writes its rollbacks hash (writeExecuted)
		RollbacksHash:  []byte{},
		ConsensusMode:  b.Header.ConsensusMode,
		CandidateNodes: b.Header.CandidateNodes,
	}
	if err := bc.Create(dbTx); err != nil {
		dbTx.Rollback()
		return fmt.Errorf("writing block %d: %w", b.Header.BlockId, err)
	}
	if err := c.Create(dbTx); err != nil {
		dbTx.Rollback()
		return fmt.Errorf("writing the certificate of block %d: %w", b.Header.BlockId, err)
	}
	if err := dbTx.Commit(); err != nil {
		return err
	}
	b.addToWindow()
	transaction.TxPool.Remove(b.hashes())
	return nil
}

// Committed is the committed block of the height, to execute, of the header of the block before
// it: the block the node parsed when it committed it, else parsed from block_chain
func Committed(height int64) (*Block, error) {
	if e := decoded.take(height); e != nil {
		metrics.BlockDecodes.WithLabelValues(useExecute, "cached").Inc()
		return e.block()
	}
	metrics.BlockDecodes.WithLabelValues(useExecute, "parsed").Inc()
	bc := &sqldb.BlockChain{}
	found, err := bc.Get(height)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("block %d is not committed", height)
	}
	return ProcessBlockByBinData(bc.Data, false)
}

// checkExecHash checks, before the execution of a committed ordered block, that the execution hash
// it carries, which the validators agreed on, is that of the execution of this node: a node whose
// execution differs from that of the chain executes no further (ErrExecHash)
func (b *Block) checkExecHash() error {
	exec, err := ExecHashAt(b.Header.BlockId - consts.BftExecDepth)
	if err != nil {
		return err
	}
	if !bytes.Equal(b.Header.ExecHash, exec) {
		return fmt.Errorf("%w: block %d carries %x of block %d, this node has %x", ErrExecHash, b.Header.BlockId,
			b.Header.ExecHash, b.Header.BlockId-consts.BftExecDepth, exec)
	}
	return nil
}
