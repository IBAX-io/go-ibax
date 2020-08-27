/*---------------------------------------------------------------------------------------------
}

func (VDESrcChainInfo) TableName() string {
	return "vde_src_chain_info"
}

func (m *VDESrcChainInfo) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcChainInfo) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcChainInfo) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcChainInfo) Get() (*VDESrcChainInfo, error) {
	err := DBConn.First(&m).Error
	return m, err
}

func (m *VDESrcChainInfo) GetAll() ([]VDESrcChainInfo, error) {
	var result []VDESrcChainInfo
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcChainInfo) GetOneByID() (*VDESrcChainInfo, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
