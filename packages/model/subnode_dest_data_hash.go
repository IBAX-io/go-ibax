/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

type SubNodeDestDataHash struct {
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

func (SubNodeDestDataHash) TableName() string {
	return "subnode_dest_data_hash"
}

func (m *SubNodeDestDataHash) Create() error {
	return DBConn.Create(&m).Error
}

func (m *SubNodeDestDataHash) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *SubNodeDestDataHash) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *SubNodeDestDataHash) GetAll() ([]SubNodeDestDataHash, error) {
	var result []SubNodeDestDataHash
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *SubNodeDestDataHash) GetOneByID() (*SubNodeDestDataHash, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
func (m *SubNodeDestDataHash) GetOneByDataUUID(DataUUID string) (*SubNodeDestDataHash, error) {
	err := DBConn.Where("data_uuid=?", DataUUID).First(&m).Error
	return m, err
}
func (m *SubNodeDestDataHash) GetOneByTaskUUID(TaskUUID string) (*SubNodeDestDataHash, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}
	return result, err
}

func (m *SubNodeDestDataHash) GetAllBySignState(SignState int64) ([]SubNodeDestDataHash, error) {
	result := make([]SubNodeDestDataHash, 0)
	err := DBConn.Table("subnode_dest_data_hash").Where("sign_state = ?", SignState).Find(&result).Error
	return result, err
}

func (m *SubNodeDestDataHash) GetAllByTaskUUIDAndDataStatus(TaskUUID string, AuthState int64, SignState int64, HashState int64) ([]SubNodeDestDataHash, error) {
	result := make([]SubNodeDestDataHash, 0)
	err := DBConn.Table("subnode_dest_data_hash").Where("task_uuid = ? AND auth_state = ? AND sign_state = ? AND hash_state = ?", TaskUUID, AuthState, SignState, HashState).Find(&result).Error
	return result, err
}

func (m *SubNodeDestDataHash) GetAllByTaskUUIDAndDataStatusAndTime(TaskUUID string, AuthState int64, SignState int64, HashState int64, BTime int64, ETime int64) ([]SubNodeDestDataHash, error) {
	result := make([]SubNodeDestDataHash, 0)
	err := DBConn.Table("subnode_dest_data_hash").Where("task_uuid = ? AND auth_state = ? AND sign_state = ? AND hash_state = ? AND create_time > ? AND create_time <= ?", TaskUUID, AuthState, SignState, HashState, BTime, ETime).Find(&result).Error
	return result, err
}

func (m *SubNodeDestDataHash) Get(Hash string) (SubNodeDestDataHash, error) {
	var sndd SubNodeDestDataHash
	err := DBConn.Where("hash=?", Hash).First(&sndd).Error
	return sndd, err
}
