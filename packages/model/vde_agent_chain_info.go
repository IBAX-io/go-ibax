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
	return DBConn.Delete(m).Error
}

func (m *VDEAgentChainInfo) Get() (*VDEAgentChainInfo, error) {
	err := DBConn.First(&m).Error
	return m, err
}

func (m *VDEAgentChainInfo) GetAll() ([]VDEAgentChainInfo, error) {
	var result []VDEAgentChainInfo
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEAgentChainInfo) GetOneByID() (*VDEAgentChainInfo, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}
