	Minerid      int64 `gorm:"not null"`
	Poolid       int64 `gorm:"not null"`
	Status       int64 `gorm:"null"`            //
	Online       int64 `gorm:"null default 0" ` //
	MineCapacity int64 `gorm:"null default 0" ` //
	Count        int64 `gorm:"null default 0" ` //
	Stime        int64 `gorm:"not null" `       //
	Etime        int64 `gorm:"not null" `       //
}

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
