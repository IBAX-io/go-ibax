/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
}

func (m *VDEScheTaskTime) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEScheTaskTime) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEScheTaskTime) Delete() error {
	return DBConn.Delete(m).Error
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
