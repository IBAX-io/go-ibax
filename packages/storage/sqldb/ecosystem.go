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
	// Frozen is not 0 for an ecosystem whose transactions the nodes refuse (a suspended agency of
	// the federal chain)
	Frozen int64
}

type FeeModeFlag struct {
	Flag           string `json:"flag"`
	ConversionRate string `json:"conversion_rate"`
}

type EcoParam struct {
	Id      int64
	Percent int64
	Digits  int
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
	if err := DBConn.Select("id,name").Order("id asc").Find(&ecosystems).Error; err != nil {
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

func GetAllSystemCount() (int64, error) {
	if !NewDbTransaction(DBConn).IsTable(ecosysTable) {
		return 0, nil
	}

	var total int64
	if err := DBConn.Model(Ecosystem{}).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// GetEcoParam is ecosystem combustion percent, digits
func GetEcoParam(db *DbTransaction, ids []int64) ([]EcoParam, error) {
	query :=
		`
			SELECT eco.id,(eco.fee_mode_info::json#>>'{combustion,percent}')::int as percent ,eco.digits
			FROM "1_parameters" as par
			LEFT JOIN "1_ecosystems" as eco ON par.ecosystem = eco.id 
			WHERE par.name = 'utxo_fee' and par.value = '1' and par.ecosystem IN ?
		`

	var ret []EcoParam
	if len(ids) > 0 {
		err := GetDB(db).Raw(query, ids).Scan(&ret).Error
		if err != nil {
			return nil, err
		}
	}

	return ret, nil
}

// Get reads the ecosystem id. A transaction reads each once, and the views of a block share what
// they read; an ecosystem with another id than id already is read as First reads it.
func (sys *Ecosystem) Get(dbTx *DbTransaction, id int64) (bool, error) {
	if sys.ID != 0 && sys.ID != id {
		return isFound(GetDB(dbTx).First(sys, "id = ?", id))
	}
	row, err := cached(dbTx, ecosysTable, "row:"+strconv.FormatInt(id, 10), func() (*Ecosystem, error) {
		e := new(Ecosystem)
		found, err := isFound(GetDB(dbTx).First(e, "id = ?", id))
		if err != nil || !found {
			return nil, err
		}
		return e, nil
	})
	if err != nil || row == nil {
		return false, err
	}
	*sys = *row
	return true, nil
}

// IsFrozen reports whether the nodes refuse the transactions of the ecosystem. Ecosystem 1, which
// governs the others and resumes them, is never frozen.
func IsFrozen(dbTx *DbTransaction, id int64) (bool, error) {
	if id == consts.DefaultTokenEcosystem {
		return false, nil
	}
	var frozen []int64
	if err := GetDB(dbTx).Model(&Ecosystem{}).Where("id = ?", id).Pluck("frozen", &frozen).Error; err != nil {
		return false, err
	}
	return len(frozen) > 0 && frozen[0] != 0, nil
}

// ControlMode is the control mode of the ecosystem: 1 its founder governs it, 2 the votings of its
// members do (DAO mode). An ecosystem that does not exist has 0.
func ControlMode(dbTx *DbTransaction, id int64) (int64, error) {
	return cached(dbTx, ecosysTable, "controlmode:"+strconv.FormatInt(id, 10), func() (int64, error) {
		var mode []int64
		if err := GetDB(dbTx).Model(&Ecosystem{}).Where("id = ?", id).Pluck("control_mode", &mode).Error; err != nil {
			return 0, err
		}
		if len(mode) == 0 {
			return 0, nil
		}
		return mode[0], nil
	})
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
