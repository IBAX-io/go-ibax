/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"reflect"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

func GetTxRecord(tx *DbTransaction, hashStr string) (resultList []interface{}, err error) {
	db := GetDB(tx)
	// get record from rollback_tx
	var (
		rollbackTxs []RollbackTx
	)
	err = db.Table("rollback_tx").Where("tx_hash = ?", []byte(converter.HexToBin(hashStr))).Find(&rollbackTxs).Error
	if err != nil {
		return
	}
	for _, rtx := range rollbackTxs {
		id := rtx.TableID
		tableName := rtx.NameTable
		if tableName == `1_keys` {
			continue
		}
		rows, err := db.Table(tableName).Exec(`select * from "` + tableName + `" where id = ` + id).Rows()
		defer rows.Close()
		if err == nil {
			cols, er := rows.Columns()
			if er != nil {
				continue
			}
			values := make([][]byte, len(cols))
			scanArgs := make([]interface{}, len(values))
			for i := range values {
				scanArgs[i] = &values[i]
			}
			for rows.Next() {
				err = rows.Scan(scanArgs...)
				if err == nil {
					row := make(map[string]interface{})
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
