/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

// QueueBlock is model
type QueueBlock struct {
	Hash        []byte `gorm:"primary_key;not null"`
	BlockID     int64  `gorm:"not null"`
	HonorNodeID int64  `gorm:"not null"`
}

// Get is retrieving model from database
func (qb *QueueBlock) Get() (bool, error) {
	return isFound(DBConn.First(qb))
}

// GetQueueBlockByHash is retrieving blocks queue by hash
func (qb *QueueBlock) GetQueueBlockByHash(hash []byte) (bool, error) {
	return isFound(DBConn.Where("hash = ?", hash).First(qb))
}
}

// Create is creating record of model
func (qb *QueueBlock) Create() error {
	return DBConn.Create(qb).Error
}
