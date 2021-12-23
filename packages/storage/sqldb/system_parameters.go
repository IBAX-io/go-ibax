/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package sqldb

import (
	"encoding/json"
)

// SystemParameter is model
type SystemParameter struct {
	ID         int64  `gorm:"primary_key;not null;"`
	Name       string `gorm:"not null;size:255"`
	Value      string `gorm:"not null"`
	Conditions string `gorm:"not null"`
}

// TableName returns name of table
func (sp SystemParameter) TableName() string {
	return "1_system_parameters"
}

// Get is retrieving model from database
func (sp *SystemParameter) Get(name string) (bool, error) {
	return isFound(DBConn.Where("name = ?", name).First(sp))
}

// GetTransaction is retrieving model from database using transaction
func (sp *SystemParameter) GetTransaction(transaction *DbTransaction, name string) (bool, error) {
	return isFound(GetDB(transaction).Where("name = ?", name).First(sp))
}

// GetJSONField returns fields as json
func (sp *SystemParameter) GetJSONField(jsonField string, name string) (string, error) {
	var result string
	err := DBConn.Table("1_system_parameters").Where("name = ?", name).Select(jsonField).Row().Scan(&result)
	return result, err
}

// GetValueParameterByName returns value parameter by name
func (sp *SystemParameter) GetValueParameterByName(name, value string) (*string, error) {
	var result *string
	err := DBConn.Raw(`SELECT value->'`+value+`' FROM "1_system_parameters" WHERE name = ?`, name).Row().Scan(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetAllSystemParameters returns all system parameters
func GetAllSystemParameters(transaction *DbTransaction) ([]SystemParameter, error) {
	parameters := new([]SystemParameter)
	if err := GetDB(transaction).Find(&parameters).Error; err != nil {
		return nil, err
	}
	return *parameters, nil
}

// ToMap is converting SystemParameter to map
func (sp *SystemParameter) ToMap() map[string]string {
	result := make(map[string]string, 0)
	result["name"] = sp.Name
	result["value"] = sp.Value
	result["conditions"] = sp.Conditions
	return result
}

// Update is update model
func (sp SystemParameter) Update(value string) error {
	return DBConn.Model(sp).Where("name = ?", sp.Name).Update(`value`, value).Error
}

// SaveArray is saving array
func (sp *SystemParameter) SaveArray(list [][]string) error {
	ret, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return sp.Update(string(ret))
}

func (sp *SystemParameter) GetNumberOfHonorNodes() (int, error) {
	var hns []map[string]interface{}
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
