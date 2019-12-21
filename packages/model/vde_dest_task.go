/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEDestTaskFromSrc struct {
	ID         int64  `gorm:"primary_key; not null" json:"id"`
	TaskUUID   string `gorm:"not null" json:"task_uuid"`
	TaskName   string `gorm:"not null" json:"task_name"`
	TaskSender string `gorm:"not null" json:"task_sender"`
	Comment    string `gorm:"not null" json:"comment"`
	Parms      string `gorm:"type:jsonb" json:"parms"`
	TaskType   int64  `gorm:"not null" json:"task_type"`
	TaskState  int64  `gorm:"not null" json:"task_state"`

	ContractSrcName     string `gorm:"not null" json:"contract_src_name"`
	ContractSrcGet      string `gorm:"not null" json:"contract_src_get"`
	ContractSrcGetHash  string `gorm:"not null" json:"contract_src_get_hash"`
	ContractDestName    string `gorm:"not null" json:"contract_dest_name"`
	ContractDestGet     string `gorm:"not null" json:"contract_dest_get"`
	ContractDestGetHash string `gorm:"not null" json:"contract_dest_get_hash"`

	ContractRunHttp      string `gorm:"not null" json:"contract_run_http"`
	ContractRunEcosystem string `gorm:"not null" json:"contract_run_ecosystem"`
	ContractRunParms     string `gorm:"type:jsonb" json:"contract_run_parms"`

	ContractMode int64 `gorm:"not null" json:"contract_mode"`

	ContractStateSrc     int64  `gorm:"not null" json:"contract_state_src"`
	ContractStateDest    int64  `gorm:"not null" json:"contract_state_dest"`
	ContractStateSrcErr  string `gorm:"not null" json:"contract_state_src_err"`
	ContractStateDestErr string `gorm:"not null" json:"contract_state_dest_err"`

	TaskRunState    int64  `gorm:"not null" json:"task_run_state"`
	TaskRunStateErr string `gorm:"not null" json:"task_run_state_err"`

	//TxHash                 string `gorm:"not null" json:"tx_hash"`
	//ChainState             int64  `gorm:"not null" json:"chain_state"`
	//BlockId                int64  `gorm:"not null" json:"block_id"`
	//ChainId                int64  `gorm:"not null" json:"chain_id"`
	//ChainErr               string `gorm:"not null" json:"chain_err"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDEDestTaskFromSrc) TableName() string {
	return "vde_dest_task_from_src"
}

func (m *VDEDestTaskFromSrc) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEDestTaskFromSrc) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEDestTaskFromSrc) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEDestTaskFromSrc) GetAll() ([]VDEDestTaskFromSrc, error) {
	var result []VDEDestTaskFromSrc
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEDestTaskFromSrc) GetOneByID() (*VDEDestTaskFromSrc, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDEDestTaskFromSrc) GetAllByTaskUUID(TaskUUID string) ([]VDEDestTaskFromSrc, error) {
	result := make([]VDEDestTaskFromSrc, 0)
	err := DBConn.Table("vde_dest_task_from_src").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDEDestTaskFromSrc) GetAllByTaskUUIDAndTaskState(TaskUUID string, TaskState int64) ([]VDEDestTaskFromSrc, error) {
	result := make([]VDEDestTaskFromSrc, 0)
	err := DBConn.Table("vde_dest_task_from_src").Where("task_uuid = ? AND task_state=?", TaskUUID, TaskState).Find(&result).Error
	return result, err
}

func (m *VDEDestTaskFromSrc) GetOneByTaskUUID(TaskUUID string) (*VDEDestTaskFromSrc, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}
func (m *VDEDestTaskFromSrc) GetOneByTaskUUIDAndTaskState(TaskUUID string, TaskState int64) (*VDEDestTaskFromSrc, error) {
	err := DBConn.Where("task_uuid=? AND task_state=?", TaskUUID, TaskState).First(&m).Error
	return m, err
}

func (m *VDEDestTaskFromSrc) GetAllByTaskState(TaskState int64) ([]VDEDestTaskFromSrc, error) {
	result := make([]VDEDestTaskFromSrc, 0)
	err := DBConn.Table("vde_dest_task_from_src").Where("task_state = ?", TaskState).Find(&result).Error
	return result, err
}

func (m *VDEDestTaskFromSrc) GetOneByTaskState(TaskState int64) (bool, error) {
	return isFound(DBConn.Where("task_state = ?", TaskState).First(m))
}

func (m *VDEDestTaskFromSrc) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}

func (m *VDEDestTaskFromSrc) GetAllByContractStateAndChainState(ContractStateSrc int64, ContractStateDest int64, ChainState int64) ([]VDEDestTaskFromSrc, error) {
	result := make([]VDEDestTaskFromSrc, 0)
	err := DBConn.Table("vde_dest_task_from_src").Where("contract_state_src = ? AND contract_state_dest = ? AND chain_state = ?", ContractStateSrc, ContractStateDest, ChainState).Find(&result).Error
	return result, err
}

func (m *VDEDestTaskFromSrc) GetAllByContractStateSrc(ContractStateSrc int64) ([]VDEDestTaskFromSrc, error) {
	result := make([]VDEDestTaskFromSrc, 0)
	err := DBConn.Table("vde_dest_task_from_src").Where("contract_state_src = ?", ContractStateSrc).Find(&result).Error
	return result, err
}

