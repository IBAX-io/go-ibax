/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

type SubNodeAgentData struct {
	ID       int64  `gorm:"primary_key; not null" json:"id"`
	DataUUID string `gorm:"not null" json:"data_uuid"`
	TaskUUID string `gorm:"not null" json:"task_uuid"`
	Hash     string `gorm:"not null" json:"hash"`

func (SubNodeAgentData) TableName() string {
	return "subnode_agent_data"
}

func (m *SubNodeAgentData) Create() error {
	return DBConn.Create(&m).Error
}

func (m *SubNodeAgentData) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *SubNodeAgentData) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *SubNodeAgentData) GetAll() ([]SubNodeAgentData, error) {
	var result []SubNodeAgentData
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *SubNodeAgentData) GetOneByID() (*SubNodeAgentData, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
func (m *SubNodeAgentData) GetOneByDataUUID(DataUUID string) (*SubNodeAgentData, error) {
	err := DBConn.Where("data_uuid=?", DataUUID).First(&m).Error
	return m, err
}
func (m *SubNodeAgentData) GetOneByTaskUUID(TaskUUID string) (*SubNodeAgentData, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}
func (m *SubNodeAgentData) GetAllByTaskUUID(TaskUUID string) ([]SubNodeAgentData, error) {
	result := make([]SubNodeAgentData, 0)
	err := DBConn.Table("subnode_agent_data").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *SubNodeAgentData) GetAllByDataSendStatus(DataSendStatus int64) ([]SubNodeAgentData, error) {
	result := make([]SubNodeAgentData, 0)
	err := DBConn.Table("subnode_agent_data").Where("data_send_state = ?", DataSendStatus).Find(&result).Error
	return result, err
}

func (m *SubNodeAgentData) GetOneByDataStatus(DataStatus int64) (bool, error) {
	return isFound(DBConn.Where("data_state = ?", DataStatus).First(m))
}
