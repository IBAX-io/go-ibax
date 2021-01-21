package model

type MineInvitepowadd struct {
	ID           int64 `gorm:"primary_key;not null"`
	Devid        int64 `gorm:"not null"`
	Ydevid       int64 `gorm:"not null"`
	Count        int64 `gorm:"not null"`
	Type         int64 `gorm:"not null"`
	Stime        int64 `gorm:"not null"`
	Etime        int64 `gorm:"not null"`
	Date_created int64 `gorm:"not null"`
}

// TableName returns name of table
func (m MineInvitepowadd) TableName() string {
