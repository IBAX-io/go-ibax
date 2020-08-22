/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDESrcTaskTime struct {
	ID             int64 `gorm:"primary_key; not null" json:"id"`
	SrcUpdateTime  int64 `gorm:"not null" json:"src_update_time"`
	ScheUpdateTime int64 `gorm:"not null" json:"sche_update_time"`
	CreateTime     int64 `gorm:"not null" json:"create_time"`
}

func (VDESrcTaskTime) TableName() string {
	return "vde_src_task_time"
}

func (m *VDESrcTaskTime) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcTaskTime) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcTaskTime) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcTaskTime) Get() (*VDESrcTaskTime, error) {
	err := DBConn.First(&m).Error
	return m, err
}

func (m *VDESrcTaskTime) GetAll() ([]VDESrcTaskTime, error) {
	var result []VDESrcTaskTime
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcTaskTime) GetOneByID() (*VDESrcTaskTime, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
