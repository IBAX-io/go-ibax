/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"time"
)

// BlockChain is model
type BlockChain struct {
	ID             int64  `gorm:"primary_key;not_null"`
	Hash           []byte `gorm:"not null"`
	RollbacksHash  []byte `gorm:"not null"`
	Data           []byte `gorm:"not null"`
	EcosystemID    int64  `gorm:"not null"`
	KeyID          int64  `gorm:"not null"`
	NodePosition   int64  `gorm:"not null"`
	Time           int64  `gorm:"not null"`
	Tx             int32  `gorm:"not null"`
	ConsensusMode  int32  `gorm:"not null"`
	CandidateNodes []byte `gorm:"not null"`
}

// TableName returns name of table
func (BlockChain) TableName() string {
	return "block_chain"
}

// Create is creating record of model
func (b *BlockChain) Create(dbTx *DbTransaction) error {
	return GetDB(dbTx).Create(b).Error
}

// Get is retrieving model from database
func (b *BlockChain) Get(blockID int64) (bool, error) {
	return isFound(DBConn.Where("id = ?", blockID).First(b))
}

// GetMaxBlock returns last block existence
func (b *BlockChain) GetMaxBlock() (bool, error) {
	return isFound(DBConn.Last(b))
}

// GetMaxForeignBlock returns last block generated not by key_id
func (b *BlockChain) GetMaxForeignBlock(keyId int64) (bool, error) {
	return isFound(DBConn.Order("id DESC").Where("key_id != ?", keyId).First(b))
}

// GetBlockchain is retrieving chain of blocks from database
func GetBlockchain(startBlockID int64, endblockID int64, order ordering) ([]BlockChain, error) {
	var err error
	blockchain := new([]BlockChain)

	orderStr := "id " + string(order)
	query := DBConn.Model(&BlockChain{}).Order(orderStr)
	if endblockID > 0 {
		query = query.Where("id > ? AND id <= ?", startBlockID, endblockID).Find(&blockchain)
	} else {
		query = query.Where("id > ?", startBlockID).Find(&blockchain)
	}

	if query.Error != nil {
		return nil, err
	}
	return *blockchain, nil
}

// GetBlocks is retrieving limited chain of blocks from database
func (b *BlockChain) GetBlocks(startFromID int64, limit int) ([]BlockChain, error) {
	var err error
	blockchain := new([]BlockChain)
	if startFromID > 0 {
		err = DBConn.Order("id desc").Limit(limit).Where("id > ?", startFromID).Find(&blockchain).Error
	} else {
		err = DBConn.Order("id desc").Limit(limit).Find(&blockchain).Error
	}
	return *blockchain, err
}

// GetBlocksFrom is retrieving ordered chain of blocks from database
func (b *BlockChain) GetBlocksFrom(startFromID int64, ordering string, limit int) ([]BlockChain, error) {
	blockchain := new([]BlockChain)
	q := DBConn.Model(&BlockChain{}).Order("id "+ordering).Where("id > ?", startFromID)
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&blockchain).Error
	return *blockchain, err
}

// GetReverseBlockchain returns records of blocks in reverse ordering
func (b *BlockChain) GetReverseBlockchain(endBlockID int64, limit int) ([]BlockChain, error) {
	var err error
	blockchain := new([]BlockChain)
	err = DBConn.Model(&BlockChain{}).Order("id DESC").Where("id <= ?", endBlockID).Limit(limit).Find(&blockchain).Error
	return *blockchain, err
}

// GetNodeBlocksAtTime returns records of blocks for time interval and position of node
func (b *BlockChain) GetNodeBlocksAtTime(from, to time.Time, node int64) ([]BlockChain, error) {
	var err error
	blockchain := new([]BlockChain)
	err = DBConn.Model(&BlockChain{}).Where("node_position = ? AND time BETWEEN ? AND ?", node, from.Unix(), to.Unix()).Find(&blockchain).Error
	return *blockchain, err
}

// DeleteById is deleting block by ID
func (b *BlockChain) DeleteById(dbTx *DbTransaction, id int64) error {
	return GetDB(dbTx).Where("id = ?", id).Delete(BlockChain{}).Error
}

func GetTxCount() (int64, error) {
	var txCount int64
	row := DBConn.Raw("SELECT SUM(tx) tx_count FROM block_chain").Select("tx_count").Row()
	err := row.Scan(&txCount)

	return txCount, err
}

func GetBlockCountByNode(NodePosition int64) (int64, error) {
	var BlockCount int64
	row := DBConn.Raw("SELECT count(*) block_count FROM block_chain where node_Position = ?", NodePosition).Select("block_count").Row()
	err := row.Scan(&BlockCount)

	return BlockCount, err
}
func (b *BlockChain) GetRecentBlockChain(startBlockId int64, maxBlockId int64) ([]BlockChain, error) {
	blockchain := new([]BlockChain)
	err := DBConn.Where("id > ? and id <= ?", startBlockId, maxBlockId).Find(&blockchain).Error

	return *blockchain, err
}
