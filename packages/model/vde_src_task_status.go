/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDESrcTaskStatus struct {
	ID                   int64  `gorm:"primary_key; not null" json:"id"`
	TaskUUID             string `gorm:"not null" json:"task_uuid"`
	ContractRunHttp      string `gorm:"not null" json:"contract_run_http"`
	ContractRunEcosystem string `gorm:"not null" json:"contract_run_ecosystem"`
	ContractRunParms     string `gorm:"type:jsonb" json:"contract_run_parms"`
	ContractSrcName      string `gorm:"not null" json:"contract_src_name"`
	TxHash               string `gorm:"not null" json:"tx_hash"`
	ChainState           int64  `gorm:"not null" json:"chain_state"`
	BlockId              int64  `gorm:"not null" json:"block_id"`
	ChainId              int64  `gorm:"not null" json:"chain_id"`
	ChainErr             string `gorm:"not null" json:"chain_err"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDESrcTaskStatus) TableName() string {
	return "vde_src_task_status"
}

func (m *VDESrcTaskStatus) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcTaskStatus) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcTaskStatus) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcTaskStatus) GetAll() ([]VDESrcTaskStatus, error) {
	var result []VDESrcTaskStatus
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcTaskStatus) GetOneByID() (*VDESrcTaskStatus, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDESrcTaskStatus) GetAllByTaskUUID(TaskUUID string) ([]VDESrcTaskStatus, error) {
	result := make([]VDESrcTaskStatus, 0)
	err := DBConn.Table("vde_src_task_status").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDESrcTaskStatus) GetOneByTaskUUID(TaskUUID string) (*VDESrcTaskStatus, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}

func (m *VDESrcTaskStatus) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}
	ContractRunEcosystem string `gorm:"not null" json:"contract_run_ecosystem"`
	ContractRunParms     string `gorm:"type:jsonb" json:"contract_run_parms"`
	ContractSrcName      string `gorm:"not null" json:"contract_src_name"`
	TxHash               string `gorm:"not null" json:"tx_hash"`
	ChainState           int64  `gorm:"not null" json:"chain_state"`
	BlockId              int64  `gorm:"not null" json:"block_id"`
	ChainId              int64  `gorm:"not null" json:"chain_id"`
	ChainErr             string `gorm:"not null" json:"chain_err"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDESrcTaskFromScheStatus) TableName() string {
	return "vde_src_task_from_sche_status"
}

func (m *VDESrcTaskFromScheStatus) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcTaskFromScheStatus) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcTaskFromScheStatus) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcTaskFromScheStatus) GetAll() ([]VDESrcTaskFromScheStatus, error) {
	var result []VDESrcTaskFromScheStatus
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcTaskFromScheStatus) GetOneByID() (*VDESrcTaskFromScheStatus, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDESrcTaskFromScheStatus) GetAllByTaskUUID(TaskUUID string) ([]VDESrcTaskFromScheStatus, error) {
	result := make([]VDESrcTaskFromScheStatus, 0)
	err := DBConn.Table("vde_src_task_from_sche_status").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDESrcTaskFromScheStatus) GetOneByTaskUUID(TaskUUID string) (*VDESrcTaskFromScheStatus, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}

func (m *VDESrcTaskFromScheStatus) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}

func (m *VDESrcTaskFromScheStatus) GetAllByChainState(ChainState int64) ([]VDESrcTaskFromScheStatus, error) {
	result := make([]VDESrcTaskFromScheStatus, 0)
	err := DBConn.Table("vde_src_task_from_sche_status").Where("chain_state = ?", ChainState).Find(&result).Error
	return result, err
}
