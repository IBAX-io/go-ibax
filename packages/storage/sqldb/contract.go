/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import "github.com/IBAX-io/go-ibax/packages/converter"

// Contract represents record of 1_contracts table
type Contract struct {
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Value       string `json:"value,omitempty"`
	WalletID    int64  `json:"wallet_id,omitempty"`
	Active      bool   `json:"active,omitempty"`
	TokenID     int64  `json:"token_id,omitempty"`
	Conditions  string `json:"conditions,omitempty"`
	AppID       int64  `json:"app_id,omitempty"`
	EcosystemID int64  `gorm:"column:ecosystem" json:"ecosystem_id,omitempty"`
}

// TableName returns name of table
func (c *Contract) TableName() string {
	return `1_contracts`
}

// GetList is retrieving records from database
func (c *Contract) GetList(offset, limit int) ([]Contract, error) {
	result := new([]Contract)
	err := DBConn.Table(c.TableName()).Offset(offset).Limit(limit).Order("id asc").Find(&result).Error
	return *result, err
}

// GetFromEcosystem retrieving ecosystem contracts from database
func (c *Contract) GetFromEcosystem(db *DbTransaction, ecosystem int64) ([]Contract, error) {
	result := new([]Contract)
	err := GetDB(db).Table(c.TableName()).Where("ecosystem = ?", ecosystem).Order("id asc").Find(&result).Error
	return *result, err
}

// Count returns count of records in table
func (c *Contract) Count(db *DbTransaction) (count int64, err error) {
	err = GetDB(db).Table(c.TableName()).Count(&count).Error
	return
}

func (c *Contract) GetListByEcosystem(offset, limit int) ([]Contract, error) {
	var list []Contract
	err := DBConn.Table(c.TableName()).Offset(offset).Limit(limit).
		Order("id asc").Where("ecosystem = ?", c.EcosystemID).
		Find(&list).Error
	return list, err
}

func (c *Contract) CountByEcosystem() (n int64, err error) {
	err = DBConn.Table(c.TableName()).Where("ecosystem = ?", c.EcosystemID).Count(&n).Error
	return
}

func (c *Contract) ToMap() (v map[string]string) {
	v = make(map[string]string)
	v["id"] = converter.Int64ToStr(c.ID)
	v["name"] = c.Name
	v["value"] = c.Value
	v["wallet_id"] = converter.Int64ToStr(c.WalletID)
	v["token_id"] = converter.Int64ToStr(c.TokenID)
	v["conditions"] = c.Conditions
	v["app_id"] = converter.Int64ToStr(c.AppID)
	v["ecosystem_id"] = converter.Int64ToStr(c.EcosystemID)
	return
}

// GetByApp returns all contracts belonging to selected app
func (c *Contract) GetByApp(appID int64, ecosystemID int64) ([]Contract, error) {
	var result []Contract
	err := DBConn.Select("id, name").Where("app_id = ? and ecosystem = ?", appID, ecosystemID).Find(&result).Error
	return result, err
}
