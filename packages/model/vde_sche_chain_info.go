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
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
