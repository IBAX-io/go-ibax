/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDESrcTask struct {
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

	TxHash     string `gorm:"not null" json:"tx_hash"`
	ChainState int64  `gorm:"not null" json:"chain_state"`
	BlockId    int64  `gorm:"not null" json:"block_id"`
	ChainId    int64  `gorm:"not null" json:"chain_id"`
	ChainErr   string `gorm:"not null" json:"chain_err"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDESrcTask) TableName() string {
	return "vde_src_task"
}

func (m *VDESrcTask) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcTask) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcTask) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcTask) GetAll() ([]VDESrcTask, error) {
	var result []VDESrcTask
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcTask) GetOneByID() (*VDESrcTask, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDESrcTask) GetAllByTaskUUIDAndTaskState(TaskUUID string, TaskState int64) ([]VDESrcTask, error) {
	result := make([]VDESrcTask, 0)
	err := DBConn.Table("vde_src_task").Where("task_uuid = ? AND task_state=?", TaskUUID, TaskState).Find(&result).Error
	return result, err
}

func (m *VDESrcTask) GetAllByTaskUUID(TaskUUID string) ([]VDESrcTask, error) {
	result := make([]VDESrcTask, 0)
	err := DBConn.Table("vde_src_task").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

// func (m *VDESrcTask) GetOneByTaskUUID(TaskUUID string) (*VDESrcTask, bool) {
// 	var (
// 		result VDESrcTask
// 		b	bool
// 	)
// 	b = DBConn.Where("task_uuid = ?", TaskUUID).First(m).RecordNotFound()
// 	return &result, b
// }
func (m *VDESrcTask) GetOneByTaskUUID(TaskUUID string) (*VDESrcTask, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}

func (m *VDESrcTask) GetOneByTaskUUIDAndTaskState(TaskUUID string, TaskState int64) (*VDESrcTask, error) {
	err := DBConn.Where("task_uuid=? AND task_state=?", TaskUUID, TaskState).First(&m).Error
	return m, err
}
func (m *VDESrcTask) GetAllByTaskState(TaskState int64) ([]VDESrcTask, error) {
	result := make([]VDESrcTask, 0)
	err := DBConn.Table("vde_src_task").Where("task_state = ?", TaskState).Find(&result).Error
	return result, err
}

func (m *VDESrcTask) GetOneByTaskState(TaskState int64) (bool, error) {
	return isFound(DBConn.Where("task_state = ?", TaskState).First(m))
}

func (m *VDESrcTask) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}

func (m *VDESrcTask) GetAllByContractStateAndChainState(ContractStateSrc int64, ContractStateDest int64, ChainState int64) ([]VDESrcTask, error) {
	result := make([]VDESrcTask, 0)
	err := DBConn.Table("vde_src_task").Where("contract_state_src = ? AND contract_state_dest = ? AND chain_state = ?", ContractStateSrc, ContractStateDest, ChainState).Find(&result).Error
	return result, err
}

func (m *VDESrcTask) GetAllByContractStateSrc(ContractStateSrc int64) ([]VDESrcTask, error) {
	result := make([]VDESrcTask, 0)
	err := DBConn.Table("vde_src_task").Where("contract_state_src = ?", ContractStateSrc).Find(&result).Error
	return result, err
}

func (m *VDESrcTask) GetAllByContractStateDest(ContractStateDest int64) ([]VDESrcTask, error) {
	result := make([]VDESrcTask, 0)
	err := DBConn.Table("vde_src_task").Where("contract_state_dest = ?", ContractStateDest).Find(&result).Error
	return result, err
}

func (m *VDESrcTask) GetOneTimeTasks() ([]VDESrcTask, error) {
	result := make([]VDESrcTask, 0)
	err := DBConn.Table("vde_src_task").Where("task_state = 1 AND contract_state_src = 1 AND contract_state_dest = 0 AND chain_state = 2 AND task_run_state = 0 AND task_type = 1").Find(&result).Error
	return result, err
}

func (m *VDESrcTask) GetScheTimeTasks() ([]VDESrcTask, error) {
	result := make([]VDESrcTask, 0)
	err := DBConn.Table("vde_src_task").Where("task_state = 1 AND contract_state_src = 1 AND contract_state_dest = 0 AND chain_state = 2 AND task_run_state != 3  AND task_type = 2").Find(&result).Error
	return result, err
}

//func UpdateVDESrcTaskByID_TaskState(transaction *DbTransaction, TaskState int64, TaskID int64) (int64, error) {
//	query := GetDB(transaction).Exec("UPDATE vde_src_task SET task_state = ? WHERE id = ?", TaskState, TaskID)
//	return query.RowsAffected, query.Error
//}

type VDESrcTaskFromSche struct {
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

func (VDESrcTaskFromSche) TableName() string {
	return "vde_src_task_from_sche"
}

func (m *VDESrcTaskFromSche) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcTaskFromSche) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcTaskFromSche) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcTaskFromSche) GetAll() ([]VDESrcTaskFromSche, error) {
	var result []VDESrcTaskFromSche
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcTaskFromSche) GetOneByID() (*VDESrcTaskFromSche, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDESrcTaskFromSche) GetAllByTaskUUID(TaskUUID string) ([]VDESrcTaskFromSche, error) {
	err := DBConn.Table("vde_src_task_from_sche").Where("task_uuid = ? AND task_state=?", TaskUUID, TaskState).Find(&result).Error
	return result, err
}

func (m *VDESrcTaskFromSche) GetOneByTaskUUID(TaskUUID string) (*VDESrcTaskFromSche, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}

func (m *VDESrcTaskFromSche) GetOneByTaskUUIDAndTaskState(TaskUUID string, TaskState int64) (*VDESrcTaskFromSche, error) {
	err := DBConn.Where("task_uuid=? AND task_state=?", TaskUUID, TaskState).First(&m).Error
	return m, err
}

func (m *VDESrcTaskFromSche) GetAllByTaskState(TaskState int64) ([]VDESrcTaskFromSche, error) {
	result := make([]VDESrcTaskFromSche, 0)
	err := DBConn.Table("vde_src_task_from_sche").Where("task_state = ?", TaskState).Find(&result).Error
	return result, err
}

func (m *VDESrcTaskFromSche) GetOneByTaskState(TaskState int64) (bool, error) {
	return isFound(DBConn.Where("task_state = ?", TaskState).First(m))
}

//func (m *VDESrcTaskFromSche) GetOneByChainState(ChainState int64) (*VDESrcTaskFromSche, bool) {
//	var (
//		result VDESrcTaskFromSche
//		b	bool
//	)
//	b = DBConn.Where("chain_state = ?", ChainState).First(m).RecordNotFound()
//	return &result, b
//}

func (m *VDESrcTaskFromSche) GetAllByContractStateAndChainState(ContractStateSrc int64, ContractStateDest int64, ChainState int64) ([]VDESrcTaskFromSche, error) {
	result := make([]VDESrcTaskFromSche, 0)
	err := DBConn.Table("vde_src_task_from_sche").Where("contract_state_src = ? AND contract_state_dest = ? AND chain_state = ?", ContractStateSrc, ContractStateDest, ChainState).Find(&result).Error
	return result, err
}

func (m *VDESrcTaskFromSche) GetAllByContractStateSrc(ContractStateSrc int64) ([]VDESrcTaskFromSche, error) {
	result := make([]VDESrcTaskFromSche, 0)
	err := DBConn.Table("vde_src_task_from_sche").Where("contract_state_src = ?", ContractStateSrc).Find(&result).Error
	return result, err
}

func (m *VDESrcTaskFromSche) GetAllByContractStateDest(ContractStateDest int64) ([]VDESrcTaskFromSche, error) {
	result := make([]VDESrcTaskFromSche, 0)
	err := DBConn.Table("vde_src_task_from_sche").Where("contract_state_dest = ?", ContractStateDest).Find(&result).Error
	return result, err
}

func (m *VDESrcTaskFromSche) GetOneTimeTasks() ([]VDESrcTaskFromSche, error) {
	result := make([]VDESrcTaskFromSche, 0)
	err := DBConn.Table("vde_src_task_from_sche").Where("task_state = 1 AND contract_state_src = 0 AND contract_state_dest = 0 AND task_run_state = 0 AND task_type = 1").Find(&result).Error
	return result, err
}

func (m *VDESrcTaskFromSche) GetScheTimeTasks() ([]VDESrcTaskFromSche, error) {
	result := make([]VDESrcTaskFromSche, 0)
	err := DBConn.Table("vde_src_task_from_sche").Where("task_state = 1 AND contract_state_src = 0 AND contract_state_dest = 0 AND task_run_state != 3  AND task_type = 2").Find(&result).Error
	return result, err
}
