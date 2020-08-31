package model

type MineCount struct {
	Devid        int64 `gorm:";not null" ` //
	Keyid        int64 `gorm:"not null" `  //

// TableName returns name of table
func (MineCount) TableName() string {
	return `1_v_miner_count`
}

func (m *MineCount) GetActiveMiner(dbt *DbTransaction, time int64) ([]MineCount, error) {
	var mp []MineCount
	err := GetDB(dbt).Table(m.TableName()).
		Where("stime <= ? and etime >=? and (status = ? or status = ?) and online = ?", time, time, 0, 2, 1).
		Order("poolid asc, devid asc").
		Find(&mp).Error
	return mp, err
}
