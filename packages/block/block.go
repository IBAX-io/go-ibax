/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
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
)

// Block is storing block data
type Block struct {
	*types.BlockData
	PrevRollbacksHash []byte
	Transactions      []*transaction.Transaction
	GenBlock          bool // it equals true when we are generating a new block
	Notifications     []types.Notifications
	OutputsMap        map[sqldb.KeyUTXO][]sqldb.SpentInfo
	ClassifyTxsMap    map[int][]*transaction.Transaction
	PrevSysPar        map[string]string
	ComPercents       map[int64]int64 // combustion percent for each ecosystem
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
func InsertBlockWOForksNew(data []byte, classifyTxsMap map[int][]*transaction.Transaction, genBlock, firstBlock bool) error {
	block, err := ProcessBlockByBinData(data, !firstBlock)
	if err != nil {
		return err
	}

	block.GenBlock = genBlock
	if !firstBlock {
		block.ClassifyTxsMap = classifyTxsMap
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
