	LogoId           int64           `gorm:"not null" ` //logo
	Name             string          `gorm:"not null" ` //poolname
	SettlementType   int64           `gorm:"not null" ` // 1 pps   2  pplns
	SettlementRate   float64         `gorm:"not null" ` //rate
	SettlementAmount decimal.Decimal `gorm:"not null `  //min amount
	SettlementCycle  int64           `gorm:"not null" ` //time
	Review           int64           `gorm:"not null" ` //review
	HomeUrl          string          `gorm:"null" `     //home_url
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
