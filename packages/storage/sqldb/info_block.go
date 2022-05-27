/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"github.com/IBAX-io/go-ibax/packages/converter"
)

// InfoBlock is model
type InfoBlock struct {
	Hash           []byte `gorm:"not null"`
	EcosystemID    int64  `gorm:"not null default 0"`
	KeyID          int64  `gorm:"not null default 0"`
	NodePosition   string `gorm:"not null default 0"`
	BlockID        int64  `gorm:"not null"`
	Time           int64  `gorm:"not null"`
	CurrentVersion string `gorm:"not null"`
	Sent           int8   `gorm:"not null"`
	RollbacksHash  []byte `gorm:"not null"`
	ConsensusMode  int32  `gorm:"not null"`
	CandidateNodes []byte `gorm:"not null"`
}

// TableName returns name of table
func (ib *InfoBlock) TableName() string {
	return "info_block"
}

// Get is retrieving model from database
func (ib *InfoBlock) Get() (bool, error) {
	return isFound(DBConn.Last(ib))
}

// Update is update model
func (ib *InfoBlock) Update(dbTx *DbTransaction) error {
	return GetDB(dbTx).Model(&InfoBlock{}).Updates(ib).Error
}

// GetUnsent is retrieving model from database
func (ib *InfoBlock) GetUnsent() (bool, error) {
	return isFound(DBConn.Where("sent = ?", "0").First(&ib))
}

// Create is creating record of model
func (ib *InfoBlock) Create(dbTx *DbTransaction) error {
	return GetDB(dbTx).Create(ib).Error
}

// MarkSent update model sent field
func (ib *InfoBlock) MarkSent() error {
	return DBConn.Model(ib).Update("sent", 1).Error
}

// UpdRollbackHash update model rollbacks_hash field
func UpdRollbackHash(dbTx *DbTransaction, hash []byte) error {
	return GetDB(dbTx).Model(&InfoBlock{}).Update("rollbacks_hash", hash).Error
}

// BlockGetUnsent returns InfoBlock
func BlockGetUnsent() (*InfoBlock, error) {
	ib := &InfoBlock{}
	found, err := ib.GetUnsent()
	if !found {
		return nil, err
	}
	return ib, err
}

// Marshall returns block as []byte
func (ib *InfoBlock) Marshall() []byte {
	if ib != nil {
		toBeSent := converter.DecToBin(ib.BlockID, 3)
		return append(toBeSent, ib.Hash...)
	}
	return []byte{}
}
