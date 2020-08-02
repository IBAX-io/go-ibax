/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model
func (VDESrcMember) TableName() string {
	return "vde_src_member"
}

func (m *VDESrcMember) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcMember) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcMember) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcMember) GetAll() ([]VDESrcMember, error) {
	var result []VDESrcMember
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcMember) GetOneByID() (*VDESrcMember, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDESrcMember) GetOneByPubKey(VDEPubKey string) (*VDESrcMember, error) {
	err := DBConn.Where("vde_pub_key=?", VDEPubKey).First(&m).Error
	return m, err
}

func (m *VDESrcMember) GetAllByType(Type int64) ([]VDESrcMember, error) {
	result := make([]VDESrcMember, 0)
	err := DBConn.Table("vde_src_member").Where("vde_type = ?", Type).Find(&result).Error
	return result, err
}
