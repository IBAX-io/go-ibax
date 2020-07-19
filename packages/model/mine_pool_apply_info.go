package model

import (
	"github.com/shopspring/decimal"
)

//MinePoolApplyInfo example
type MinePoolApplyInfo struct {
	Date_review      int64           `gorm:"not null" ` //
	Date_created     int64           `gorm:"not null" ` //
}

// TableName returns name of table
func (m MinePoolApplyInfo) TableName() string {
	return `1_mine_pool_apply_info`
}

// Get is retrieving model from database
func (m *MinePoolApplyInfo) Get(id int64) (bool, error) {
	return isFound(DBConn.Where("id = ?", id).First(m))
}

// Get is retrieving model from database
func (m *MinePoolApplyInfo) GetPool(keyid int64) (bool, error) {
	return isFound(DBConn.Where("keyid = ? and  review = ?", keyid, 2).Last(m))
}
