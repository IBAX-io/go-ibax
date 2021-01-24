/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEScheChainInfo struct {
	ID                  int64  `gorm:"primary_key; not null" json:"id"`
	BlockchainHttp      string `gorm:"not null" json:"blockchain_http"`
	BlockchainEcosystem string `gorm:"not null" json:"blockchain_ecosystem"`
	Comment             string `gorm:"not null" json:"comment"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDEScheChainInfo) TableName() string {
	return "vde_sche_chain_info"
}

func (m *VDEScheChainInfo) Create() error {
	return DBConn.Create(&m).Error
}

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
