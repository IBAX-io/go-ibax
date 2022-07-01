/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SpentInfo is model
type SpentInfo struct {
	InputTxHash  []byte `gorm:"default:(-)"`
	InputIndex   int32
	OutputTxHash []byte `gorm:"not null"`
	OutputIndex  int32  `gorm:"not null"`
	OutputKeyId  int64  `gorm:"not null"`
	OutputValue  string `gorm:"not null"`
	Scene        string
	Ecosystem    int64
	Contract     string
	BlockId      int64
	Asset        string
	Action       string `gorm:"-"` // UTXO operation control : change
}

// TableName returns name of table
func (si *SpentInfo) TableName() string {
	return "spent_info"
}

// CreateSpentInfoBatches is creating record of model
func CreateSpentInfoBatches(dbTx *gorm.DB, spentInfos []SpentInfo) error {
	//for _, info := range spentInfos {
	//	fmt.Println(hex.EncodeToString(info.InputTxHash), info.InputIndex, hex.EncodeToString(info.OutputTxHash), info.OutputIndex, info.OutputKeyId, info.OutputValue, info.BlockId)
	//}

	return dbTx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "output_tx_hash"}, {Name: "output_key_id"}, {Name: "output_index"}},
		DoUpdates: clause.AssignmentColumns([]string{"input_tx_hash", "input_index"}),
		Where: clause.Where{Exprs: []clause.Expression{
			clause.Eq{Column: "spent_info.output_tx_hash", Value: gorm.Expr(`"excluded"."output_tx_hash"`)},
			clause.Eq{Column: "spent_info.output_key_id", Value: gorm.Expr(`"excluded"."output_key_id"`)},
			clause.Eq{Column: "spent_info.output_index", Value: gorm.Expr(`"excluded"."output_index"`)},
		}},
	}).CreateInBatches(spentInfos, 1000).Error
}

func GetTxOutputsEcosystem(db *DbTransaction, ecosystem int64, keyIds []int64) ([]SpentInfo, error) {
	query :=
		` SELECT si.output_tx_hash, si.output_index, si.output_key_id, si.output_value, si.scene, si.ecosystem, si.contract, si.block_id, si.asset
		FROM spent_info si LEFT JOIN log_transactions AS tr ON si.output_tx_hash = tr.hash
		WHERE si.ecosystem = ? AND si.output_key_id IN ? AND  si.input_tx_hash IS NULL
		ORDER BY si.output_key_id, si.block_id ASC, tr.timestamp ASC `
	var result []SpentInfo
	err := GetDB(db).Raw(query, ecosystem, keyIds).Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}

func GetTxOutputs(db *DbTransaction, keyIds []int64) ([]SpentInfo, error) {
	query :=
		` SELECT si.output_tx_hash, si.output_index, si.output_key_id, si.output_value, si.scene, si.ecosystem, si.contract, si.block_id, si.asset
		FROM spent_info si LEFT JOIN log_transactions AS tr ON si.output_tx_hash = tr.hash
		WHERE si.output_key_id IN ? AND si.input_tx_hash IS NULL
		ORDER BY si.output_key_id, si.block_id ASC, tr.timestamp ASC `
	var result []SpentInfo
	err := GetDB(db).Raw(query, keyIds).Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}
