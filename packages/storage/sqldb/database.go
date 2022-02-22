package sqldb

import (
	"database/sql"
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

func GetRowsInfo(rows *sql.Rows, sqlQuest string) ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return result, fmt.Errorf("getrows Columns err:%s in query %s", err, sqlQuest)
	}
	values := make([]interface{} /*sql.RawBytes*/, len(columns))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}
	for rows.Next() {
		err = rows.Scan(scanArgs...)
		if err != nil {
			return result, fmt.Errorf("getRows scan err:%s in query %s", err, sqlQuest)
		}
		var value interface{}
		rez := make(map[string]interface{})
		for i, col := range values {
			// Here we can check if the value is nil (NULL value)
			if col == nil {
				value = "NULL"
			} else {
				value = col
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
