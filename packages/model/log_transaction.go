/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import "gorm.io/gorm"

// LogTransaction is model
type LogTransaction struct {
	Hash  []byte `gorm:"primary_key;not null"`
	Block int64  `gorm:"not null"`
}

// GetByHash returns LogTransactions existence by hash
func (lt *LogTransaction) GetByHash(hash []byte) (bool, error) {
	return isFound(DBConn.Where("hash = ?", hash).First(lt))
}

// Create is creating record of model
func (lt *LogTransaction) Create(transaction *DbTransaction) error {
	return GetDB(transaction).Create(lt).Error
		return -1, err
	}
	return rowsCount, nil
}
