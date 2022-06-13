/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"encoding/json"

	"github.com/pkg/errors"
)

const ecosysTable = "1_ecosystems"

// Ecosystem is model
type Ecosystem struct {
	ID             int64 `gorm:"primary_key;not null"`
	Name           string
	IsValued       bool
	EmissionAmount string `gorm:"type:jsonb"`
	TokenSymbol    string
	TokenName      string
	TypeEmission   int64
	TypeWithdraw   int64
	Info           string `gorm:"type:jsonb"`
	FeeModeInfo    string `json:"fee_mode_info" gorm:"type:jsonb"`
}

type FeeModeFlag struct {
	Flag           int64   `json:"flag"`
	ConversionRate float64 `json:"conversion_rate"`
}

type Combustion struct {
	Flag    int64 `json:"flag"`
	Percent int64 `json:"percent"`
}

type FeeModeInfo struct {
	FeeModeDetail map[string]FeeModeFlag `json:"fee_mode_detail"`
	Combustion    Combustion             `json:"combustion"`
	FollowFuel    int64                  `json:"follow_fuel"`
}

// TableName returns name of table
// only first ecosystem has this entity
func (sys *Ecosystem) TableName() string {
	return ecosysTable
}

// GetAllSystemStatesIDs is retrieving all ecosystems ids
func GetAllSystemStatesIDs() ([]int64, []string, error) {
	if !NewDbTransaction(DBConn).IsTable(ecosysTable) {
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
func (sys *Ecosystem) Delete(dbTx *DbTransaction) error {
	return GetDB(dbTx).Delete(sys).Error
}

// FeeMode is get ecosystem fee mode
func (sys *Ecosystem) FeeMode() (*FeeModeInfo, error) {
	if len(sys.TokenSymbol) == 0 {
		return nil, nil
	}
	if len(sys.FeeModeInfo) == 0 {
		return nil, nil
	}
	var info = &FeeModeInfo{}
	err := json.Unmarshal([]byte(sys.FeeModeInfo), info)
	if err != nil {
		return nil, errors.Wrapf(err, "Unmarshal eco[%d] feemode err", sys.ID)
	}
	return info, nil
}
