/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

func (VDEDestTaskTime) TableName() string {
	return "vde_dest_task_time"
}

func (m *VDEDestTaskTime) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEDestTaskTime) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEDestTaskTime) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEDestTaskTime) Get() (*VDEDestTaskTime, error) {
	err := DBConn.First(&m).Error
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
