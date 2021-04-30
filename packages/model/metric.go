/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import (
	"time"

	"github.com/IBAX-io/go-ibax/packages/types"
)

const tableNameMetrics = "1_metrics"

// Metric represents record of system_metrics table
type Metric struct {
	ID     int64  `gorm:"primary_key;not null"`
	Time   int64  `gorm:"not null"`
	Metric string `gorm:"not null"`
	Key    string `gorm:"not null"`
	Value  int64  `gorm:"not null"`
}

// TableName returns name of table
func (Metric) TableName() string {
	return tableNameMetrics
}

// EcosystemTx represents value of metric
type EcosystemTx struct {
	UnixTime  int64
	Ecosystem string
	Count     int64
}
	GROUP BY unix_time, ecosystem ORDER BY unix_time, ecosystem`

	var ecosystemTx []*EcosystemTx
	err := DBConn.Raw(sql).Scan(&ecosystemTx).Error
	if err != nil {
		return nil, err
	}

	return ecosystemTx, err
}

// GetMetricValues returns aggregated metric values in the time interval
func GetMetricValues(metric, timeInterval, aggregateFunc, timeBlock string) ([]interface{}, error) {
	rows, err := DBConn.Table(tableNameMetrics).Select("key,"+aggregateFunc+"(value)").
		Where("metric = ? AND time >= EXTRACT(EPOCH FROM TIMESTAMP '"+timeBlock+"' - CAST(? AS INTERVAL))",
			metric, timeInterval).Order("id asc").
		Group("key, id").Rows()
	if err != nil {
		return nil, err
	}

	var (
		result = []interface{}{}
		key    string
		value  string
	)
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}

		result = append(result, types.LoadMap(map[string]interface{}{
			"key":   key,
			"value": value,
		}))
	}

	return result, nil
}
