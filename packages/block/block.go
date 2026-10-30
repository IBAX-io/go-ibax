/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"time"

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
	// errTxTooLong is a transaction that does not end within the time a block of a chain of the
	// BFT consensus leaves its first transaction
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
	trial             bool             // the block is executed by Trial and held for the decision
	// genStart is when the proposer of a chain of the BFT consensus read the clock for the block:
	// the block is made within max_block_generation_time from then (syspar.GetBftTimeliness), its
	// transactions leaving the time the block takes after them
	genStart time.Time
	// genStopped is that the time left to the block stopped a transaction of it
	genStopped bool
	// execEnd is when the transactions of the execution running ended, before it writes their
	// state
	execEnd time.Time
	// prof is the profile of the execution running
	prof *execProfile
	// rollbacksHash is the hash of the rollback records and the outputs the block wrote
	rollbacksHash []byte
	// onCommit takes each transaction the block the node generates commits, in their order
	onCommit func(t *transaction.Transaction)
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
	return transaction.GetLetParsing()
}

// InsertBlockWOForks is inserting blocks
// InsertBlockWOForksNew inserts a block: a block the node generates runs txs, the parsed transactions
// of its data in its order
func InsertBlockWOForksNew(data []byte, txs []*transaction.Transaction, genBlock, firstBlock bool) error {
	block, err := ProcessBlockByBinData(data, !firstBlock)
	if err != nil {
		return err
	}

	block.GenBlock = genBlock
	if genBlock {
		block.Transactions = txs
	}
	if err := block.Check(); err != nil {
		return err
	}

	err = block.PlaySafe()
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{"block_id": block.Header.BlockId}).Debug("block was inserted successfully")
	return nil
}
