/*---------------------------------------------------------------------------------------------
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDEScheChainInfo) TableName() string {
	return "vde_sche_chain_info"
}

func (m *VDEScheChainInfo) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEScheChainInfo) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEScheChainInfo) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEScheChainInfo) Get() (*VDEScheChainInfo, error) {
	err := DBConn.First(&m).Error
	return m, err
}

func (m *VDEScheChainInfo) GetAll() ([]VDEScheChainInfo, error) {
	var result []VDEScheChainInfo
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEScheChainInfo) GetOneByID() (*VDEScheChainInfo, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
