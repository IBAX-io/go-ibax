/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEScheTaskTime struct {
	ID             int64 `gorm:"primary_key; not null" json:"id"`
	SrcUpdateTime  int64 `gorm:"not null" json:"src_update_time"`
	ScheUpdateTime int64 `gorm:"not null" json:"sche_update_time"`
	CreateTime     int64 `gorm:"not null" json:"create_time"`
}

func (VDEScheTaskTime) TableName() string {
	return "vde_sche_task_time"
}

func (m *VDEScheTaskTime) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEScheTaskTime) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

}

func (m *VDEScheTaskTime) Get() (*VDEScheTaskTime, error) {
	err := DBConn.First(&m).Error
	return m, err
}

func (m *VDEScheTaskTime) GetAll() ([]VDEScheTaskTime, error) {
	var result []VDEScheTaskTime
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEScheTaskTime) GetOneByID() (*VDEScheTaskTime, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
