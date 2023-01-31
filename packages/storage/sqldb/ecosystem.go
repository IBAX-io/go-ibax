/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"encoding/json"
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/consts"
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
	Digits         int64
	Info           string `gorm:"type:jsonb"`
	FeeModeInfo    string `json:"fee_mode_info" gorm:"type:jsonb"`
}

type FeeModeFlag struct {
	Flag           string `json:"flag"`
	ConversionRate string `json:"conversion_rate"`
}

func (f FeeModeFlag) FlagToInt() int64 {
	ret, _ := strconv.ParseInt(f.Flag, 10, 64)
	return ret

}

func (f FeeModeFlag) ConversionRateToFloat() float64 {
	ret, _ := strconv.ParseFloat(f.ConversionRate, 64)
	return ret
}

type Combustion struct {
	Flag    int64 `json:"flag"`
	Percent int64 `json:"percent"`
}

type FeeModeInfo struct {
	FeeModeDetail map[string]FeeModeFlag `json:"fee_mode_detail"`
	Combustion    Combustion             `json:"combustion"`
	FollowFuel    float64                `json:"follow_fuel"`
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

// GetCombustionPercents is ecosystem combustion percent
func GetCombustionPercents(db *DbTransaction, ids []int64) (map[int64]int64, error) {
	query :=
		`
			SELECT eco.id,(eco.fee_mode_info::json#>>'{combustion,percent}')::int as percent  
			FROM "1_parameters" as par
			LEFT JOIN "1_ecosystems" as eco ON par.ecosystem = eco.id 
			WHERE par.name = 'utxo_fee' and par.value = '1' and par.ecosystem IN ?
		`

	type Combustion1 struct {
		Id      int64
		Percent int64
	}

	var ret []Combustion1
	if len(ids) > 0 {
		err := GetDB(db).Raw(query, ids).Scan(&ret).Error
		if err != nil {
			return nil, err
		}
	}
	var result = make(map[int64]int64)
	for _, combustion := range ret {
		result[combustion.Id] = combustion.Percent
	}
	return result, nil
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
	if len(sys.TokenSymbol) == 0 || len(sys.FeeModeInfo) == 0 || sys.ID == consts.DefaultTokenEcosystem {
		return nil, nil
	}
	var info = &FeeModeInfo{}
	err := json.Unmarshal([]byte(sys.FeeModeInfo), info)
	if err != nil {
		return nil, errors.Wrapf(err, "Unmarshal eco[%d] feemode err", sys.ID)
	}
	return info, nil
}

// GetTokenSymbol is get ecosystem token symbol
func (sys *Ecosystem) GetTokenSymbol(dbTx *DbTransaction, id int64) (bool, error) {
	return isFound(GetDB(dbTx).Select("token_symbol").First(sys, "id = ?", id))
}
