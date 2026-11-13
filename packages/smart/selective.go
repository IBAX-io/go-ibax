/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"fmt"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb/querycost"
	"github.com/IBAX-io/go-ibax/packages/types"

	qb "github.com/IBAX-io/go-ibax/packages/storage/sqldb/queryBuilder"

	log "github.com/sirupsen/logrus"
)

// addRollback adds the rollback record of a row a statement wrote, with the hash of the bound
// statement
func addRollback(sc *SmartContract, table, tableID, rollbackInfoStr string, dataHash []byte) {
	rollbackTx := &types.RollbackTx{
		BlockId:   sc.BlockHeader.BlockId,
		TxHash:    sc.Hash,
		NameTable: table,
		TableId:   tableID,
		Data:      rollbackInfoStr,
		DataHash:  dataHash,
	}
	sc.RollBackTx = append(sc.RollBackTx, rollbackTx)
}

// boundHash returns the hash of a statement bound to its values, which the rollback records of the
// rows it writes keep. The text is written into a buffer of the transaction used again
func (sc *SmartContract) boundHash(query string, args []any) []byte {
	sc.writeBuf = qb.AppendBoundStatement(sc.writeBuf[:0], query, args)
	return crypto.Hash(sc.writeBuf)
}

func (sc *SmartContract) selectiveLoggingAndUpd(fields []string, ivalues []any,
	table string, inWhere *types.Map, generalRollback bool, exists bool) (int64, string, error) {
	cost, tableID, _, err := sc.writeRows(fields, ivalues, table, inWhere, generalRollback, exists)
	return cost, tableID, err
}

// deleteRow deletes the row with the id from the table. The row, all its columns, is kept in a
// rollback of the system, which inserts it again.
func (sc *SmartContract) deleteRow(table string, id int64, generalRollback bool) (int64, error) {
	logger := sc.GetLogger()
	if generalRollback && sc.BlockHeader == nil {
		logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("Block is undefined")
		return 0, fmt.Errorf(`it is impossible to write to DB when Block is undefined`)
	}
	sqlBuilder := &qb.SQLQueryBuilder{
		Entry:        logger,
		Table:        table,
		Where:        types.LoadMap(map[string]any{"id": id}),
		TxEcoID:      sc.TxSmart.EcosystemID,
		KeyTableChkr: sqldb.KeyTableChecker{},
	}
	whereExpr, whereArgs, err := sqlBuilder.GetSQLWhereExpr()
	if err != nil {
		return 0, err
	}
	queryCoster := querycost.GetQueryCoster(querycost.FormulaQueryCosterType)
	selectQuery := `SELECT to_jsonb(t)::text FROM "` + sqlBuilder.Table + `" t ` + whereExpr
	selectCost, err := queryCoster.QueryCost(sc.DbTransaction, selectQuery, whereArgs...)
	if err != nil {
		return 0, logErrorDB(err, "getting query total cost")
	}
	row, err := sc.DbTransaction.Single(selectQuery, whereArgs...).String()
	if err != nil {
		return 0, logErrorDB(err, "selecting the row to delete")
	}
	if len(row) == 0 {
		return 0, errDelNotExistRecord
	}
	deleteQuery := `DELETE FROM "` + sqlBuilder.Table + `" ` + whereExpr
	deleteCost, err := queryCoster.QueryCost(sc.DbTransaction, deleteQuery, whereArgs...)
	if err != nil {
		return 0, logErrorDB(err, "getting query total cost for delete query")
	}
	if err = sc.DbTransaction.Delete(sqlBuilder.Table, whereExpr, whereArgs...); err != nil {
		return 0, logErrorDB(err, "deleting the row")
	}
	if generalRollback {
		if err = SysRollback(sc, SysRollData{Type: "DeleteRow", TableName: sqlBuilder.Table, Data: row}); err != nil {
			return 0, err
		}
	}
	return selectCost + deleteCost, nil
}

func (sc *SmartContract) insert(fields []string, ivalues []any,
	table string) (int64, string, error) {
	return sc.selectiveLoggingAndUpd(fields, ivalues, table, nil, !sc.ChildChain && sc.Rollback, false)
}

func (sc *SmartContract) updateWhere(fields []string, values []any,
	table string, where *types.Map) (int64, string, error) {
	return sc.selectiveLoggingAndUpd(fields, values, table, where, !sc.ChildChain && sc.Rollback, true)
}

func (sc *SmartContract) update(fields []string, values []any,
	table string, whereField string, whereValue any) (int64, string, error) {
	return sc.updateWhere(fields, values, table, types.LoadMap(map[string]any{
		whereField: fmt.Sprint(whereValue)}))
}

// setsID tells whether an update of the fields may change the id of a row: a field names a column,
// with the sign of an increment or the prefix of a timestamp
func setsID(fields []string) bool {
	for _, field := range fields {
		field = strings.TrimPrefix(field, "timestamp ")
		field = strings.TrimLeft(field, "+-")
		if strings.TrimSpace(field) == "id" {
			return true
		}
	}
	return false
}

func shortString(raw string, length int) string {
	if len(raw) > length {
		return raw[:length]
	}

	return raw
}

// dbLevel is the level of the log of the error of a statement: the statement after which the
// transaction runs again is the flow of the block, not a fault
func dbLevel(err error) log.Level {
	if sqldb.IsRerun(err) {
		return log.DebugLevel
	}
	return log.ErrorLevel
}
