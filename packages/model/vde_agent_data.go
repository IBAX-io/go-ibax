/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEAgentData struct {
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
	DataSendState  int64  `gorm:"not null" json:"data_send_state"`
	DataSendErr    string `gorm:"not null" json:"data_send_err"`
	CreateTime     int64  `gorm:"not null" json:"create_time"`
}

func (VDEAgentData) TableName() string {
	return "vde_agent_data"
}

func (m *VDEAgentData) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEAgentData) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEAgentData) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEAgentData) GetAll() ([]VDEAgentData, error) {
	var result []VDEAgentData
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEAgentData) GetOneByID() (*VDEAgentData, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
func (m *VDEAgentData) GetOneByDataUUID(DataUUID string) (*VDEAgentData, error) {
	err := DBConn.Where("data_uuid=?", DataUUID).First(&m).Error
	return m, err
}
func (m *VDEAgentData) GetOneByTaskUUID(TaskUUID string) (*VDEAgentData, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}
func (m *VDEAgentData) GetAllByTaskUUID(TaskUUID string) ([]VDEAgentData, error) {
	result := make([]VDEAgentData, 0)
	err := DBConn.Table("vde_agent_data").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDEAgentData) GetAllByDataSendStatus(DataSendStatus int64) ([]VDEAgentData, error) {
	result := make([]VDEAgentData, 0)
	err := DBConn.Table("vde_agent_data").Where("data_send_state = ?", DataSendStatus).Find(&result).Error
	return result, err
}

func (m *VDEAgentData) GetOneByDataStatus(DataStatus int64) (bool, error) {
	return isFound(DBConn.Where("data_state = ?", DataStatus).First(m))
}
