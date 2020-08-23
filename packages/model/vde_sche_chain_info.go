/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEScheChainInfo struct {
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
