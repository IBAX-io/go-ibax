/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDESrcDataHash struct {
	ID                  int64  `gorm:"primary_key; not null" json:"id"`
	DataUUID            string `gorm:"not null" json:"data_uuid"`
	TaskUUID            string `gorm:"not null" json:"task_uuid"`
	Hash                string `gorm:"not null" json:"hash"`
	BlockchainHttp      string `gorm:"not null" json:"blockchain_http"`
	BlockchainEcosystem string `gorm:"not null" json:"blockchain_ecosystem"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDESrcDataHash) TableName() string {
	return "vde_src_data_hash"
}

func (m *VDESrcDataHash) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDESrcDataHash) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *VDESrcDataHash) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDESrcDataHash) GetAll() ([]VDESrcDataHash, error) {
	var result []VDESrcDataHash
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDESrcDataHash) GetOneByID() (*VDESrcDataHash, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDESrcDataHash) GetAllByTaskUUID(TaskUUID string) ([]VDESrcDataHash, error) {
	result := make([]VDESrcDataHash, 0)
	err := DBConn.Table("vde_src_data_hash").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDESrcDataHash) GetOneByTaskUUID(TaskUUID string) (*VDESrcDataHash, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}

func (m *VDESrcDataHash) GetAllByChainState(ChainState int64) ([]VDESrcDataHash, error) {
	result := make([]VDESrcDataHash, 0)
	err := DBConn.Table("vde_src_data_hash").Where("chain_state = ?", ChainState).Find(&result).Error
	return result, err
}

func (m *VDESrcDataHash) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}
