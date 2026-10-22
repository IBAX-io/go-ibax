/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// SpentInfo is model
type SpentInfo struct {
	InputTxHash  []byte `gorm:"default:(-)"`
	InputIndex   int32
	OutputTxHash []byte `gorm:"not null"`
	OutputIndex  int32  `gorm:"not null"`
	OutputKeyId  int64  `gorm:"not null"`
	OutputValue  string `gorm:"not null"`
	Ecosystem    int64
	BlockId      int64
	Type         int32
	// SpentBlock is the block that spent the output, 0 while it is unspent
	SpentBlock int64
}

type KeyUTXO struct {
	Ecosystem int64
	//At        string
	KeyId int64
	// Asset        string
}

func (k *KeyUTXO) String() string {
	return fmt.Sprintf("%d%s%d", k.Ecosystem, "@", k.KeyId)
}

// TableName returns name of table
func (si *SpentInfo) TableName() string {
	return "spent_info"
}

// CreateSpentInfoBatches records the outputs of a block and the spending of earlier ones: an
// output it spends takes the input that spends it
func CreateSpentInfoBatches(dbTx *gorm.DB, spentInfos []SpentInfo) error {
	if len(spentInfos) == 0 {
		return nil
	}
	return dbTx.Exec(`INSERT INTO "spent_info" ("input_tx_hash", "input_index", "output_tx_hash", "output_index",
			"output_key_id", "output_value", "ecosystem", "block_id", "type", "spent_block")
		SELECT i, ii, o, oi, k, v::numeric, e, b, t, s
		FROM unnest(?::bytea[], ?::integer[], ?::bytea[], ?::integer[], ?::bigint[], ?::text[], ?::bigint[], ?::bigint[],
			?::bigint[], ?::bigint[]) AS u(i, ii, o, oi, k, v, e, b, t, s)
		ON CONFLICT ("output_tx_hash", "output_key_id", "output_index") DO UPDATE SET
			"input_tx_hash" = "excluded"."input_tx_hash", "input_index" = "excluded"."input_index",
			"spent_block" = "excluded"."spent_block"`,
		column(spentInfos, func(r SpentInfo) []byte { return r.InputTxHash }),
		column(spentInfos, func(r SpentInfo) int32 { return r.InputIndex }),
		column(spentInfos, func(r SpentInfo) []byte { return r.OutputTxHash }),
		column(spentInfos, func(r SpentInfo) int32 { return r.OutputIndex }),
		column(spentInfos, func(r SpentInfo) int64 { return r.OutputKeyId }),
		column(spentInfos, func(r SpentInfo) string { return r.OutputValue }),
		column(spentInfos, func(r SpentInfo) int64 { return r.Ecosystem }),
		column(spentInfos, func(r SpentInfo) int64 { return r.BlockId }),
		column(spentInfos, func(r SpentInfo) int64 { return int64(r.Type) }),
		column(spentInfos, func(r SpentInfo) int64 { return r.SpentBlock })).Error
}

func GetTxOutputsEcosystem(db *DbTransaction, ecosystem int64, keyIds []int64) ([]SpentInfo, error) {
	query :=
		` SELECT si.output_tx_hash, si.output_index, si.output_key_id, si.output_value, si.ecosystem, si.block_id
		FROM spent_info si
		WHERE si.ecosystem = ? AND si.output_key_id IN ? AND si.input_tx_hash IS NULL
		ORDER BY si.output_key_id, si.block_id, si.output_tx_hash, si.output_index `
	var result []SpentInfo
	err := GetDB(db).Raw(query, ecosystem, keyIds).Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}

func GetTxOutputs(db *DbTransaction, keyIds []int64) ([]SpentInfo, error) {
	query :=
		` SELECT si.output_tx_hash, si.output_index, si.output_key_id, si.output_value, si.ecosystem, si.block_id
		FROM spent_info si
		WHERE si.output_key_id IN ? AND si.input_tx_hash IS NULL
		ORDER BY si.output_key_id, si.block_id, si.output_tx_hash, si.output_index `
	var result []SpentInfo
	err := GetDB(db).Raw(query, keyIds).Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RollbackOutputs undoes the outputs of the block: those it spent are unspent again, and those it
// made are deleted
func RollbackOutputs(blockID int64, db *DbTransaction, logger *log.Entry) error {
	err := GetDB(db).Exec(`UPDATE spent_info SET input_tx_hash = null, input_index = 0, spent_block = 0 WHERE spent_block = ?`, blockID).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Errorf("updating input_tx_hash rollback outputs by blockID : %d", blockID)
		return err
	}

	err = GetDB(db).Exec(`DELETE FROM spent_info WHERE block_id = ? `, blockID).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Errorf("deleting rollback outputs by blockID : %d", blockID)
		return err
	}

	return nil
}

func GetBlockOutputs(dbTx *DbTransaction, blockID int64) ([]SpentInfo, error) {
	var result []SpentInfo
	err := GetDB(dbTx).Where("block_id = ?", blockID).Find(&result).Error
	return result, err
}

func (si *SpentInfo) GetBalance(db *DbTransaction, keyId, ecosystem int64) (decimal.Decimal, error) {
	var amount decimal.Decimal
	f, err := isFound(GetDB(db).Table(si.TableName()).Select("coalesce(sum(output_value),'0') amount").
		Where("input_tx_hash is NULL AND output_key_id = ? AND ecosystem = ?", keyId, ecosystem).Take(&amount))
	if err != nil {
		return decimal.Zero, err
	}
	if !f {
		return decimal.Zero, errors.New("doesn't not exist UTXO output_key_id")
	}

	return amount, err
}

// GetTopAmounts returns top amounts by ecosystem, rank and dense
func GetTopAmounts(db *DbTransaction, ecosystem int64, rank int64, dense bool) ([]any, error) {
	query := `
SELECT * FROM 
    (SELECT sum ( amount ) AS amount,
     id,
     CASE 
    	WHEN ? THEN dense_rank() OVER (ORDER BY sum ( amount )  DESC)
    	ELSE rank() OVER (ORDER BY sum ( amount )  DESC)
  	END AS rank
    FROM 
        (SELECT output_value AS amount,
     output_key_id AS id
        FROM spent_info
        LEFT JOIN "1_keys"
          ON spent_info.ecosystem = "1_keys".ecosystem
            AND id = spent_info.output_key_id
        WHERE input_tx_hash is null
            AND "spent_info".ecosystem = ?
            AND blocked = 0
            AND deleted = 0
            AND length ( pub ) > 0
UNION
SELECT amount, id  FROM "1_keys"
        WHERE ecosystem = ?
            AND blocked = 0
            AND deleted = 0
            AND amount > 0
            AND length ( pub ) > 0 ) tmp
        GROUP BY  id
        ORDER BY  rank ASC ) r
    WHERE rank <= ?
ORDER BY  rank, id;
`
	rows, err := GetDB(db).Raw(query, dense, ecosystem, ecosystem, rank).Rows()
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, "getting rows columns")
	}
	values := make([][]byte, len(cols))
	scanArgs := make([]any, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	result := make([]any, 0)
	for rows.Next() {
		err = rows.Scan(scanArgs...)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", err, "scanning next row")
		}
		row := types.NewMap()
		for i, col := range values {
			var value string
			if col != nil {
				value = string(col)
			}
			row.Set(cols[i], value)
		}
		result = append(result, reflect.ValueOf(row).Interface())
	}
	return result, nil
}
