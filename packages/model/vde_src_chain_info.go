/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDESrcChainInfo struct {
	ID                  int64  `gorm:"primary_key; not null" json:"id"`
	BlockchainHttp      string `gorm:"not null" json:"blockchain_http"`
	BlockchainEcosystem string `gorm:"not null" json:"blockchain_ecosystem"`
	Comment             string `gorm:"not null" json:"comment"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
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
}
func (m *VDESrcChainInfo) GetOneByID() (*VDESrcChainInfo, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
