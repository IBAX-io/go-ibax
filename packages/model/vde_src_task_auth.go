/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDESrcTaskAuth struct {
	ID                   int64  `gorm:"primary_key; not null" json:"id"`
	TaskUUID             string `gorm:"not null" json:"task_uuid"`
	Comment              string `gorm:"not null" json:"comment"`
	VDEPubKey            string `gorm:"not null" json:"vde_pub_key"`
	ContractRunHttp      string `gorm:"not null" json:"contract_run_http"`
	ContractRunEcosystem string `gorm:"not null" json:"contract_run_ecosystem"`
	ChainState           int64  `gorm:"not null" json:"chain_state"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDESrcTaskAuth) TableName() string {
	return "vde_src_task_auth"
}

func (m *VDESrcTaskAuth) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcTaskAuth) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcTaskAuth) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcTaskAuth) GetAll() ([]VDESrcTaskAuth, error) {
	var result []VDESrcTaskAuth
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcTaskAuth) GetOneByID() (*VDESrcTaskAuth, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDESrcTaskAuth) GetOneByPubKey(VDEPubKey string) (*VDESrcTaskAuth, error) {
	err := DBConn.Where("vde_pub_key=?", VDEPubKey).First(&m).Error
	return m, err
}

func (m *VDESrcTaskAuth) GetAllByChainState(ChainState int64) ([]VDESrcTaskAuth, error) {
	result := make([]VDESrcTaskAuth, 0)
	err := DBConn.Table("vde_src_task_auth").Where("chain_state = ?", ChainState).Find(&result).Error
	return result, err
}

func (m *VDESrcTaskAuth) GetOneByTaskUUID(TaskUUID string) (*VDESrcTaskAuth, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
