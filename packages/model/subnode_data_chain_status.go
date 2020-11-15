/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

type SubNodeSrcDataChainStatus struct {
	ID       int64  `gorm:"primary_key; not null" json:"id"`
	DataUUID string `gorm:"not null" json:"data_uuid"`
	TaskUUID string `gorm:"not null" json:"task_uuid"`
	Hash     string `gorm:"not null" json:"hash"`
	Data     []byte `gorm:"column:data;not null" json:"data"`
	DataInfo string `gorm:"type:jsonb" json:"data_info"`
	TranMode int64  `gorm:"not null" json:"tran_mode"`
	BlockchainHttp      string `gorm:"not null" json:"blockchain_http"`
	BlockchainEcosystem string `gorm:"not null" json:"blockchain_ecosystem"`

	TxHash     string `gorm:"not null" json:"tx_hash"`
	ChainState int64  `gorm:"not null" json:"chain_state"`
	BlockId    int64  `gorm:"not null" json:"block_id"`
	ChainId    int64  `gorm:"not null" json:"chain_id"`
	ChainErr   string `gorm:"not null" json:"chain_err"`

	UpdateTime int64 `gorm:"not null" json:"update_time"`
	CreateTime int64 `gorm:"not null" json:"create_time"`
}

func (SubNodeSrcDataChainStatus) TableName() string {
	return "subnode_src_data_chain_status"
}

func (m *SubNodeSrcDataChainStatus) Create() error {
	return DBConn.Create(&m).Error
}

func (m *SubNodeSrcDataChainStatus) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *SubNodeSrcDataChainStatus) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *SubNodeSrcDataChainStatus) GetAll() ([]SubNodeSrcDataChainStatus, error) {
	var result []SubNodeSrcDataChainStatus
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *SubNodeSrcDataChainStatus) GetOneByID() (*SubNodeSrcDataChainStatus, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *SubNodeSrcDataChainStatus) GetAllByTaskUUID(TaskUUID string) ([]SubNodeSrcDataChainStatus, error) {
	result := make([]SubNodeSrcDataChainStatus, 0)
	err := DBConn.Table("subnode_src_data_chain_status").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *SubNodeSrcDataChainStatus) GetOneByTaskUUID(TaskUUID string) (*SubNodeSrcDataChainStatus, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}

func (m *SubNodeSrcDataChainStatus) GetAllByChainState(ChainState int64) ([]SubNodeSrcDataChainStatus, error) {
	result := make([]SubNodeSrcDataChainStatus, 0)
	err := DBConn.Table("subnode_src_data_chain_status").Where("chain_state = ?", ChainState).Find(&result).Error
	return result, err
}

func (m *SubNodeSrcDataChainStatus) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}
