/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEScheTaskTime struct {
	ID             int64 `gorm:"primary_key; not null" json:"id"`
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
