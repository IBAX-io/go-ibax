/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEAgentChainInfo struct {
	ID                  int64  `gorm:"primary_key; not null" json:"id"`
	BlockchainHttp      string `gorm:"not null" json:"blockchain_http"`
	BlockchainEcosystem string `gorm:"not null" json:"blockchain_ecosystem"`
	Comment             string `gorm:"not null" json:"comment"`
	LogMode             int64  `gorm:"not null" json:"log_mode"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDEAgentChainInfo) TableName() string {
	return "vde_agent_chain_info"
}

func (m *VDEAgentChainInfo) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEAgentChainInfo) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEAgentChainInfo) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEAgentChainInfo) Get() (*VDEAgentChainInfo, error) {
	err := DBConn.First(&m).Error
	return m, err
	return m, err
}
