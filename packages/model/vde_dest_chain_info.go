/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEDestChainInfo struct {
	ID                  int64  `gorm:"primary_key; not null" json:"id"`
	BlockchainHttp      string `gorm:"not null" json:"blockchain_http"`
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
