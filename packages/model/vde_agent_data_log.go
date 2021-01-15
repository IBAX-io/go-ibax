/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

type VDEAgentDataLog struct {
	ID                  int64  `gorm:"primary_key; not null" json:"id"`
	DataUUID            string `gorm:"not null" json:"data_uuid"`
	TaskUUID            string `gorm:"not null" json:"task_uuid"`
	Log                 string `gorm:"not null" json:"log"`
	LogType             int64  `gorm:"not null" json:"log_type"`
	LogSender           string `gorm:"not null" json:"log_sender"`
	BlockchainHttp      string `gorm:"not null" json:"blockchain_http"`
	BlockchainEcosystem string `gorm:"not null" json:"blockchain_ecosystem"`

	TxHash     string `gorm:"not null" json:"tx_hash"`
	ChainState int64  `gorm:"not null" json:"chain_state"`
	BlockId    int64  `gorm:"not null" json:"block_id"`
	ChainId    int64  `gorm:"not null" json:"chain_id"`
}

func (m *VDEAgentDataLog) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *VDEAgentDataLog) GetAll() ([]VDEAgentDataLog, error) {
	var result []VDEAgentDataLog
	err := DBConn.Find(&result).Error
	return result, err
}
func (m *VDEAgentDataLog) GetOneByID() (*VDEAgentDataLog, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *VDEAgentDataLog) GetAllByTaskUUID(TaskUUID string) ([]VDEAgentDataLog, error) {
	result := make([]VDEAgentDataLog, 0)
	err := DBConn.Table("vde_agent_data_log").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *VDEAgentDataLog) GetOneByTaskUUID(TaskUUID string) (*VDEAgentDataLog, error) {
	err := DBConn.Where("task_uuid=?", TaskUUID).First(&m).Error
	return m, err
}

func (m *VDEAgentDataLog) GetAllByChainState(ChainState int64) ([]VDEAgentDataLog, error) {
	result := make([]VDEAgentDataLog, 0)
	err := DBConn.Table("vde_agent_data_log").Where("chain_state = ?", ChainState).Find(&result).Error
	return result, err
}

func (m *VDEAgentDataLog) GetOneByChainState(ChainState int64) (bool, error) {
	return isFound(DBConn.Where("chain_state = ?", ChainState).First(m))
}
