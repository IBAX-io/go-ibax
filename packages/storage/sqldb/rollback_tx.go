/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"gorm.io/gorm"
)

// RollbackTx is model. Its rows are found by the block (the primary key, block_id and id) and by the
// row of a table (table_name, table_id); the transaction of a row is found in its block
type RollbackTx struct {
	ID        int64  `gorm:"primary_key;not null"`
	BlockID   int64  `gorm:"not null" json:"block_id"`
	TxHash    []byte `gorm:"not null" json:"tx_hash"`
	NameTable string `gorm:"not null;size:255;column:table_name" json:"table_name"`
	TableID   string `gorm:"not null;size:255" json:"table_id"`
	Data      string `gorm:"not null;type:jsonb" json:"data"`
}

// TableName returns name of table
func (*RollbackTx) TableName() string {
	return "rollback_tx"
}

// GetRollbackTransactions returns the rollback records of the block, the last first, by the hex of
// the hashes of the transactions they are of
func (rt *RollbackTx) GetRollbackTransactions(dbTx *DbTransaction, blockID int64) (map[string][]map[string]string, error) {
	rows, err := dbTx.GetAllTransaction(`SELECT id, tx_hash, table_name, table_id, data FROM rollback_tx
		WHERE block_id = ? ORDER BY block_id DESC, id DESC`, -1, blockID)
	if err != nil {
		return nil, err
	}
	byTx := make(map[string][]map[string]string)
	for _, r := range rows {
		byTx[r["tx_hash"]] = append(byTx[r["tx_hash"]], r)
	}
	return byTx, nil
}

// GetBlockRollbackTransactions returns records of rollback by blockID
func (rt *RollbackTx) GetBlockRollbackTransactions(dbTx *DbTransaction, blockID int64) ([]RollbackTx, error) {
	var rollbackTransactions []RollbackTx
	err := GetDB(dbTx).Where("block_id = ?", blockID).Omit("id").Order("id asc").Find(&rollbackTransactions).Error
	return rollbackTransactions, err
}

// GetRollbackTxsByTableIDAndTableName returns records of rollback by table name and id: those in
// the window for a contract, wherever they are for the API (archive)
func (rt *RollbackTx) GetRollbackTxsByTableIDAndTableName(tableID, tableName string, limit int, archive bool) (*[]RollbackTx, error) {
	var rollbackTx []RollbackTx
	db := DBConn.Table(rt.TableName())
	if archive {
		db = archived(rt.TableName())
	}
	if err := db.Where("table_id = ? AND table_name = ?", tableID, tableName).
		Order("id desc").Limit(limit).Find(&rollbackTx).Error; err != nil {
		return nil, err
	}
	return &rollbackTx, nil
}

// DeleteBlockRollbackTransactions deletes the rollback records of the block
func DeleteBlockRollbackTransactions(dbTx *DbTransaction, blockID int64) error {
	return GetDB(dbTx).Exec("DELETE FROM rollback_tx WHERE block_id = ?", blockID).Error
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
	copied, err := copyInto(dbTx, "rollback_tx",
		[]string{"id", "block_id", "tx_hash", "table_name", "table_id", "data"},
		func() []stateRow {
			rows := make([]stateRow, len(rts))
			for i, r := range rts {
				rows[i] = stateRow{textInt(r.ID), textInt(r.BlockID), textBytea(r.TxHash), textString(r.NameTable),
					textString(r.TableID), textString(r.Data)}
			}
			return rows
		})
	if copied || err != nil {
		return err
	}
	return dbTx.Exec(`INSERT INTO "rollback_tx" ("id", "block_id", "tx_hash", "table_name", "table_id", "data")
		SELECT * FROM unnest(?::bigint[], ?::bigint[], ?::bytea[], ?::varchar[], ?::varchar[], ?::text[])`,
		column(rts, func(r *RollbackTx) int64 { return r.ID }),
		column(rts, func(r *RollbackTx) int64 { return r.BlockID }),
		column(rts, func(r *RollbackTx) []byte { return r.TxHash }),
		column(rts, func(r *RollbackTx) string { return r.NameTable }),
		column(rts, func(r *RollbackTx) string { return r.TableID }),
		column(rts, func(r *RollbackTx) string { return r.Data })).Error
}
