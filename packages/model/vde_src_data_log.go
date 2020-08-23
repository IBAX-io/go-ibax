/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDESrcDataLog struct {
	ID                  int64  `gorm:"primary_key; not null" json:"id"`
	BlockId    int64  `gorm:"not null" json:"block_id"`
	ChainId    int64  `gorm:"not null" json:"chain_id"`
	ChainErr   string `gorm:"not null" json:"chain_err"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDESrcDataLog) TableName() string {
	return "vde_src_data_log"
}

func (m *VDESrcDataLog) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcDataLog) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcDataLog) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcDataLog) GetAll() ([]VDESrcDataLog, error) {
	var result []VDESrcDataLog
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcDataLog) GetOneByID() (*VDESrcDataLog, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDESrcDataLog) GetAllByTaskUUID(TaskUUID string) ([]VDESrcDataLog, error) {
	result := make([]VDESrcDataLog, 0)
	err := DBConn.Table("vde_src_data_log").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDESrcDataLog) GetOneByTaskUUID(TaskUUID string) (*VDESrcDataLog, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}

func (m *VDESrcDataLog) GetAllByChainState(ChainState int64) ([]VDESrcDataLog, error) {
	result := make([]VDESrcDataLog, 0)
	err := DBConn.Table("vde_src_data_log").Where("chain_state = ?", ChainState).Find(&result).Error
	return result, err
}

func (m *VDESrcDataLog) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}
