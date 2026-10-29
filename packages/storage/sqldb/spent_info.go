/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"slices"

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

// blockOutputWrites splits the outputs a block holds into the ones it made, which it inserts,
// spent or not, and the earlier ones it spent, which it updates. The earlier outputs it read and
// left unspent are not written.
func blockOutputWrites(blockID int64, outputs []SpentInfo) (made, spent []SpentInfo) {
	for _, o := range outputs {
		switch {
		case o.BlockId == blockID:
			made = append(made, o)
		case o.SpentBlock == blockID:
			spent = append(spent, o)
		}
	}
	// the rows of a block go into the primary key in its order, so that they fill its pages one
	// after the other
	order := func(a, b SpentInfo) int {
		if c := bytes.Compare(a.OutputTxHash, b.OutputTxHash); c != 0 {
			return c
		}
		if c := cmp.Compare(a.OutputKeyId, b.OutputKeyId); c != 0 {
			return c
		}
		return cmp.Compare(a.OutputIndex, b.OutputIndex)
	}
	slices.SortFunc(made, order)
	slices.SortFunc(spent, func(a, b SpentInfo) int {
		if c := cmp.Compare(a.BlockId, b.BlockId); c != 0 {
			return c
		}
		return order(a, b)
	})
	return made, spent
}

// WriteBlockOutputs records the outputs the block made and the spending of earlier ones: an output
// it spends takes the input that spends it. An output is unspent while its spent_block is 0 (the
// check spent_info_spent), so the update reads each output it spends by the primary key alone.
func WriteBlockOutputs(dbTx *gorm.DB, blockID int64, outputs []SpentInfo) error {
	made, spent := blockOutputWrites(blockID, outputs)
	if len(made) > 0 {
		if err := dbTx.Exec(`INSERT INTO "spent_info" ("input_tx_hash", "input_index", "output_tx_hash",
				"output_index", "output_key_id", "output_value", "ecosystem", "block_id", "type", "spent_block")
			SELECT i, ii, o, oi, k, v::numeric, e, b, t, s
			FROM unnest(?::bytea[], ?::integer[], ?::bytea[], ?::integer[], ?::bigint[], ?::text[], ?::bigint[],
				?::bigint[], ?::bigint[], ?::bigint[]) AS u(i, ii, o, oi, k, v, e, b, t, s)`,
			column(made, func(r SpentInfo) []byte { return r.InputTxHash }),
			column(made, func(r SpentInfo) int32 { return r.InputIndex }),
			column(made, func(r SpentInfo) []byte { return r.OutputTxHash }),
			column(made, func(r SpentInfo) int32 { return r.OutputIndex }),
			column(made, func(r SpentInfo) int64 { return r.OutputKeyId }),
			column(made, func(r SpentInfo) string { return r.OutputValue }),
			column(made, func(r SpentInfo) int64 { return r.Ecosystem }),
			column(made, func(r SpentInfo) int64 { return r.BlockId }),
			column(made, func(r SpentInfo) int64 { return int64(r.Type) }),
			column(made, func(r SpentInfo) int64 { return r.SpentBlock })).Error; err != nil {
			return fmt.Errorf("inserting the outputs of block %d: %w", blockID, err)
		}
	}
	if len(spent) == 0 {
		return nil
	}
	res := dbTx.Exec(`UPDATE "spent_info" SET "input_tx_hash" = u.i, "input_index" = u.ii, "spent_block" = u.s
		FROM unnest(?::bigint[], ?::bytea[], ?::bigint[], ?::integer[], ?::bytea[], ?::integer[], ?::bigint[])
			AS u(b, o, k, oi, i, ii, s)
		WHERE "block_id" = u.b AND "output_tx_hash" = u.o AND "output_key_id" = u.k AND "output_index" = u.oi
			AND "spent_block" = 0`,
		column(spent, func(r SpentInfo) int64 { return r.BlockId }),
		column(spent, func(r SpentInfo) []byte { return r.OutputTxHash }),
		column(spent, func(r SpentInfo) int64 { return r.OutputKeyId }),
		column(spent, func(r SpentInfo) int32 { return r.OutputIndex }),
		column(spent, func(r SpentInfo) []byte { return r.InputTxHash }),
		column(spent, func(r SpentInfo) int32 { return r.InputIndex }),
		column(spent, func(r SpentInfo) int64 { return r.SpentBlock }))
	if res.Error != nil {
		return fmt.Errorf("spending the outputs of block %d: %w", blockID, res.Error)
	}
	if res.RowsAffected != int64(len(spent)) {
		return fmt.Errorf("block %d spends %d outputs, %d of them are unspent in spent_info", blockID,
			len(spent), res.RowsAffected)
	}
	return nil
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
// made are deleted. spent_block > 0 lets the generic plan of the statement read the spent outputs
// by their partial index.
func RollbackOutputs(blockID int64, db *DbTransaction, logger *log.Entry) error {
	err := GetDB(db).Exec(`UPDATE spent_info SET input_tx_hash = null, input_index = 0, spent_block = 0 WHERE spent_block = ? AND spent_block > 0`, blockID).Error
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
