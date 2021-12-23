/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import "time"

type NodeBanLogs struct {
	ID       int64
	BannedAt time.Time
	BanTime  time.Duration
	Reason   string
}

// TableName returns name of table
func (r NodeBanLogs) TableName() string {
	return "1_node_ban_logs"
}
