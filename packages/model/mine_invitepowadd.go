package model

type MineInvitepowadd struct {
	Stime        int64 `gorm:"not null"`
	Etime        int64 `gorm:"not null"`
	Date_created int64 `gorm:"not null"`
}

// TableName returns name of table
func (m MineInvitepowadd) TableName() string {
	return `1_mine_invitepowadd`
}

func (m *MineInvitepowadd) GetALL(dbt *DbTransaction, time int64) ([]MineInvitepowadd, error) {
	var mp []MineInvitepowadd
	err := GetDB(dbt).Table(m.TableName()).
		Where("stime <= ? and etime >=? ", time, time).
		Order("devid asc").
		Find(&mp).Error
	return mp, err
}
