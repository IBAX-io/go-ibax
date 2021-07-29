/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEScheMember struct {
	ID                   int64  `gorm:"primary_key; not null" json:"id"`
	VDEPubKey            string `gorm:"not null" json:"vde_pub_key"`
	VDEComment           string `gorm:"not null" json:"vde_comment"`
	VDEName              string `gorm:"not null" json:"vde_name"`
	VDEIp                string `gorm:"not null" json:"vde_ip"`

func (m *VDEScheMember) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDEScheMember) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEScheMember) GetAll() ([]VDEScheMember, error) {
	var result []VDEScheMember
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEScheMember) GetOneByID() (*VDEScheMember, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDEScheMember) GetOneByPubKey(VDEPubKey string) (*VDEScheMember, error) {
	err := DBConn.Where("vde_pub_key=?", VDEPubKey).First(&m).Error
	return m, err
}

func (m *VDEScheMember) GetAllByType(Type int64) ([]VDEScheMember, error) {
	result := make([]VDEScheMember, 0)
	err := DBConn.Table("vde_sche_member").Where("vde_type = ?", Type).Find(&result).Error
	return result, err
}
