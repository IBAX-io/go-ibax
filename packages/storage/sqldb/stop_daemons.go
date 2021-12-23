/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"time"
)

// StopDaemon is model
type StopDaemon struct {
	StopTime int64 `gorm:"not null"`
}

// TableName returns name of table
func (sd *StopDaemon) TableName() string {
	return "stop_daemons"
}

// Create is creating record of model
func (sd *StopDaemon) Create() error {
	return DBConn.Create(sd).Error
}

// Delete is deleting record
func (sd *StopDaemon) Delete() error {
	return DBConn.Delete(&StopDaemon{}).Error
}

// Get is retrieving model from database
func (sd *StopDaemon) Get() (bool, error) {
	return isFound(DBConn.First(sd))
}

// SetStopNow is updating daemon stopping time to now
func SetStopNow() error {
	stopTime := &StopDaemon{StopTime: time.Now().Unix()}
	return stopTime.Create()
}
