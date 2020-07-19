/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEDestDataHash struct {
	ID                  int64  `gorm:"primary_key; not null" json:"id"`
	DataUUID            string `gorm:"not null" json:"data_uuid"`
	TaskUUID            string `gorm:"not null" json:"task_uuid"`
	Hash                string `gorm:"not null" json:"hash"`
	BlockchainHttp      string `gorm:"not null" json:"blockchain_http"`
	BlockchainEcosystem string `gorm:"not null" json:"blockchain_ecosystem"`

	//TxHash                 string `gorm:"not null" json:"tx_hash"`
	//ChainState             int64  `gorm:"not null" json:"chain_state"`
	//BlockId                int64  `gorm:"not null" json:"block_id"`
	//ChainId                int64  `gorm:"not null" json:"chain_id"`
	//ChainErr               string `gorm:"not null" json:"chain_err"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (VDEDestDataHash) TableName() string {
	return "vde_dest_data_hash"
}

func (m *VDEDestDataHash) Create() error {
	return DBConn.Create(&m).Error
}

func (m *VDEDestDataHash) Updates() error {
func (m *VDEDestDataHash) GetOneByID() (*VDEDestDataHash, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDEDestDataHash) GetAllByTaskUUID(TaskUUID string) ([]VDEDestDataHash, error) {
	result := make([]VDEDestDataHash, 0)
	err := DBConn.Table("vde_dest_data_hash").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDEDestDataHash) GetOneByTaskUUID(TaskUUID string) (*VDEDestDataHash, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}

func (m *VDEDestDataHash) GetOneByTaskUUIDAndDataUUID(TaskUUID string, DataUUID string) (*VDEDestDataHash, error) {
	err := DBConn.Where("task_uuid=? AND data_uuid=?", TaskUUID, DataUUID).First(&m).Error
	return m, err
}

func (m *VDEDestDataHash) GetAllByChainState(ChainState int64) ([]VDEDestDataHash, error) {
	result := make([]VDEDestDataHash, 0)
	err := DBConn.Table("vde_dest_data_hash").Where("chain_state = ?", ChainState).Find(&result).Error
	return result, err
}

func (m *VDEDestDataHash) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}
