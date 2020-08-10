package model

import (
	"github.com/shopspring/decimal"
)

//MinePoolInfo example
type MinePoolInfo struct {
	Id               int64           `gorm:"not null" ` //
	Poolid           int64           `gorm:"not null"`  //
	LogoId           int64           `gorm:"not null" ` //logo
	Name             string          `gorm:"not null" ` //
	SettlementType   int64           `gorm:"not null" ` //  1 pps   2  pplns
	SettlementRate   float64         `gorm:"not null" ` //
	SettlementAmount decimal.Decimal `gorm:"not null `  //
	SettlementCycle  int64           `gorm:"not null" ` //
	Status           int64           `gorm:"not null" ` //
	HomeUrl          string          `gorm:"null" `     //
	Date_created     int64           `gorm:"not null" ` //
}

// TableName returns name of table
func (m MinePoolInfo) TableName() string {
	return `1_mine_pool_info`
}

// Get is retrieving model from database
func (m *MinePoolInfo) Get(id int64) (bool, error) {
	return isFound(DBConn.Where("id = ?", id).First(m))
}

// GetAllMinePool is returning all pools
func (m *MinePoolInfo) GetAllMinePoolInfos(dbt *DbTransaction) ([]MinePoolInfo, error) {
	var pools []MinePoolInfo
	err := GetDB(dbt).Table(m.TableName()).Find(&pools).Error
