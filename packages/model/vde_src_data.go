/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
	DataState  int64  `gorm:"not null" json:"data_state"`
	DataErr    string `gorm:"not null" json:"data_err"`
	UpdateTime int64  `gorm:"not null" json:"update_time"`
	CreateTime int64  `gorm:"not null" json:"create_time"`
}

func (VDESrcData) TableName() string {
	return "vde_src_data"
}

func (m *VDESrcData) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcData) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcData) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcData) GetAll() ([]VDESrcData, error) {
	var result []VDESrcData
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcData) GetOneByID() (*VDESrcData, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDESrcData) GetOneByDataUUID(DataUUID string) (*VDESrcData, error) {
	err := DBConn.Where("data_uuid = ?", DataUUID).First(&m).Error
	return m, err
}
func (m *VDESrcData) GetAllByTaskUUID(TaskUUID string) ([]VDESrcData, error) {
	result := make([]VDESrcData, 0)
	err := DBConn.Table("vde_src_data").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDESrcData) GetAllByDataStatus(DataStatus int64) ([]VDESrcData, error) {
	result := make([]VDESrcData, 0)
	err := DBConn.Table("vde_src_data").Where("data_state = ?", DataStatus).Find(&result).Error
	return result, err
}

func (m *VDESrcData) GetOneByDataStatus(DataStatus int64) (bool, error) {
	return isFound(DBConn.Where("data_state = ?", DataStatus).First(m))
}
