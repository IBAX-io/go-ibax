/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEScheTask struct {
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

func (VDEScheTask) TableName() string {
	return "vde_sche_task"
}

func (m *VDEScheTask) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEScheTask) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEScheTask) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEScheTask) GetAll() ([]VDEScheTask, error) {
	var result []VDEScheTask
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEScheTask) GetOneByID() (*VDEScheTask, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDEScheTask) GetAllByTaskUUID(TaskUUID string) ([]VDEScheTask, error) {
	result := make([]VDEScheTask, 0)
	err := DBConn.Table("vde_sche_task").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

// func (m *VDEScheTask) GetOneByTaskUUID(TaskUUID string) (*VDEScheTask, bool) {
// 	var (
// 		result VDEScheTask
// 		b	bool
// 	)
// 	b = DBConn.Where("task_uuid = ?", TaskUUID).First(m).RecordNotFound()
// 	return &result, b
// }

func (m *VDEScheTask) GetOneByTaskUUID(TaskUUID string, TaskState int64) (*VDEScheTask, error) {
	err := DBConn.Where("task_uuid=? AND task_state=?", TaskUUID, TaskState).First(&m).Error
	return m, err
}

func (m *VDEScheTask) GetAllByTaskState(TaskState int64) ([]VDEScheTask, error) {
	result := make([]VDEScheTask, 0)
	err := DBConn.Table("vde_sche_task").Where("task_state = ?", TaskState).Find(&result).Error
	return result, err
}

func (m *VDEScheTask) GetOneByTaskState(TaskState int64) (bool, error) {
	return isFound(DBConn.Where("task_state = ?", TaskState).First(m))
}

func (m *VDEScheTask) GetOneByContractState(ContractState int64) (bool, error) {
	return isFound(DBConn.Where("contract_state = ?", ContractState).First(m))
}

func (m *VDEScheTask) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}

func (m *VDEScheTask) GetAllByContractStateAndChainState(ContractStateSrc int64, ContractStateDest int64, ChainState int64) ([]VDEScheTask, error) {
	result := make([]VDEScheTask, 0)
	err := DBConn.Table("vde_sche_task").Where("contract_state_src = ? AND contract_state_dest = ? AND chain_state = ?", ContractStateSrc, ContractStateDest, ChainState).Find(&result).Error
	return result, err
}

func (m *VDEScheTask) GetAllByContractStateSrc(ContractStateSrc int64) ([]VDEScheTask, error) {
	result := make([]VDEScheTask, 0)
	err := DBConn.Table("vde_sche_task").Where("contract_state_src = ?", ContractStateSrc).Find(&result).Error
	return result, err
}

func (m *VDEScheTask) GetAllByContractStateDest(ContractStateDest int64) ([]VDEScheTask, error) {
	result := make([]VDEScheTask, 0)
	err := DBConn.Table("vde_sche_task").Where("contract_state_dest = ? AND contract_state_src = 1", ContractStateDest).Find(&result).Error
	return result, err
}

type VDEScheTaskFromSrc struct {
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

func (VDEScheTaskFromSrc) TableName() string {
	return "vde_sche_task_from_src"
}

func (m *VDEScheTaskFromSrc) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEScheTaskFromSrc) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEScheTaskFromSrc) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEScheTaskFromSrc) GetAll() ([]VDEScheTaskFromSrc, error) {
	var result []VDEScheTaskFromSrc
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEScheTaskFromSrc) GetOneByID() (*VDEScheTaskFromSrc, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDEScheTaskFromSrc) GetAllByTaskUUID(TaskUUID string) ([]VDEScheTaskFromSrc, error) {
	result := make([]VDEScheTaskFromSrc, 0)
	err := DBConn.Table("vde_sche_task_from_src").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDEScheTaskFromSrc) GetAllByTaskUUIDAndTaskState(TaskUUID string, TaskState int64) ([]VDEScheTaskFromSrc, error) {
	result := make([]VDEScheTaskFromSrc, 0)
	err := DBConn.Table("vde_sche_task_from_src").Where("task_uuid = ? AND task_state=?", TaskUUID, TaskState).Find(&result).Error
	return result, err
}

func (m *VDEScheTaskFromSrc) GetOneByTaskUUID(TaskUUID string) (*VDEScheTaskFromSrc, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}
func (m *VDEScheTaskFromSrc) GetOneByTaskUUIDAndTaskState(TaskUUID string, TaskState int64) (*VDEScheTaskFromSrc, error) {
	err := DBConn.Where("task_uuid=? AND task_state=?", TaskUUID, TaskState).First(&m).Error
	return m, err
}

func (m *VDEScheTaskFromSrc) GetAllByTaskState(TaskState int64) ([]VDEScheTaskFromSrc, error) {
	result := make([]VDEScheTaskFromSrc, 0)
	err := DBConn.Table("vde_sche_task_from_src").Where("task_state = ?", TaskState).Find(&result).Error
	return result, err
}

func (m *VDEScheTaskFromSrc) GetOneByTaskState(TaskState int64) (bool, error) {
	return isFound(DBConn.Where("task_state = ?", TaskState).First(m))
}

func (m *VDEScheTaskFromSrc) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}

func (m *VDEScheTaskFromSrc) GetAllByContractStateAndChainState(ContractStateSrc int64, ContractStateDest int64, ChainState int64) ([]VDEScheTaskFromSrc, error) {
	result := make([]VDEScheTaskFromSrc, 0)
	err := DBConn.Table("vde_sche_task_from_src").Where("contract_state_src = ? AND contract_state_dest = ? AND chain_state = ?", ContractStateSrc, ContractStateDest, ChainState).Find(&result).Error
	return result, err
}

func (m *VDEScheTaskFromSrc) GetAllByContractStateSrc(ContractStateSrc int64) ([]VDEScheTaskFromSrc, error) {
