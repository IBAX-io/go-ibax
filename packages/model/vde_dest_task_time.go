/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEDestTaskTime struct {
	ID             int64 `gorm:"primary_key; not null" json:"id"`
	SrcUpdateTime  int64 `gorm:"not null" json:"src_update_time"`
	ScheUpdateTime int64 `gorm:"not null" json:"sche_update_time"`
	CreateTime     int64 `gorm:"not null" json:"create_time"`
}

func (VDEDestTaskTime) TableName() string {
	return "vde_dest_task_time"
}

func (m *VDEDestTaskTime) Create() error {
	return DBConn.Create(&m).Error
}

	return m, err
}

func (m *VDEDestTaskTime) GetAll() ([]VDEDestTaskTime, error) {
	var result []VDEDestTaskTime
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEDestTaskTime) GetOneByID() (*VDEDestTaskTime, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
