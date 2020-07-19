/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

type SubNodeSrcData struct {
	ID         int64  `gorm:"primary_key; not null" json:"id"`
	DataUUID   string `gorm:"not null" json:"data_uuid"`
	TaskUUID   string `gorm:"not null" json:"task_uuid"`

func (m *SubNodeSrcData) Create() error {
	return DBConn.Create(&m).Error
}

func (m *SubNodeSrcData) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *SubNodeSrcData) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *SubNodeSrcData) GetAll() ([]SubNodeSrcData, error) {
	var result []SubNodeSrcData
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *SubNodeSrcData) GetOneByID() (*SubNodeSrcData, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *SubNodeSrcData) GetOneByDataUUID(DataUUID string) (*SubNodeSrcData, error) {
	err := DBConn.Where("data_uuid = ?", DataUUID).First(&m).Error
	return m, err
}
func (m *SubNodeSrcData) GetAllByTaskUUID(TaskUUID string) ([]SubNodeSrcData, error) {
	result := make([]SubNodeSrcData, 0)
	err := DBConn.Table("subnode_src_data").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *SubNodeSrcData) GetAllByDataStatus(DataStatus int64) ([]SubNodeSrcData, error) {
	result := make([]SubNodeSrcData, 0)
	err := DBConn.Table("subnode_src_data").Where("data_state = ?", DataStatus).Find(&result).Error
	return result, err
}

func (m *SubNodeSrcData) GetOneByDataStatus(DataStatus int64) (bool, error) {
	return isFound(DBConn.Where("data_state = ?", DataStatus).First(m))
}
