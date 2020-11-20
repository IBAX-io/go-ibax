/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

type SubNodeAgentData struct {
	ID       int64  `gorm:"primary_key; not null" json:"id"`
	DataUUID string `gorm:"not null" json:"data_uuid"`
	TaskUUID string `gorm:"not null" json:"task_uuid"`
	Hash     string `gorm:"not null" json:"hash"`
	Data     []byte `gorm:"not null" json:"data"`
	DataInfo string `gorm:"type:jsonb" json:"data_info"`
	//SubNodeSrcPubkey     string `gorm:"not null" json:"subnode_src_pubkey"`
	SubNodeSrcPubkey string `gorm:"column:subnode_src_pubkey;not null" json:"subnode_src_pubkey"`
	//SubNodeDestPubkey    string `gorm:"not null" json:"subnode_dest_pubkey"`
	SubNodeDestPubkey string `gorm:"column:subnode_dest_pubkey;not null" json:"subnode_dest_pubkey"`
	//SubNodeDestIP        string `gorm:"not null" json:"subnode_dest_ip"`
	SubNodeDestIP string `gorm:"column:subnode_dest_ip;not null" json:"subnode_dest_ip"`
	//SubNodeAgentPubkey   string `gorm:"not null" json:"subnode_agent_pubkey"`
	SubNodeAgentPubkey string `gorm:"column:subnode_agent_pubkey;not null" json:"subnode_agent_pubkey"`
	//SubNodeAgentIP       string `gorm:"not null" json:"subnode_agent_ip"`
	SubNodeAgentIP string `gorm:"column:subnode_agent_ip;not null" json:"subnode_agent_ip"`
	AgentMode      int64  `gorm:"not null" json:"agent_mode"`
	TranMode       int64  `gorm:"not null" json:"tran_mode"`
	DataSendState  int64  `gorm:"not null" json:"data_send_state"`
	DataSendErr    string `gorm:"not null" json:"data_send_err"`
	UpdateTime     int64  `gorm:"not null" json:"update_time"`
	CreateTime     int64  `gorm:"not null" json:"create_time"`
}

func (SubNodeAgentData) TableName() string {
	return "subnode_agent_data"
}

func (m *SubNodeAgentData) Create() error {
	return DBConn.Create(&m).Error
}

func (m *SubNodeAgentData) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *SubNodeAgentData) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *SubNodeAgentData) GetAll() ([]SubNodeAgentData, error) {
	var result []SubNodeAgentData
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *SubNodeAgentData) GetOneByID() (*SubNodeAgentData, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
func (m *SubNodeAgentData) GetOneByDataUUID(DataUUID string) (*SubNodeAgentData, error) {
	err := DBConn.Where("data_uuid=?", DataUUID).First(&m).Error
	return m, err
}
func (m *SubNodeAgentData) GetOneByTaskUUID(TaskUUID string) (*SubNodeAgentData, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}
func (m *SubNodeAgentData) GetAllByTaskUUID(TaskUUID string) ([]SubNodeAgentData, error) {
	result := make([]SubNodeAgentData, 0)
	err := DBConn.Table("subnode_agent_data").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return isFound(DBConn.Where("data_state = ?", DataStatus).First(m))
}
