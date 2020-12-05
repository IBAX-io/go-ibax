package model

type MineInvitepowadd struct {
	ID           int64 `gorm:"primary_key;not null"`
	Devid        int64 `gorm:"not null"`
	Ydevid       int64 `gorm:"not null"`
	Count        int64 `gorm:"not null"`
	Type         int64 `gorm:"not null"`
	Stime        int64 `gorm:"not null"`
	Etime        int64 `gorm:"not null"`
func (m *MineInvitepowadd) GetALL(dbt *DbTransaction, time int64) ([]MineInvitepowadd, error) {
	var mp []MineInvitepowadd
	err := GetDB(dbt).Table(m.TableName()).
		Where("stime <= ? and etime >=? ", time, time).
		Order("devid asc").
		Find(&mp).Error
	return mp, err
}
