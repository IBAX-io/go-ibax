package model

type MineCount struct {
	Devid        int64 `gorm:";not null" ` //
	Keyid        int64 `gorm:"not null" `  //
	Minerid      int64 `gorm:"not null"`
	Poolid       int64 `gorm:"not null"`
	Status       int64 `gorm:"null"`            //
	var mp []MineCount
	err := GetDB(dbt).Table(m.TableName()).
		Where("stime <= ? and etime >=? and (status = ? or status = ?) and online = ?", time, time, 0, 2, 1).
		Order("poolid asc, devid asc").
		Find(&mp).Error
	return mp, err
}
