/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEAgentData struct {
	UpdateTime     int64  `gorm:"not null" json:"update_time"`
	CreateTime     int64  `gorm:"not null" json:"create_time"`
}

func (VDEAgentData) TableName() string {
	return "vde_agent_data"
}

func (m *VDEAgentData) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEAgentData) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEAgentData) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEAgentData) GetAll() ([]VDEAgentData, error) {
	var result []VDEAgentData
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEAgentData) GetOneByID() (*VDEAgentData, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
func (m *VDEAgentData) GetOneByDataUUID(DataUUID string) (*VDEAgentData, error) {
	err := DBConn.Where("data_uuid=?", DataUUID).First(&m).Error
	return m, err
}
func (m *VDEAgentData) GetOneByTaskUUID(TaskUUID string) (*VDEAgentData, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}
func (m *VDEAgentData) GetAllByTaskUUID(TaskUUID string) ([]VDEAgentData, error) {
	result := make([]VDEAgentData, 0)
	err := DBConn.Table("vde_agent_data").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDEAgentData) GetAllByDataSendStatus(DataSendStatus int64) ([]VDEAgentData, error) {
	result := make([]VDEAgentData, 0)
	err := DBConn.Table("vde_agent_data").Where("data_send_state = ?", DataSendStatus).Find(&result).Error
	return result, err
}

func (m *VDEAgentData) GetOneByDataStatus(DataStatus int64) (bool, error) {
	return isFound(DBConn.Where("data_state = ?", DataStatus).First(m))
}
