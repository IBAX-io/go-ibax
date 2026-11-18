/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"github.com/IBAX-io/go-ibax/packages/converter"
	"gorm.io/gorm"
)

// RollbackTx is model. Its rows are found by the block (the primary key, block_id and id) and by the
// row of a table (table_ref, table_id). A record is of the transaction TxIdx of its block, the
// index of the transaction in the block, or the settlement of the fees, which follows them; its
// table is TableRef, the ref rollback_tables gives NameTable
type RollbackTx struct {
	ID        int64  `gorm:"primary_key;not null"`
	BlockID   int64  `gorm:"not null" json:"block_id"`
	TxIdx     int32  `gorm:"not null" json:"tx_idx"`
	TableRef  int32  `gorm:"not null" json:"table_ref"`
	NameTable string `gorm:"-" json:"table_name"`
	TableID   string `gorm:"not null;size:255" json:"table_id"`
	Data      string `gorm:"not null;type:jsonb" json:"data"`
}

// TableName returns name of table
func (*RollbackTx) TableName() string {
	return "rollback_tx"
}

// GetRollbackTransactions returns the rollback records of the block, the last first, with the names
// of their tables, by the indexes of the transactions they are of
func (rt *RollbackTx) GetRollbackTransactions(dbTx *DbTransaction, blockID int64) (map[int][]map[string]string, error) {
	rows, err := dbTx.GetAllTransaction(`SELECT r.id, r.block_id, r.tx_idx, t.name AS table_name, r.table_id, r.data
		FROM rollback_tx r JOIN rollback_tables t ON t.ref = r.table_ref
		WHERE r.block_id = ? ORDER BY r.block_id DESC, r.id DESC`, -1, blockID)
	if err != nil {
		return nil, err
	}
	byTx := make(map[int][]map[string]string)
	for _, r := range rows {
		idx := converter.StrToInt(r["tx_idx"])
		byTx[idx] = append(byTx[idx], r)
	}
	return byTx, nil
}

// GetRollbackTxsByTableIDAndTableName returns records of rollback by table name and id, the last
// first: those in the window for a contract, wherever they are for the API (archive)
func (rt *RollbackTx) GetRollbackTxsByTableIDAndTableName(tableID, tableName string, limit int, archive bool) (*[]RollbackTx, error) {
	var rollbackTx []RollbackTx
	db := DBConn.Table(rt.TableName())
	if archive {
		db = archived(rt.TableName())
	}
	if err := db.Where(`table_ref = (SELECT ref FROM rollback_tables WHERE name = ?) AND table_id = ?`, tableName, tableID).
		Order("block_id desc, id desc").Limit(limit).Find(&rollbackTx).Error; err != nil {
		return nil, err
	}
	for i := range rollbackTx {
		rollbackTx[i].NameTable = tableName
	}
	return &rollbackTx, nil
}

// DeleteBlockRollbackTransactions deletes the rollback records of the block and the refs of the
// tables the block gave
func DeleteBlockRollbackTransactions(dbTx *DbTransaction, blockID int64) error {
	db := GetDB(dbTx)
	if err := db.Exec("DELETE FROM rollback_tx WHERE block_id = ?", blockID).Error; err != nil {
		return err
	}
	return deleteTableRefs(db, blockID)
}

// CreateBatchesRollbackTx writes the rollback records rts of a block, which give the tables their
// refs through refs, the refs of the block
func CreateBatchesRollbackTx(dbTx *gorm.DB, rts []*RollbackTx, refs *TableRefs) error {
	if len(rts) == 0 {
		return nil
	}
	if err := refs.set(dbTx, rts); err != nil {
		return err
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
		[]string{"id", "block_id", "tx_idx", "table_ref", "table_id", "data"},
		func() []stateRow {
			rows := make([]stateRow, len(rts))
			for i, r := range rts {
				rows[i] = stateRow{textInt(r.ID), textInt(r.BlockID), textInt(int64(r.TxIdx)), textInt(int64(r.TableRef)),
					textString(r.TableID), textString(r.Data)}
			}
			return rows
		})
	if copied || err != nil {
		return err
	}
	return dbTx.Exec(`INSERT INTO "rollback_tx" ("id", "block_id", "tx_idx", "table_ref", "table_id", "data")
		SELECT * FROM unnest(?::bigint[], ?::bigint[], ?::integer[], ?::integer[], ?::varchar[], ?::text[])`,
		column(rts, func(r *RollbackTx) int64 { return r.ID }),
		column(rts, func(r *RollbackTx) int64 { return r.BlockID }),
		column(rts, func(r *RollbackTx) int32 { return r.TxIdx }),
		column(rts, func(r *RollbackTx) int32 { return r.TableRef }),
		column(rts, func(r *RollbackTx) string { return r.TableID }),
		column(rts, func(r *RollbackTx) string { return r.Data })).Error
}
