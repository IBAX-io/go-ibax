/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import (
	"time"
)

// StopDaemon is model
type StopDaemon struct {
	StopTime int64 `gorm:"not null"`
}

// TableName returns name of table
// Get is retrieving model from database
func (sd *StopDaemon) Get() (bool, error) {
	return isFound(DBConn.First(sd))
}

// SetStopNow is updating daemon stopping time to now
func SetStopNow() error {
	stopTime := &StopDaemon{StopTime: time.Now().Unix()}
	return stopTime.Create()
}
