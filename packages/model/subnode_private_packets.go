/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

// RollbackTx is model
type PrivatePackets struct {
	Hash string `gorm:"not null" json:"hash"`
	Data []byte `gorm:"not null" json:"data"`
	Time int64  `gorm:"not null" json:"time"`
}

// TableName returns name of table
func (PrivatePackets) TableName() string {
	return "subnode_private_packets"
}

// Create is creating record of model
func (pp *PrivatePackets) Create() error {
	return DBConn.Create(&pp).Error
}

func (pp *PrivatePackets) GetAll() ([]PrivatePackets, error) {
	var result []PrivatePackets
	err := DBConn.Find(&result).Error
	return result, err
}

func (pp *PrivatePackets) Get(Hash string) (PrivatePackets, error) {
	var m PrivatePackets
	err := DBConn.Where("hash=?", Hash).First(&m).Error
	return m, err
}

// GetDataByHash is returns private packet
func (pp *PrivatePackets) GetDataByHash(dbTransaction *DbTransaction, Hash string) ([]map[string]string, error) {
	return GetAllTx(dbTransaction, "SELECT * from subnode_private_packets WHERE hash = ? ORDER BY ID DESC", -1, Hash)
}

// DeleteByHash is deleting private packet by hash
func (pp *PrivatePackets) DeleteByHash(dbTransaction *DbTransaction) error {
	return GetDB(dbTransaction).Exec("DELETE FROM subnode_private_packets WHERE hash = ?", pp.Hash).Error
}

type PrivateFilePackets struct {
	TaskUUID   string `gorm:"column:task_uuid;not null" json:"task_uuid"`
	TaskName   string `gorm:"column:task_name;not null" json:"task_name"`
	TaskSender string `gorm:"column:task_sender;not null" json:"task_sender"`
	TaskType   string `gorm:"column:task_type;not null" json:"task_type"`
	Name       string `gorm:"column:name;not null" json:"name"`
	MimeType   string `gorm:"column:mimetype;not null" json:"mimetype"`
	Hash       string `gorm:"not null" json:"hash"`
	Data       []byte `gorm:"not null" json:"data"`
}

// TableName returns name of table
func (PrivateFilePackets) TableName() string {
	return "subnode_privatefile_packets"
}

// Create is creating record of model
func (pp *PrivateFilePackets) Create() error {
	return DBConn.Create(&pp).Error
}

func (pp *PrivateFilePackets) Get(Hash string) (PrivateFilePackets, error) {
	var m PrivateFilePackets
	err := DBConn.Where("hash=?", Hash).First(&m).Error
	return m, err
}

// GetDataByHash is returns privatefile packet
func (pp *PrivateFilePackets) GetDataByHash(dbTransaction *DbTransaction, Hash string) ([]map[string]string, error) {
	return GetAllTx(dbTransaction, "SELECT * from subnode_privatefile_packets WHERE hash = ? ORDER BY ID DESC", -1, Hash)
}

// DeleteByHash is deleting privatefile packet by hash
func (pp *PrivateFilePackets) DeleteByHash(dbTransaction *DbTransaction) error {
	return GetDB(dbTransaction).Exec("DELETE FROM subnode_privatefile_packets WHERE hash = ?", pp.Hash).Error
}

type PrivateFilePacketsHash struct {
	TaskUUID   string `gorm:"column:task_uuid;not null" json:"task_uuid"`
	TaskName   string `gorm:"column:task_name;not null" json:"task_name"`
	TaskSender string `gorm:"column:task_sender;not null" json:"task_sender"`
	TaskType   string `gorm:"column:task_type;not null" json:"task_type"`
	Name       string `gorm:"column:name;not null" json:"name"`
	MimeType   string `gorm:"column:mimetype;not null" json:"mimetype"`
	Hash       string `gorm:"not null" json:"hash"`
	Data       []byte `gorm:"column:spphdata;not null" json:"spphdata"`
}

// TableName returns name of table
func (PrivateFilePacketsHash) TableName() string {
	return "2_subnode_share_hash_100"
}

func (pp *PrivateFilePacketsHash) Get(Hash string) (PrivateFilePacketsHash, error) {
	var m PrivateFilePacketsHash
	err := DBConn.Where("hash=?", Hash).First(&m).Error
	return m, err
}

// GetDataByHash is returns privatefile packet
func (pp *PrivateFilePacketsHash) GetDataByHash(dbTransaction *DbTransaction, Hash string) ([]map[string]string, error) {
	return GetAllTx(dbTransaction, "SELECT * from 2_subnode_share_hash_100 WHERE hash = ? ORDER BY ID DESC", -1, Hash)
}

// DeleteByHash is deleting privatefile packet by hash
	TaskSender string `gorm:"column:task_sender;not null" json:"task_sender"`
	TaskType   string `gorm:"column:task_type;not null" json:"task_type"`
	Name       string `gorm:"column:name;not null" json:"name"`
	MimeType   string `gorm:"column:mimetype;not null" json:"mimetype"`
	Hash       string `gorm:"not null" json:"hash"`
	Data       []byte `gorm:"column:sppadata;not null" json:"sppadata"`
}

// TableName returns name of table
func (PrivateFilePacketsAll) TableName() string {
	return "2_subnode_share_data_502"
}

func (pp *PrivateFilePacketsAll) Get(Hash string) (PrivateFilePacketsAll, error) {
	var m PrivateFilePacketsAll
	err := DBConn.Where("hash=?", Hash).First(&m).Error
	return m, err
}

// GetDataByHash is returns privatefile packet
func (pp *PrivateFilePacketsAll) GetDataByHash(dbTransaction *DbTransaction, Hash string) ([]map[string]string, error) {
	return GetAllTx(dbTransaction, "SELECT * from 2_subnode_share_data_502 WHERE hash = ? ORDER BY ID DESC", -1, Hash)
}

// DeleteByHash is deleting privatefile packet by hash
func (pp *PrivateFilePacketsAll) DeleteByHash(dbTransaction *DbTransaction) error {
	return GetDB(dbTransaction).Exec("DELETE FROM 2_subnode_share_data_502 WHERE hash = ?", pp.Hash).Error
}

// Create is creating record of model
func (pp *PrivateFilePacketsAll) Create() error {
	return DBConn.Create(&pp).Error
}
