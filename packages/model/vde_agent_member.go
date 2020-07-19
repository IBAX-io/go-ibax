/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEAgentMember struct {
	ID                   int64  `gorm:"primary_key; not null" json:"id"`
	VDEPubKey            string `gorm:"not null" json:"vde_pub_key"`
	VDEComment           string `gorm:"not null" json:"vde_comment"`
	VDEName              string `gorm:"not null" json:"vde_name"`
	VDEIp                string `gorm:"not null" json:"vde_ip"`
	VDEType              int64  `gorm:"not null" json:"vde_type"`
	ContractRunHttp      string `gorm:"not null" json:"contract_run_http"`
	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDEAgentMember) TableName() string {
	return "vde_agent_member"
}

func (m *VDEAgentMember) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEAgentMember) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEAgentMember) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEAgentMember) GetAll() ([]VDEAgentMember, error) {
	var result []VDEAgentMember
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEAgentMember) GetOneByID() (*VDEAgentMember, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDEAgentMember) GetOneByPubKey(VDEPubKey string) (*VDEAgentMember, error) {
	err := DBConn.Where("vde_pub_key=?", VDEPubKey).First(&m).Error
	return m, err
}

func (m *VDEAgentMember) GetAllByType(Type int64) ([]VDEAgentMember, error) {
	result := make([]VDEAgentMember, 0)
	err := DBConn.Table("vde_agent_member").Where("vde_type = ?", Type).Find(&result).Error
	return result, err
}
