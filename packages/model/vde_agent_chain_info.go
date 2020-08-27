/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model


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
