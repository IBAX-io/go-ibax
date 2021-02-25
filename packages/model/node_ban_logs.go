/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
	Reason   string
}

// TableName returns name of table
func (r NodeBanLogs) TableName() string {
	return "1_node_ban_logs"
}
