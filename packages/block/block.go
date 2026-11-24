/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"time"

	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var (
	ErrIncorrectRollbackHash = errors.New("Rollback hash doesn't match")
	ErrEmptyBlock            = errors.New("Block doesn't contain transactions")
	ErrIncorrectBlockTime    = utils.WithBan(errors.New("Incorrect block time"))
	// errTxTooLong is a transaction that does not end within max_block_generation_time, the time
	// a block a node generates leaves its first transaction
	errTxTooLong = errors.New("the transaction does not end within the time a block leaves it")
	// errStopNetworkNotAlone: a StopNetwork transaction is the only transaction of its block
	errStopNetworkNotAlone = errors.New("a block with a StopNetwork transaction has other transactions")
)

// Block is storing block data
type Block struct {
	*types.BlockData
	PrevRollbacksHash []byte
	Transactions      []*transaction.Transaction
	GenBlock          bool // it equals true when we are generating a new block
	Notifications     []types.Notifications
	OutputsMap        map[sqldb.KeyUTXO][]sqldb.SpentInfo
	PrevSysPar        map[string]string
	EcoParams         []sqldb.EcoParam // combustion percent,digits for each ecosystem
	size              *blockSize       // the encoded length of the block being generated
	// trial is that the block is a proposal the node checks: it bans no key and marks no
	// transaction bad, and it does not wait for a state hash it has not
	trial bool
	// discards are the reasons of the transactions the execution of an ordered block discarded, by
	// their hashes (discardOf)
	discards map[string]int32
	// systemTxs is the number of the system transactions (delayed contracts) the execution of an
	// ordered block runs before the transactions of the block, at the head of Transactions
	systemTxs int
	// executed is the execution hash the execution of an ordered block left (execHash)
	executed []byte
	// genStopped is that the time left to the block being generated stopped a transaction of it
	genStopped bool
	// prof is the profile of the execution running
	prof *execProfile
	// rollbacksHash is the hash of the rollback records and the outputs the block wrote
	rollbacksHash []byte
	// nextValidators is the encoded validators set of the next height the execution of the block
	// leaves, on a chain of the BFT consensus
	nextValidators []byte
}

// GetLogger is returns logger
func (b *Block) GetLogger() *log.Entry {
	return log.WithFields(log.Fields{"block_id": b.Header.BlockId, "block_time": b.Header.Timestamp, "block_wallet_id": b.Header.KeyId,
		"block_state_id": b.Header.EcosystemId, "block_hash": b.Header.BlockHash, "block_version": b.Header.Version})
}

func (b *Block) IsGenesis() bool {
	return b.Header.BlockId == 1
}

func (b *Block) limitMode() transaction.LimitMode {
	if b == nil {
		return transaction.GetLetPreprocess()
	}
	if b.GenBlock {
		return transaction.GetLetGenBlock()
	}
	if b.ordered() && !b.IsGenesis() {
		return transaction.GetLetExecute()
	}
	return transaction.GetLetParsing()
}

// InsertFirstBlock inserts block 1 of its data
func InsertFirstBlock(data []byte) error {
	block, err := ProcessBlockByBinData(data, false)
	if err != nil {
		return err
	}
	if err := block.Check(); err != nil {
		return err
	}
	return block.PlaySafe()
}

// InsertGenerated runs and inserts the block of header this node generates of the transactions
// txs, which processTransactionsNew took and checked at the time of the block. The block is not
// encoded before it runs: InsertIntoBlockchain encodes and signs it of the transactions that ran
func InsertGenerated(header *types.BlockHeader, txs []*transaction.Transaction) error {
	start := time.Now()
	prev, err := GetBlockHeaderFromBlockChain(header.BlockId - 1)
	if err != nil {
		return err
	}
	block := &Block{
		BlockData:         &types.BlockData{Header: header, PrevHeader: prev},
		PrevRollbacksHash: prev.RollbacksHash,
		Transactions:      txs,
		GenBlock:          true,
	}
	if err := block.checkGenerated(); err != nil {
		return err
	}
	metrics.BlockStage.WithLabelValues(metrics.KindGenerate, metrics.StageCheck).Observe(metrics.Since(start))
	if err := block.PlaySafe(); err != nil {
		return err
	}
	log.WithFields(log.Fields{"block_id": header.BlockId}).Debug("block was inserted successfully")
	return nil
}
