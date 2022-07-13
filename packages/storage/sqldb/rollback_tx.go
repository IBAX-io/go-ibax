/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"bytes"
	"encoding/json"

	"gorm.io/gorm"
)

// RollbackTx is model
type RollbackTx struct {
	ID        int64  `gorm:"primary_key;not null"`
	BlockID   int64  `gorm:"not null" json:"block_id"`
	TxHash    []byte `gorm:"not null" json:"tx_hash"`
	NameTable string `gorm:"not null;size:255;column:table_name" json:"table_name"`
	TableID   string `gorm:"not null;size:255" json:"table_id"`
	Data      string `gorm:"not null;type:jsonb" json:"data"`
	DataHash  []byte `gorm:"not null" json:"data_hash"`
}

// TableName returns name of table
func (*RollbackTx) TableName() string {
	return "rollback_tx"
}

// GetRollbackTransactions is returns rollback transactions
func (rt *RollbackTx) GetRollbackTransactions(dbTx *DbTransaction, transactionHash []byte) ([]map[string]string, error) {
	return dbTx.GetAllTransaction("SELECT * from rollback_tx WHERE tx_hash = ? ORDER BY ID DESC", -1, transactionHash)
}

// GetBlockRollbackTransactions returns records of rollback by blockID
func (rt *RollbackTx) GetBlockRollbackTransactions(dbTx *DbTransaction, blockID int64) ([]RollbackTx, error) {
	var rollbackTransactions []RollbackTx
	err := GetDB(dbTx).Where("block_id = ?", blockID).Omit("id").Order("id asc").Find(&rollbackTransactions).Error
	return rollbackTransactions, err
}

// GetRollbackTxsByTableIDAndTableName returns records of rollback by table name and id
func (rt *RollbackTx) GetRollbackTxsByTableIDAndTableName(tableID, tableName string, limit int) (*[]RollbackTx, error) {
	rollbackTx := new([]RollbackTx)
	if err := DBConn.Where("table_id = ? AND table_name = ?", tableID, tableName).
		Order("id desc").Limit(limit).Find(rollbackTx).Error; err != nil {
		return nil, err
	}
	return rollbackTx, nil
}

// DeleteByHash is deleting rollbackTx by hash
func (rt *RollbackTx) DeleteByHash(dbTx *DbTransaction) error {
	return GetDB(dbTx).Exec("DELETE FROM rollback_tx WHERE tx_hash = ?", rt.TxHash).Error
}

// DeleteByHashAndTableName is deleting tx by hash and table name
func (rt *RollbackTx) DeleteByHashAndTableName(dbTx *DbTransaction) error {
	return GetDB(dbTx).Where("tx_hash = ? and table_name = ?", rt.TxHash, rt.NameTable).Delete(rt).Error
}

func CreateBatchesRollbackTx(dbTx *gorm.DB, rts []*RollbackTx) error {
	if len(rts) == 0 {
		return nil
	}
	rollbackSys := &RollbackTx{}
	var err error
	if rollbackSys.ID, err = NewDbTransaction(dbTx).GetNextID(rollbackSys.TableName()); err != nil {
		return err
	}
	for i := 1; i < len(rts)+1; i++ {
		rts[i-1].ID = rollbackSys.ID + int64(i) - 1
	}
	return dbTx.Model(&RollbackTx{}).Create(&rts).Error
}

// Get is retrieving model from database
func (rt *RollbackTx) Get(dbTx *DbTransaction, transactionHash []byte, tableName string) (bool, error) {
	return isFound(GetDB(dbTx).Where("tx_hash = ? AND table_name = ?", transactionHash,
		tableName).Order("id desc").First(rt))
}

func (rt *RollbackTx) GetRollbacksDiff(dbTx *DbTransaction, blockID int64) ([]byte, error) {
	list, err := rt.GetBlockRollbackTransactions(dbTx, blockID)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	for _, rtx := range list {
		if err = enc.Encode(&rtx); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
