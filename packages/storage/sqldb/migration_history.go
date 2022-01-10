/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"time"
)

const noVersion = "0.0.0"

// MigrationHistory is model
type MigrationHistory struct {
	ID          int64  `gorm:"primary_key;not null"`
	Version     string `gorm:"not null"`
	DateApplied int64  `gorm:"not null"`
}

// TableName returns name of table
func (mh *MigrationHistory) TableName() string {
	return "migration_history"
}

// CurrentVersion returns current version of database migrations
func (mh *MigrationHistory) CurrentVersion() (string, error) {
	if !NewDbTransaction(DBConn).IsTable(mh.TableName()) {
		return noVersion, nil
	}

	err := DBConn.Last(mh).Error

	if mh.Version == "" {
		return noVersion, nil
	}

	return mh.Version, err
}

// ApplyMigration executes database schema and writes migration history
func (mh *MigrationHistory) ApplyMigration(version, query string) error {
	err := DBConn.Exec(query).Error
	if err != nil {
		return err
	}

	return DBConn.Create(&MigrationHistory{Version: version, DateApplied: time.Now().Unix()}).Error
}
