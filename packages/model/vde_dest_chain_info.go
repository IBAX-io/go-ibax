/*---------------------------------------------------------------------------------------------
	return "vde_dest_chain_info"
}

func (m *VDEDestChainInfo) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEDestChainInfo) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEDestChainInfo) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEDestChainInfo) Get() (*VDEDestChainInfo, error) {
	err := DBConn.First(&m).Error
	return m, err
}

func (m *VDEDestChainInfo) GetAll() ([]VDEDestChainInfo, error) {
	var result []VDEDestChainInfo
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEDestChainInfo) GetOneByID() (*VDEDestChainInfo, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
