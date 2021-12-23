/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"time"
)

type BadBlocks struct {
	ID             int64
	ProducerNodeId int64
	BlockId        int64
	ConsumerNodeId int64
	BlockTime      time.Time
	Deleted        bool
}

// TableName returns name of table
func (r BadBlocks) TableName() string {
	return "1_bad_blocks"
}

// BanRequests represents count of unique ban requests for node
type BanRequests struct {
	ProducerNodeId int64
	Count          int64
}

// GetNeedToBanNodes is returns list of ban requests for each node
func (r *BadBlocks) GetNeedToBanNodes(now time.Time, blocksPerNode int) ([]BanRequests, error) {
	var res []BanRequests

	err := DBConn.
		Raw(
			`SELECT
				producer_node_id,
				COUNT(consumer_node_id) as count
			FROM (
				SELECT
					producer_node_id,
					consumer_node_id,
					count(DISTINCT block_id)
				FROM
				"1_bad_blocks"
				WHERE
					block_time > ?::date - interval '24 hours'
					AND deleted = 0
				GROUP BY
					producer_node_id,
					consumer_node_id
				HAVING
					count(DISTINCT block_id) >= ?) AS tbl
			GROUP BY
			producer_node_id`,
			now,
			blocksPerNode,
		).
		Scan(&res).
		Error

	return res, err
}

func (r *BadBlocks) GetNodeBlocks(nodeId int64, now time.Time) ([]BadBlocks, error) {
	var res []BadBlocks
	err := DBConn.
		Table(r.TableName()).
		Model(&BadBlocks{}).
		Where(
			"producer_node_id = ? AND block_time > ?::date - interval '24 hours' AND deleted = ?",
			nodeId,
			now,
			false,
		).
		Scan(&res).
		Error

	return res, err
}