func (m *VDEDestTaskFromSrc) GetAllByContractStateDest(ContractStateDest int64) ([]VDEDestTaskFromSrc, error) {
	result := make([]VDEDestTaskFromSrc, 0)
	err := DBConn.Table("vde_dest_task_from_src").Where("contract_state_dest = ?", ContractStateDest).Find(&result).Error
	return result, err
}

type VDEDestTaskFromSche struct {
	ID         int64  `gorm:"primary_key; not null" json:"id"`
	TaskUUID   string `gorm:"not null" json:"task_uuid"`
	TaskName   string `gorm:"not null" json:"task_name"`
	TaskSender string `gorm:"not null" json:"task_sender"`
	Comment    string `gorm:"not null" json:"comment"`
	Parms      string `gorm:"type:jsonb" json:"parms"`
	TaskType   int64  `gorm:"not null" json:"task_type"`
	TaskState  int64  `gorm:"not null" json:"task_state"`

	ContractSrcName     string `gorm:"not null" json:"contract_src_name"`
	ContractSrcGet      string `gorm:"not null" json:"contract_src_get"`
	ContractSrcGetHash  string `gorm:"not null" json:"contract_src_get_hash"`
	ContractDestName    string `gorm:"not null" json:"contract_dest_name"`
	ContractDestGet     string `gorm:"not null" json:"contract_dest_get"`
	ContractDestGetHash string `gorm:"not null" json:"contract_dest_get_hash"`
	ContractStateDest    int64  `gorm:"not null" json:"contract_state_dest"`
	ContractStateSrcErr  string `gorm:"not null" json:"contract_state_src_err"`
	ContractStateDestErr string `gorm:"not null" json:"contract_state_dest_err"`

	TaskRunState    int64  `gorm:"not null" json:"task_run_state"`
	TaskRunStateErr string `gorm:"not null" json:"task_run_state_err"`

	//TxHash                 string `gorm:"not null" json:"tx_hash"`
	//ChainState             int64  `gorm:"not null" json:"chain_state"`
	//BlockId                int64  `gorm:"not null" json:"block_id"`
	//ChainId                int64  `gorm:"not null" json:"chain_id"`
	//ChainErr               string `gorm:"not null" json:"chain_err"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDEDestTaskFromSche) TableName() string {
	return "vde_dest_task_from_sche"
}

func (m *VDEDestTaskFromSche) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEDestTaskFromSche) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEDestTaskFromSche) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEDestTaskFromSche) GetAll() ([]VDEDestTaskFromSche, error) {
	var result []VDEDestTaskFromSche
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEDestTaskFromSche) GetOneByID() (*VDEDestTaskFromSche, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDEDestTaskFromSche) GetOneByTaskUUID(TaskUUID string) (*VDEDestTaskFromSche, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}

func (m *VDEDestTaskFromSche) GetOneByTaskUUIDAndTaskState(TaskUUID string, TaskState int64) (*VDEDestTaskFromSche, error) {
	err := DBConn.Where("task_uuid=? AND task_state=?", TaskUUID, TaskState).First(&m).Error
	return m, err
}

func (m *VDEDestTaskFromSche) GetAllByTaskUUID(TaskUUID string) ([]VDEDestTaskFromSche, error) {
	result := make([]VDEDestTaskFromSche, 0)
	err := DBConn.Table("vde_dest_task_from_sche").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDEDestTaskFromSche) GetAllByTaskUUIDAndTaskState(TaskUUID string, TaskState int64) ([]VDEDestTaskFromSche, error) {
	result := make([]VDEDestTaskFromSche, 0)
	err := DBConn.Table("vde_dest_task_from_sche").Where("task_uuid = ? AND task_state=?", TaskUUID, TaskState).Find(&result).Error
	return result, err
}

func (m *VDEDestTaskFromSche) GetAllByTaskState(TaskState int64) ([]VDEDestTaskFromSche, error) {
	result := make([]VDEDestTaskFromSche, 0)
	err := DBConn.Table("vde_dest_task_from_sche").Where("task_state = ?", TaskState).Find(&result).Error
	return result, err
}

func (m *VDEDestTaskFromSche) GetOneByTaskState(TaskState int64) (bool, error) {
	return isFound(DBConn.Where("task_state = ?", TaskState).First(m))
}

func (m *VDEDestTaskFromSche) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}

func (m *VDEDestTaskFromSche) GetAllByContractStateAndChainState(ContractStateSrc int64, ContractStateDest int64, ChainState int64) ([]VDEDestTaskFromSche, error) {
	result := make([]VDEDestTaskFromSche, 0)
	err := DBConn.Table("vde_dest_task_from_sche").Where("contract_state_src = ? AND contract_state_dest = ? AND chain_state = ?", ContractStateSrc, ContractStateDest, ChainState).Find(&result).Error
	return result, err
}

func (m *VDEDestTaskFromSche) GetAllByContractStateSrc(ContractStateSrc int64) ([]VDEDestTaskFromSche, error) {
	result := make([]VDEDestTaskFromSche, 0)
	err := DBConn.Table("vde_dest_task_from_sche").Where("contract_state_src = ?", ContractStateSrc).Find(&result).Error
	return result, err
}

func (m *VDEDestTaskFromSche) GetAllByContractStateDest(ContractStateDest int64) ([]VDEDestTaskFromSche, error) {
	result := make([]VDEDestTaskFromSche, 0)
	err := DBConn.Table("vde_dest_task_from_sche").Where("contract_state_Dest = ?", ContractStateDest).Find(&result).Error
	return result, err
}
