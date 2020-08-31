/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEScheTaskChainStatus struct {
	ID           int64  `gorm:"primary_key; not null" json:"id"`
	TaskUUID     string `gorm:"not null" json:"task_uuid"`
	TaskName     string `gorm:"not null" json:"task_name"`
	TaskSender   string `gorm:"not null" json:"task_sender"`
	TaskReceiver string `gorm:"not null" json:"task_receiver"`
	Comment      string `gorm:"not null" json:"comment"`
	Parms        string `gorm:"type:jsonb" json:"parms"`
	TaskType     int64  `gorm:"not null" json:"task_type"`
	TaskState    int64  `gorm:"not null" json:"task_state"`

	ContractSrcName     string `gorm:"not null" json:"contract_src_name"`
	ContractSrcGet      string `gorm:"not null" json:"contract_src_get"`
	ContractSrcGetHash  string `gorm:"not null" json:"contract_src_get_hash"`
	ContractDestName    string `gorm:"not null" json:"contract_dest_name"`
	ContractDestGet     string `gorm:"not null" json:"contract_dest_get"`
	ContractDestGetHash string `gorm:"not null" json:"contract_dest_get_hash"`
	ContractMode        int64  `gorm:"not null" json:"contract_mode"`

	ContractStateSrc     int64  `gorm:"not null" json:"contract_state_src"`
	ContractStateDest    int64  `gorm:"not null" json:"contract_state_dest"`
	ContractStateSrcErr  string `gorm:"not null" json:"contract_state_src_err"`
	ContractStateDestErr string `gorm:"not null" json:"contract_state_dest_err"`

	ContractRunHttp      string `gorm:"not null" json:"contract_run_http"`
	ContractRunEcosystem string `gorm:"not null" json:"contract_run_ecosystem"`
	ContractRunParms     string `gorm:"type:jsonb" json:"contract_run_parms"`

	TaskRunState    int64  `gorm:"not null" json:"task_run_state"`
	TaskRunStateErr string `gorm:"not null" json:"task_run_state_err"`

	TxHash     string `gorm:"not null" json:"tx_hash"`
	ChainState int64  `gorm:"not null" json:"chain_state"`
	BlockId    int64  `gorm:"not null" json:"block_id"`
	ChainId    int64  `gorm:"not null" json:"chain_id"`
	ChainErr   string `gorm:"not null" json:"chain_err"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDEScheTaskChainStatus) TableName() string {
	return "vde_sche_task_chain_status"
}

func (m *VDEScheTaskChainStatus) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEScheTaskChainStatus) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEScheTaskChainStatus) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEScheTaskChainStatus) GetAll() ([]VDEScheTaskChainStatus, error) {
	var result []VDEScheTaskChainStatus
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEScheTaskChainStatus) GetOneByID() (*VDEScheTaskChainStatus, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDEScheTaskChainStatus) GetAllByTaskUUID(TaskUUID string) ([]VDEScheTaskChainStatus, error) {
	result := make([]VDEScheTaskChainStatus, 0)
	err := DBConn.Table("vde_sche_task_chain_status").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

// func (m *VDEScheTaskChainStatus) GetOneByTaskUUID(TaskUUID string) (*VDEScheTaskChainStatus, bool) {
// 	var (
// 		result VDEScheTaskChainStatus
// 		b	bool
// 	)
// 	b = DBConn.Where("task_uuid = ?", TaskUUID).First(m).RecordNotFound()
// 	return &result, b
// }
}

func (m *VDEScheTaskChainStatus) GetOneByTaskState(TaskState int64) (bool, error) {
	return isFound(DBConn.Where("task_state = ?", TaskState).First(m))
}

func (m *VDEScheTaskChainStatus) GetOneByContractState(ContractState int64) (bool, error) {
	return isFound(DBConn.Where("contract_state = ?", ContractState).First(m))
}

func (m *VDEScheTaskChainStatus) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}

func (m *VDEScheTaskChainStatus) GetAllByContractStateAndChainState(ContractStateSrc int64, ContractStateDest int64, ChainState int64) ([]VDEScheTaskChainStatus, error) {
	result := make([]VDEScheTaskChainStatus, 0)
	err := DBConn.Table("vde_sche_task_chain_status").Where("contract_state_src = ? AND contract_state_dest = ? AND chain_state = ?", ContractStateSrc, ContractStateDest, ChainState).Find(&result).Error
	return result, err
}

func (m *VDEScheTaskChainStatus) GetAllByContractStateSrc(ContractStateSrc int64) ([]VDEScheTaskChainStatus, error) {
	result := make([]VDEScheTaskChainStatus, 0)
	err := DBConn.Table("vde_sche_task_chain_status").Where("contract_state_src = ?", ContractStateSrc).Find(&result).Error
	return result, err
}

func (m *VDEScheTaskChainStatus) GetAllByContractStateDest(ContractStateDest int64) ([]VDEScheTaskChainStatus, error) {
	result := make([]VDEScheTaskChainStatus, 0)
	err := DBConn.Table("vde_sche_task_chain_status").Where("contract_state_dest = ? AND contract_state_src = 1", ContractStateDest).Find(&result).Error
	return result, err
}
