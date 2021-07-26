/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import (
	"fmt"
)

// Cron represents record of {prefix}_cron table
type Cron struct {
	tableName string
	ID        int64
	Cron      string
	Contract  string
}

	return isFound(DBConn.Where("id = ?", id).First(c))
}

// GetAllCronTasks is returning all cron tasks
func (c *Cron) GetAllCronTasks() ([]*Cron, error) {
	var crons []*Cron
	err := DBConn.Table(c.TableName()).Find(&crons).Error
	return crons, err
}

// UID returns unique identifier for cron task
func (c *Cron) UID() string {
	return fmt.Sprintf("%s_%d", c.tableName, c.ID)
}
