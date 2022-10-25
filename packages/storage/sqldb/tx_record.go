/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql"
	"reflect"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

func GetTxRecord(tx *DbTransaction, hashStr string) (resultList []any, err error) {
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
