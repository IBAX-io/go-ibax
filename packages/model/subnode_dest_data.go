/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

type SubNodeDestData struct {
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
	DataState      int64  `gorm:"not null" json:"data_state"`
	UpdateTime     int64  `gorm:"not null" json:"update_time"`
	CreateTime     int64  `gorm:"not null" json:"create_time"`
}

func (SubNodeDestData) TableName() string {
	return "subnode_dest_data"
}

func (m *SubNodeDestData) Create() error {
	return DBConn.Create(&m).Error
}
	err := DBConn.Where("data_uuid=?", DataUUID).First(&m).Error
	return m, err
}
func (m *SubNodeDestData) GetOneByTaskUUID(TaskUUID string) (*SubNodeDestData, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}
func (m *SubNodeDestData) GetAllByTaskUUID(TaskUUID string) ([]SubNodeDestData, error) {
	result := make([]SubNodeDestData, 0)
	err := DBConn.Table("subnode_dest_data").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *SubNodeDestData) GetAllByDataStatus(DataStatus int64) ([]SubNodeDestData, error) {
	result := make([]SubNodeDestData, 0)
	err := DBConn.Table("subnode_dest_data").Where("data_state = ?", DataStatus).Find(&result).Error
	return result, err
}

func (m *SubNodeDestData) GetOneByDataStatus(DataStatus int64) (bool, error) {
	return isFound(DBConn.Where("data_state = ?", DataStatus).First(m))
}

func (m *SubNodeDestData) Get(Hash string) (SubNodeDestData, error) {
	var sndd SubNodeDestData
	err := DBConn.Where("hash=?", Hash).First(&sndd).Error
	return sndd, err
}
