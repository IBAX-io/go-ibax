/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEDestChainInfo struct {
	ID                  int64  `gorm:"primary_key; not null" json:"id"`
	BlockchainHttp      string `gorm:"not null" json:"blockchain_http"`
	BlockchainEcosystem string `gorm:"not null" json:"blockchain_ecosystem"`
	Comment             string `gorm:"not null" json:"comment"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDEDestChainInfo) TableName() string {
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
	return m, err
}
