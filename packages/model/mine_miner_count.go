package model

type MineCount struct {
	Devid        int64 `gorm:";not null" ` //
	Keyid        int64 `gorm:"not null" `  //
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
