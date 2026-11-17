/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql"
	"reflect"
	"strings"
)

// TxBlock returns the block of the transaction hash, wherever its log is (archive)
func TxBlock(hash []byte) (int64, bool, error) {
	var blocks []int64
	if err := DBConn.Raw(`SELECT block FROM `+ArchivedTable("log_transactions")+` WHERE hash = ? LIMIT 1`, hash).
		Scan(&blocks).Error; err != nil || len(blocks) == 0 {
		return 0, false, err
	}
	return blocks[0], true, nil
}

// GetTxRecord returns the rows the transaction txIdx of the block changed, as they are
func GetTxRecord(tx *DbTransaction, blockID int64, txIdx int) (resultList []any, err error) {
	db := GetDB(tx)
	var rollbackTxs []struct {
		NameTable string `gorm:"column:table_name"`
		TableID   string
	}
	err = db.Raw(`SELECT t.name AS table_name, r.table_id FROM `+ArchivedTable("rollback_tx")+` r
		JOIN rollback_tables t ON t.ref = r.table_ref WHERE r.block_id = ? AND r.tx_idx = ? ORDER BY r.id`,
		blockID, txIdx).Scan(&rollbackTxs).Error
	if err != nil {
		return
	}
	for _, rtx := range rollbackTxs {
		id := rtx.TableID
		var ecosystem string
		tableName := rtx.NameTable
		if tableName == `1_keys` || tableName == "@system" {
			continue
		}
		if strings.Contains(id, ",") {
			ids := strings.Split(id, ",")
			if len(ids) == 2 {
				id, ecosystem = ids[0], ids[1]
			}

		}
		var (
			rows *sql.Rows
			err  error
		)
		if ecosystem == "" {
			rows, err = db.Raw(`select * from "` + tableName + `" where id = ` + id).Rows()
		} else {
			rows, err = db.Raw(`select * from "` + tableName + `" where id = ` + id + " AND ecosystem = " + ecosystem).Rows()
		}
		defer rows.Close()
		if err == nil {
			cols, er := rows.Columns()
			if er != nil {
				continue
			}
			values := make([][]byte, len(cols))
			scanArgs := make([]any, len(values))
			for i := range values {
				scanArgs[i] = &values[i]
			}
			for rows.Next() {
				err = rows.Scan(scanArgs...)
				if err == nil {
					row := make(map[string]any)
					for i, col := range values {
						var value string
						if col != nil {
							value = string(col)
						}
						row[cols[i]] = value
					}
					resultList = append(resultList, reflect.ValueOf(row).Interface())
				}
			}
		}

	}

	return
}
