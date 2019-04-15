/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import "time"

type NodeBanLogs struct {
	ID       int64
	BannedAt time.Time
	BanTime  time.Duration
	Reason   string
}

