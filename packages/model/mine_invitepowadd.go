package model

type MineInvitepowadd struct {
	ID           int64 `gorm:"primary_key;not null"`
	Devid        int64 `gorm:"not null"`
	Ydevid       int64 `gorm:"not null"`
	Count        int64 `gorm:"not null"`
	Type         int64 `gorm:"not null"`
		Order("devid asc").
		Find(&mp).Error
	return mp, err
}
