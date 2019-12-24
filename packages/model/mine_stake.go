/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

import (
	"github.com/shopspring/decimal"
)

type MineStake struct {
	ID           int64           `gorm:"primary_key not null"`
	Number       int64           `gorm:"null" `      //number
	Devid        int64           `gorm:";not null" ` //devid
	Keyid        int64           `gorm:"not null" `  //keyid
	Poolid       int64           `gorm:"not null" `  //
	MineType     int64           `gorm:"not null"`
	MineNumber   string          `gorm:"not null"`
	MineCapacity int64           `gorm:"not null"`
	Cycle        int64           `gorm:"not null" `           //
	Amount       decimal.Decimal `gorm:"not null default 0" ` //
	Expired      int64           `gorm:"null" `
	Status       int64           `gorm:"null"`            //
	Review       int64           `gorm:"null default 0" ` //
	Count        int64           `gorm:"null default 0" ` //
	Stakes       int64           `gorm:"null default 0" ` //
	Transfers    int64           `gorm:"null"  `          //
	Stime        int64           `gorm:"not null" `       //
	Etime        int64           `gorm:"not null" `       //
	DateUpdated  int64           `gorm:"not null" `
	DateCreated  int64           `gorm:"not null" `
}

// TableName returns name of table
func (MineStake) TableName() string {
	return `1_mine_stake`
}

		Where("etime <=? and expired = 0", time).
		Order("etime asc").
		Limit(10).
		Scan(&mp).Error
	return mp, err
}

// Get is retrieving model from database
func (m *MineStake) UpdateExpired(t int64) error {
	m.Expired = 1
	m.DateUpdated = t
	return DBConn.Model(m).Updates(map[string]interface{}{"expired": m.Expired, "date_updated": m.DateUpdated}).Error
}

// Get is retrieving model from database
func (m *MineStake) Update() error {
	return DBConn.Save(m).Error
}
