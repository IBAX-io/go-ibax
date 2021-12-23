package sqldb

import (
	"database/sql"
	"encoding/hex"
	"fmt"

	"gorm.io/gorm"
)

func GetNodeRows(tableName string) (int64, error) {
	var count int64
	err := DBConn.Table(tableName).Count(&count).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return count, nil
}

func GetRowsInfo(rows *sql.Rows, sqlQuest string) ([]map[string]string, error) {
	var result []map[string]string
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return result, fmt.Errorf("getrows Columns err:%s in query %s", err, sqlQuest)
	}
	columntypes, err1 := rows.ColumnTypes()
	if err1 != nil {
		return result, fmt.Errorf("getRows ColumnTypes err:%s in query %s", err1, sqlQuest)
	}
	values := make([][]byte /*sql.RawBytes*/, len(columns))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}
	for rows.Next() {
		err = rows.Scan(scanArgs...)
		if err != nil {
			return result, fmt.Errorf("getRows scan err:%s in query %s", err, sqlQuest)
		}
		var value string
		rez := make(map[string]string)
		for i, col := range values {
			// Here we can check if the value is nil (NULL value)
			if col == nil {
				value = "NULL"
			} else {
				if columntypes[i].DatabaseTypeName() == "BYTEA" {
					value = hex.EncodeToString(col)
				} else {
					value = string(col)
				}
			}
			rez[columns[i]] = value
		}
		result = append(result, rez)

	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("getRows rows err:%s in query %s", err, sqlQuest)
	}
	return result, nil
}
