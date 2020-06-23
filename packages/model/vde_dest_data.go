/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEDestData struct {
	ID             int64  `gorm:"primary_key; not null" json:"id"`
	DataUUID       string `gorm:"not null" json:"data_uuid"`
	TaskUUID       string `gorm:"not null" json:"task_uuid"`
	Hash           string `gorm:"not null" json:"hash"`
	Data           []byte `gorm:"not null" json:"data"`
	DataInfo       string `gorm:"type:jsonb" json:"data_info"`
	VDESrcPubkey   string `gorm:"not null" json:"vde_src_pubkey"`
	VDEDestPubkey  string `gorm:"not null" json:"vde_dest_pubkey"`
	VDEDestIp      string `gorm:"not null" json:"vde_dest_ip"`
	VDEAgentPubkey string `gorm:"not null" json:"vde_agent_pubkey"`
	VDEAgentIp     string `gorm:"not null" json:"vde_agent_ip"`
	AgentMode      int64  `gorm:"not null" json:"agent_mode"`
	DataState      int64  `gorm:"not null" json:"data_state"`
	UpdateTime     int64  `gorm:"not null" json:"update_time"`
	CreateTime     int64  `gorm:"not null" json:"create_time"`
}

func (VDEDestData) TableName() string {
	return "vde_dest_data"
}

func (m *VDEDestData) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEDestData) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEDestData) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEDestData) GetAll() ([]VDEDestData, error) {
	var result []VDEDestData
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEDestData) GetOneByID() (*VDEDestData, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
func (m *VDEDestData) GetOneByDataUUID(DataUUID string) (*VDEDestData, error) {
	err := DBConn.Where("data_uuid=?", DataUUID).First(&m).Error
	return m, err
}
func (m *VDEDestData) GetOneByTaskUUID(TaskUUID string) (*VDEDestData, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}
func (m *VDEDestData) GetAllByTaskUUID(TaskUUID string) ([]VDEDestData, error) {
	result := make([]VDEDestData, 0)
	err := DBConn.Table("vde_dest_data").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDEDestData) GetAllByDataStatus(DataStatus int64) ([]VDEDestData, error) {
	result := make([]VDEDestData, 0)
	err := DBConn.Table("vde_dest_data").Where("data_state = ?", DataStatus).Find(&result).Error
	return result, err
