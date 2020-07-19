/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

type SubNodeDestDataStatus struct {
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

	AuthState int64 `gorm:"not null" json:"auth_state"`
	SignState int64 `gorm:"not null" json:"sign_state"`
	HashState int64 `gorm:"not null" json:"hash_state"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (SubNodeDestDataStatus) TableName() string {
	return "subnode_dest_data_status"
}

func (m *SubNodeDestDataStatus) Create() error {
	return DBConn.Create(&m).Error
}

func (m *SubNodeDestDataStatus) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *SubNodeDestDataStatus) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *SubNodeDestDataStatus) GetAll() ([]SubNodeDestDataStatus, error) {
	var result []SubNodeDestDataStatus
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *SubNodeDestDataStatus) GetOneByID() (*SubNodeDestDataStatus, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
func (m *SubNodeDestDataStatus) GetOneByDataUUID(DataUUID string) (*SubNodeDestDataStatus, error) {
	err := DBConn.Where("data_uuid=?", DataUUID).First(&m).Error
	return m, err
}
func (m *SubNodeDestDataStatus) GetOneByTaskUUID(TaskUUID string) (*SubNodeDestDataStatus, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}
func (m *SubNodeDestDataStatus) GetAllByTaskUUID(TaskUUID string) ([]SubNodeDestDataStatus, error) {
	result := make([]SubNodeDestDataStatus, 0)
	err := DBConn.Table("subnode_dest_data_status").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *SubNodeDestDataStatus) GetAllByDataStatus(AuthState int64, SignState int64, HashState int64) ([]SubNodeDestDataStatus, error) {
	result := make([]SubNodeDestDataStatus, 0)
	err := DBConn.Table("subnode_dest_data_status").Where("auth_state = ? AND sign_state = ? AND hash_state = ?", AuthState, SignState, HashState).Find(&result).Error
	return result, err
}

func (m *SubNodeDestDataStatus) GetAllByTaskUUIDAndDataStatusAndTime(TaskUUID string, AuthState int64, SignState int64, HashState int64, BTime int64, ETime int64) ([]SubNodeDestDataStatus, error) {
	result := make([]SubNodeDestDataStatus, 0)
	err := DBConn.Table("subnode_dest_data_status").Where("task_uuid = ? AND auth_state = ? AND sign_state = ? AND hash_state = ? AND create_time > ? AND create_time <= ?", TaskUUID, AuthState, SignState, HashState, BTime, ETime).Find(&result).Error
	return result, err
}

func (m *SubNodeDestDataStatus) Get(Hash string) (SubNodeDestDataStatus, error) {
	var sndd SubNodeDestDataStatus
	err := DBConn.Where("hash=?", Hash).First(&sndd).Error
	return sndd, err
}
