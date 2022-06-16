/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package sqldb

import (
	"encoding/json"
)

// PlatformParameter is model
type PlatformParameter struct {
	ID         int64  `gorm:"primary_key;not null;"`
	Name       string `gorm:"not null;size:255"`
	Value      string `gorm:"not null"`
	Conditions string `gorm:"not null"`
}

// TableName returns name of table
func (sp PlatformParameter) TableName() string {
	return "1_platform_parameters"
}

// Get is retrieving model from database
func (sp *PlatformParameter) Get(dbTx *DbTransaction, name string) (bool, error) {
	return isFound(GetDB(dbTx).Where("name = ?", name).First(sp))
}

// GetTransaction is retrieving model from database using transaction
func (sp *PlatformParameter) GetTransaction(dbTx *DbTransaction, name string) (bool, error) {
	return isFound(GetDB(dbTx).Where("name = ?", name).First(sp))
}

// GetJSONField returns fields as json
func (sp *PlatformParameter) GetJSONField(jsonField string, name string) (string, error) {
	var result string
	err := DBConn.Table("1_platform_parameters").Where("name = ?", name).Select(jsonField).Row().Scan(&result)
	return result, err
}

// GetValueParameterByName returns value parameter by name
func (sp *PlatformParameter) GetValueParameterByName(name, value string) (*string, error) {
	var result *string
	err := DBConn.Raw(`SELECT value->'`+value+`' FROM "1_platform_parameters" WHERE name = ?`, name).Row().Scan(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetAllPlatformParameters returns all platform parameters
func GetAllPlatformParameters(dbTx *DbTransaction) ([]PlatformParameter, error) {
	parameters := new([]PlatformParameter)
	if err := GetDB(dbTx).Find(&parameters).Error; err != nil {
		return nil, err
	}
	return *parameters, nil
}

// ToMap is converting PlatformParameter to map
func (sp *PlatformParameter) ToMap() map[string]string {
	result := make(map[string]string, 0)
	result["name"] = sp.Name
	result["value"] = sp.Value
	result["conditions"] = sp.Conditions
	return result
}

// Update is update model
func (sp PlatformParameter) Update(dbTx *DbTransaction, value string) error {
	return GetDB(dbTx).Model(sp).Where("name = ?", sp.Name).Update(`value`, value).Error
}

// SaveArray is saving array
func (sp *PlatformParameter) SaveArray(dbTx *DbTransaction, list [][]string) error {
	ret, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return sp.Update(dbTx, string(ret))
}

func (sp *PlatformParameter) GetNumberOfHonorNodes() (int, error) {
	var hns []map[string]any
	f, err := sp.GetTransaction(nil, `honor_nodes`)
	if err != nil {
		return 0, err
	}
	if f {
		if len(sp.Value) > 0 {
			if err := json.Unmarshal([]byte(sp.Value), &hns); err != nil {
				return 0, err
			}
		}
	}

	if len(hns) == 0 || len(hns) == 1 {
		return 0, nil
	}
	return len(hns), nil
}
