/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

type ShareDataStatus struct {
	ID               int64  `gorm:"primary_key; not null" json:"id"`
	TaskUUID         string `gorm:"not null" json:"task_uuid"`
	TaskName         string `gorm:"not null" json:"task_name"`
	TaskSender       string `gorm:"not null" json:"task_sender"`
	TaskType         string `gorm:"not null" json:"task_type"`
	Hash             string `gorm:"not null" json:"hash"`
	Data             []byte `gorm:"not null" json:"data"`
	Dist             string `gorm:"type:jsonb" json:"dist"`
	TcpSendState     int64  `gorm:"not null" json:"tcp_send_state"`
	TcpSendStateFlag string `gorm:"not null" json:"tcp_send_state_flag"`
	Ecosystem        int64  `gorm:"not null" json:"ecosystem"`
	BlockId          int64  `gorm:"not null" json:"block_id"`
	TxHash           []byte `gorm:"not null" json:"tx_hash"`
	ChainID          int64  `gorm:"not null" json:"chain_id"`
	ChainState       int64  `gorm:"not null" json:"chain_state"` //1:send  2:success  3:error
	ChainErr         string `json:"chain_err"`                   // error text
	Time             int64  `gorm:"not null" json:"time"`
}

func (ShareDataStatus) TableName() string {
	return "subnode_share_data_status"
}

func (m *ShareDataStatus) GetAll() ([]ShareDataStatus, error) {
	var result []ShareDataStatus
	err := DBConn.Find(&result).Error
	return result, err
}

func (m *ShareDataStatus) TaskDataStatusCreate() error {
	return DBConn.Create(&m).Error
}

func (m *ShareDataStatus) Updates() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *ShareDataStatus) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *ShareDataStatus) GetOneByID() (*ShareDataStatus, error) {
	err := DBConn.Where("id=?", m.ID).First(&m).Error
	return m, err
}

func (m *ShareDataStatus) GetAllByTaskUUID(TaskUUID string) ([]ShareDataStatus, error) {
	result := make([]ShareDataStatus, 0)
	err := DBConn.Table("subnode_share_data_status").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}

func (m *ShareDataStatus) GetOneByTcpStatus(tcp_send_state int64) (bool, error) {
	return isFound(DBConn.Where("tcp_send_state = ?", tcp_send_state).First(m))
}

func (m *ShareDataStatus) GetShareTaskStatusAndTcpStatus(chain_state, tcp_send_state int) (bool, error) {
	return isFound(DBConn.Where("chain_state = ? AND tcp_send_state = ?", chain_state, tcp_send_state).First(m))
}

func (m *ShareDataStatus) GetChainShareTaskStatus() (bool, error) {
	return isFound(DBConn.Where("chain_state = 1").First(m))
}

//
//
//type SDStatus struct {
//	ID           int64  `gorm:"primary_key; not null" json:"id"`
//	TaskUUID     string `gorm:"not null" json:"task_uuid"`
//	TaskName     string `gorm:"not null" json:"task_name"`
//	TaskSender   string `gorm:"not null" json:"task_sender"`
//	TaskType     string `gorm:"not null" json:"task_type"`
//	Hash         string `gorm:"not null" json:"hash"`
//	//Data         []byte `gorm:"not null" json:"data"`
//	Dist         string `gorm:"type:jsonb" json:"dist"`
//	//TcpSendState int64  `gorm:"not null" json:"tcp_send_state"`
//	TcpSendStateFlag string  `gorm:"not null" json:"tcp_send_state_flag"`
//	//Ecosystem    int64  `gorm:"not null" json:"ecosystem"`
//	//BlockId      int64  `gorm:"not null" json:"block_id"`
//	//TxHash       []byte `gorm:"not null" json:"tx_hash"`
//	//ChainID      int64  `gorm:"not null" json:"chain_id"`
//	//ChainState   int64  `gorm:"not null" json:"chain_state"`    //1:send  2:success  3:error
//	//ChainErr     string `json:"chain_err"`                      // error text
//	//Time         int64  `gorm:"not null" json:"time"`
//}
//
//func (SDStatus) TableName() string {
//	row := DBConn.Raw("SELECT COUNT(*) task_count FROM subnode_share_data_status").Select("task_count").Row()
//	err := row.Scan(&taskCount)
//
//	return taskCount, err
//}
//
//func (s *SDStatus) GetAllSDS() ([]SDStatus, error) {
//	var result []SDStatus
//	err := DBConn.Select("id, task_uuid, task_name, task_sender, task_type, hash, dist, tcp_send_state_flag").Find(&result).Error
//	return result, err
//}

type DataUpToChainStatus struct {
	ID       int64  `gorm:"primary_key; not null" json:"id"`
	TaskUUID string `gorm:"not null" json:"task_uuid"`
	TxHash   string `gorm:"not null" json:"tx_hash"`
	BlockId  int64  `gorm:"not null" json:"block_id"`
	ChainID  int64  `gorm:"not null" json:"chain_id"`
	ChainErr string `json:"chain_err"`
	Time     int64  `gorm:"not null" json:"time"`
}

func (DataUpToChainStatus) TableName() string {
	return "subnode_data_uptochain_status"
}

func (m *DataUpToChainStatus) Create() error {
	return DBConn.Create(&m).Error
}

func (m *DataUpToChainStatus) Update() error {
	return DBConn.Model(m).Updates(m).Error
}

func (m *DataUpToChainStatus) Delete() error {
	return DBConn.Delete(m).Error
}

func (m *DataUpToChainStatus) GetAllByTaskUUID(TaskUUID string) ([]DataUpToChainStatus, error) {
	result := make([]DataUpToChainStatus, 0)
	err := DBConn.Table("subnode_data_uptochain_status").Where("task_uuid = ?", TaskUUID).Find(&result).Error
	return result, err
}
