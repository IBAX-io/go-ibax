/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEDestDataStatus struct {
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
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDEDestDataStatus) TableName() string {
	return "vde_dest_data_status"
}

func (m *VDEDestDataStatus) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEDestDataStatus) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEDestDataStatus) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEDestDataStatus) GetAll() ([]VDEDestDataStatus, error) {
	var result []VDEDestDataStatus
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEDestDataStatus) GetOneByID() (*VDEDestDataStatus, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
func (m *VDEDestDataStatus) GetOneByDataUUID(DataUUID string) (*VDEDestDataStatus, error) {
	err := DBConn.Where("data_uuid=?", DataUUID).First(&m).Error
	return m, err
}
func (m *VDEDestDataStatus) GetOneByTaskUUID(TaskUUID string) (*VDEDestDataStatus, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}
func (m *VDEDestDataStatus) GetAllByTaskUUID(TaskUUID string) ([]VDEDestDataStatus, error) {
	result := make([]VDEDestDataStatus, 0)
	err := DBConn.Table("vde_dest_data_status").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDEDestDataStatus) GetAllByDataStatus(AuthState int64, SignState int64, HashState int64) ([]VDEDestDataStatus, error) {
	result := make([]VDEDestDataStatus, 0)
	err := DBConn.Table("vde_dest_data_status").Where("auth_state = ? AND sign_state = ? AND hash_state = ?", AuthState, SignState, HashState).Find(&result).Error
	return result, err
}

func (m *VDEDestDataStatus) GetAllByHashState(HashState int64) ([]VDEDestDataStatus, error) {
	result := make([]VDEDestDataStatus, 0)
	err := DBConn.Table("vde_dest_data_status").Where("hash_state = ?", HashState).Find(&result).Error
	return result, err
}

func (m *VDEDestDataStatus) GetAllBySignState(SignState int64) ([]VDEDestDataStatus, error) {
	result := make([]VDEDestDataStatus, 0)
	err := DBConn.Table("vde_dest_data_status").Where("sign_state = ?", SignState).Find(&result).Error
	return result, err
}

func (m *VDEDestDataStatus) GetAllByTaskUUIDAndDataStatus(TaskUUID string, AuthState int64, SignState int64, HashState int64) ([]VDEDestDataStatus, error) {
	result := make([]VDEDestDataStatus, 0)
	err := DBConn.Table("vde_dest_data_status").Where("task_uuid = ? AND auth_state = ? AND sign_state = ? AND hash_state = ?", TaskUUID, AuthState, SignState, HashState).Find(&result).Error
	return result, err
}

func (m *VDEDestDataStatus) GetAllByTaskUUIDAndDataStatusAndTime(TaskUUID string, AuthState int64, SignState int64, HashState int64, BTime int64, ETime int64) ([]VDEDestDataStatus, error) {
	result := make([]VDEDestDataStatus, 0)
	err := DBConn.Table("vde_dest_data_status").Where("task_uuid = ? AND auth_state = ? AND sign_state = ? AND hash_state = ? AND create_time > ? AND create_time <= ?", TaskUUID, AuthState, SignState, HashState, BTime, ETime).Find(&result).Error
	return result, err
}
