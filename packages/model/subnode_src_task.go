/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model
	Comment    string `gorm:"not null" json:"comment"`
	Parms      string `gorm:"type:jsonb" json:"parms"`
	TaskType   int64  `gorm:"not null" json:"task_type"`
	TaskState  int64  `gorm:"not null" json:"task_state"`

	TaskRunParms    string `gorm:"type:jsonb" json:"task_run_parms"`
	TaskRunState    int64  `gorm:"not null" json:"task_run_state"`
	TaskRunStateErr string `gorm:"not null" json:"task_run_state_err"`

	ChannelState    int64  `gorm:"not null" json:"channel_state"`
	ChannelStateErr string `gorm:"not null" json:"channel_state_err"`

	//TxHash     string `gorm:"not null" json:"tx_hash"`
	//ChainState int64  `gorm:"not null" json:"chain_state"`
	//BlockId    int64  `gorm:"not null" json:"block_id"`
	//ChainId    int64  `gorm:"not null" json:"chain_id"`
	//ChainErr   string `gorm:"not null" json:"chain_err"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (SubNodeSrcTask) TableName() string {
	return "subnode_src_task"
}

func (m *SubNodeSrcTask) Create() error {
	return DBConn.Create(&m).Error
}

func (m *SubNodeSrcTask) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *SubNodeSrcTask) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *SubNodeSrcTask) GetAll() ([]SubNodeSrcTask, error) {
	var result []SubNodeSrcTask
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *SubNodeSrcTask) GetOneByID() (*SubNodeSrcTask, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *SubNodeSrcTask) GetAllByTaskUUIDAndTaskState(TaskUUID string, TaskState int64) ([]SubNodeSrcTask, error) {
	result := make([]SubNodeSrcTask, 0)
	err := DBConn.Table("subnode_src_task").Where("task_uuid = ? AND task_state=?", TaskUUID, TaskState).Find(&result).Error
	return result, err
}

func (m *SubNodeSrcTask) GetAllByTaskUUID(TaskUUID string) ([]SubNodeSrcTask, error) {
	result := make([]SubNodeSrcTask, 0)
	err := DBConn.Table("subnode_src_task").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *SubNodeSrcTask) GetOneByTaskUUID(TaskUUID string) (*SubNodeSrcTask, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}

func (m *SubNodeSrcTask) GetOneByTaskUUIDAndTaskState(TaskUUID string, TaskState int64) (*SubNodeSrcTask, error) {
	err := DBConn.Where("task_uuid=? AND task_state=?", TaskUUID, TaskState).First(&m).Error
	return m, err
}
func (m *SubNodeSrcTask) GetAllByTaskState(TaskState int64) ([]SubNodeSrcTask, error) {
	result := make([]SubNodeSrcTask, 0)
	err := DBConn.Table("subnode_src_task").Where("task_state = ?", TaskState).Find(&result).Error
	return result, err
}

func (m *SubNodeSrcTask) GetOneByTaskState(TaskState int64) (bool, error) {
	return isFound(DBConn.Where("task_state = ?", TaskState).First(m))
}

func (m *SubNodeSrcTask) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(&m))
}

func (m *SubNodeSrcTask) GetOneTimeTasks() ([]SubNodeSrcTask, error) {
	result := make([]SubNodeSrcTask, 0)
	err := DBConn.Table("subnode_src_task").Where("task_state = 1 AND task_run_state = 0 AND task_type = 1").Find(&result).Error
	return result, err
}

func (m *SubNodeSrcTask) GetScheTimeTasks() ([]SubNodeSrcTask, error) {
	result := make([]SubNodeSrcTask, 0)
	err := DBConn.Table("subnode_src_task").Where("task_state = 1 AND task_run_state != 3  AND task_type = 2").Find(&result).Error
	return result, err
}

func (m *SubNodeSrcTask) GetAllByChannelState(ChannelState int64) ([]SubNodeSrcTask, error) {
	result := make([]SubNodeSrcTask, 0)
	err := DBConn.Table("subnode_src_task").Where("channel_state = ?", ChannelState).Find(&result).Error
	return result, err
}
