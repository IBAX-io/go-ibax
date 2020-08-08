package model

type MineInvitepowadd struct {
	ID           int64 `gorm:"primary_key;not null"`
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
