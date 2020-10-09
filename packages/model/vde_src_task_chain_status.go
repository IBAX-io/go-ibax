/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

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

func (VDESrcTaskChainStatus) TableName() string {
	return "vde_src_task_chain_status"
}

func (m *VDESrcTaskChainStatus) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcTaskChainStatus) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcTaskChainStatus) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcTaskChainStatus) GetAll() ([]VDESrcTaskChainStatus, error) {
	var result []VDESrcTaskChainStatus
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcTaskChainStatus) GetOneByID() (*VDESrcTaskChainStatus, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDESrcTaskChainStatus) GetAllByTaskUUID(TaskUUID string) ([]VDESrcTaskChainStatus, error) {
	result := make([]VDESrcTaskChainStatus, 0)
	err := DBConn.Table("vde_src_task_chain_status").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

// func (m *VDESrcTaskChainStatus) GetOneByTaskUUID(TaskUUID string) (*VDESrcTaskChainStatus, bool) {
// 	var (
// 		result VDESrcTaskChainStatus
// 		b	bool
// 	)
// 	b = DBConn.Where("task_uuid = ?", TaskUUID).First(m).RecordNotFound()
// 	return &result, b
// }

func (m *VDESrcTaskChainStatus) GetOneByTaskUUID(TaskUUID string, TaskState int64) (*VDESrcTaskChainStatus, error) {
	err := DBConn.Where("task_uuid=? AND task_state=?", TaskUUID, TaskState).First(&m).Error
	return m, err
}
func (m *VDESrcTaskChainStatus) GetOneByTaskUUIDAndReceiverAndChainState(TaskUUID string, TaskReceiver string, ChainState int64) (*VDESrcTaskChainStatus, error) {
	err := DBConn.Where("task_uuid=? AND task_receiver=? AND chain_state = ?", TaskUUID, TaskReceiver, ChainState).First(&m).Error
	return m, err
}

func (m *VDESrcTaskChainStatus) GetAllByTaskState(TaskState int64) ([]VDESrcTaskChainStatus, error) {
	result := make([]VDESrcTaskChainStatus, 0)
	err := DBConn.Table("vde_src_task_chain_status").Where("task_state = ?", TaskState).Find(&result).Error
	return result, err
}

func (m *VDESrcTaskChainStatus) GetOneByTaskState(TaskState int64) (bool, error) {
	return isFound(DBConn.Where("task_state = ?", TaskState).First(m))
}

func (m *VDESrcTaskChainStatus) GetOneByContractState(ContractState int64) (bool, error) {
	return isFound(DBConn.Where("contract_state = ?", ContractState).First(m))
}

func (m *VDESrcTaskChainStatus) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}

func (m *VDESrcTaskChainStatus) GetAllByContractStateAndChainState(ContractStateSrc int64, ContractStateDest int64, ChainState int64) ([]VDESrcTaskChainStatus, error) {
	result := make([]VDESrcTaskChainStatus, 0)
	err := DBConn.Table("vde_src_task_chain_status").Where("contract_state_src = ? AND contract_state_dest = ? AND chain_state = ?", ContractStateSrc, ContractStateDest, ChainState).Find(&result).Error
	return result, err
}

func (m *VDESrcTaskChainStatus) GetAllByContractStateSrc(ContractStateSrc int64) ([]VDESrcTaskChainStatus, error) {
	result := make([]VDESrcTaskChainStatus, 0)
	err := DBConn.Table("vde_src_task_chain_status").Where("contract_state_src = ?", ContractStateSrc).Find(&result).Error
	return result, err
}

func (m *VDESrcTaskChainStatus) GetAllByContractStateDest(ContractStateDest int64) ([]VDESrcTaskChainStatus, error) {
	result := make([]VDESrcTaskChainStatus, 0)
	err := DBConn.Table("vde_src_task_chain_status").Where("contract_state_dest = ? AND contract_state_src = 1", ContractStateDest).Find(&result).Error
	return result, err
}
