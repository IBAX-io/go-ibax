/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDESrcDataStatus struct {
	ID             int64  `gorm:"primary_key; not null" json:"id"`
	DataUUID       string `gorm:"not null" json:"data_uuid"`
	TaskUUID       string `gorm:"not null" json:"task_uuid"`
	Hash           string `gorm:"not null" json:"hash"`
	Data           []byte `gorm:"column:data;not null" json:"data"`
	DataInfo       string `gorm:"type:jsonb" json:"data_info"`
	VDESrcPubkey   string `gorm:"not null" json:"vde_src_pubkey"`
	VDEDestPubkey  string `gorm:"not null" json:"vde_dest_pubkey"`
	VDEDestIP      string `gorm:"not null" json:"vde_dest_ip"`
	VDEAgentPubkey string `gorm:"not null" json:"vde_agent_pubkey"`
	VDEAgentIP     string `gorm:"not null" json:"vde_agent_ip"`
	AgentMode      int64  `gorm:"not null" json:"agent_mode"`
	DataSendState  int64  `gorm:"not null" json:"data_send_state"`
	DataSendErr    string `gorm:"not null" json:"data_send_err"`
	UpdateTime     int64  `gorm:"not null" json:"update_time"`
	CreateTime     int64  `gorm:"not null" json:"create_time"`
}

func (VDESrcDataStatus) TableName() string {
	return "vde_src_data_status"
}

func (m *VDESrcDataStatus) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcDataStatus) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcDataStatus) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcDataStatus) GetAll() ([]VDESrcDataStatus, error) {
	var result []VDESrcDataStatus
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcDataStatus) GetOneByID() (*VDESrcDataStatus, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDESrcDataStatus) GetAllByTaskUUID(TaskUUID string) ([]VDESrcDataStatus, error) {
	result := make([]VDESrcDataStatus, 0)
	err := DBConn.Table("vde_src_data_status").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
	return result, err
}

func (m *VDESrcDataStatus) GetOneByDataSendStatus(DataSendStatus int64) (bool, error) {
	return isFound(DBConn.Where("data_send_state = ?", DataSendStatus).First(m))
}
