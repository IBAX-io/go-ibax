/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import (
	"encoding/json"
	"fmt"
	"strconv"
)

const ecosysTable = "1_ecosystems"

// Ecosystem is model
type Ecosystem struct {
	ID             int64 `gorm:"primary_key;not null"`
	Name           string
	IsValued       bool
	EmissionAmount string `gorm:"type:jsonb"`
	TokenTitle     string
	TokenName      string
	TypeEmission   int64
	TypeWithdraw   int64
	Info           string `gorm:"type:jsonb"`
}

// TableName returns name of table
// only first ecosystem has this entity
func (sys *Ecosystem) TableName() string {
	return ecosysTable
}

// GetAllSystemStatesIDs is retrieving all ecosystems ids
func GetAllSystemStatesIDs() ([]int64, []string, error) {
	if !IsTable(ecosysTable) {
		return nil, nil, nil
	}

	ecosystems := new([]Ecosystem)
	if err := DBConn.Order("id asc").Find(&ecosystems).Error; err != nil {
		return nil, nil, err
	}

	ids := make([]int64, len(*ecosystems))
	names := make([]string, len(*ecosystems))
	for i, s := range *ecosystems {
		ids[i] = s.ID
		names[i] = s.Name
	}

	return ids, names, nil
}

// Get is fill receiver from db
func (sys *Ecosystem) Get(dbTx *DbTransaction, id int64) (bool, error) {
	return isFound(GetDB(dbTx).First(sys, "id = ?", id))
}

// Delete is deleting record
func (sys *Ecosystem) Delete(transaction *DbTransaction) error {
	return GetDB(transaction).Delete(sys).Error
}

func (sys *Ecosystem) IsOpenMultiFee() bool {
	if len(sys.Info) > 0 {
		var info map[string]interface{}
		json.Unmarshal([]byte(sys.Info), &info)
		if v, ok := info["multi_fee"]; ok {
			multi, _ := strconv.Atoi(fmt.Sprint(v))
			if multi == 1 {
				return true
			}
		}
	}
	return false
}
