/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

type SubNodeSrcDataStatus struct {
	ID       int64  `gorm:"primary_key; not null" json:"id"`
	DataSendState  int64  `gorm:"not null" json:"data_send_state"`
	DataSendErr    string `gorm:"not null" json:"data_send_err"`
	UpdateTime     int64  `gorm:"not null" json:"update_time"`
	CreateTime     int64  `gorm:"not null" json:"create_time"`
}

func (SubNodeSrcDataStatus) TableName() string {
	return "subnode_src_data_status"
}

func (m *SubNodeSrcDataStatus) Create() error {
	return DBConn.Create(&m).Error
}

func (m *SubNodeSrcDataStatus) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *SubNodeSrcDataStatus) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *SubNodeSrcDataStatus) GetAll() ([]SubNodeSrcDataStatus, error) {
	var result []SubNodeSrcDataStatus
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *SubNodeSrcDataStatus) GetOneByID() (*SubNodeSrcDataStatus, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *SubNodeSrcDataStatus) GetAllByTaskUUID(TaskUUID string) ([]SubNodeSrcDataStatus, error) {
	result := make([]SubNodeSrcDataStatus, 0)
	err := DBConn.Table("subnode_src_data_status").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *SubNodeSrcDataStatus) GetAllByDataSendStatus(DataSendStatus int64) ([]SubNodeSrcDataStatus, error) {
	result := make([]SubNodeSrcDataStatus, 0)
	err := DBConn.Table("subnode_src_data_status").Where("data_send_state = ?", DataSendStatus).Find(&result).Error
	return result, err
}

func (m *SubNodeSrcDataStatus) GetAllByDataSendStatusAndAgentMode(DataSendStatus int64, AgentMode int64) ([]SubNodeSrcDataStatus, error) {
	result := make([]SubNodeSrcDataStatus, 0)
	err := DBConn.Table("subnode_src_data_status").Where("data_send_state = ? AND agent_mode = ?", DataSendStatus, AgentMode).Find(&result).Error
	return result, err
}

func (m *SubNodeSrcDataStatus) GetOneByDataSendStatus(DataSendStatus int64) (bool, error) {
	return isFound(DBConn.Where("data_send_state = ?", DataSendStatus).First(m))
}
