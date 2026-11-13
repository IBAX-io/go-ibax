/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

// The statements of the contract functions as they were made before the shapes of B7 (perf/exec-cost
// f7f0d96a): a query builder for each statement, its text and the strings of its values built
// each time. The comparisons of b7_harness_test.go run them against the functions of the package.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	qb "github.com/IBAX-io/go-ibax/packages/storage/sqldb/queryBuilder"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb/querycost"
	"github.com/IBAX-io/go-ibax/packages/types"
	"gorm.io/gorm"
)

// b7OldWriteRowsCost is selectiveLoggingAndUpd of the old statements
func b7OldWriteRowsCost(sc *SmartContract, fields []string, ivalues []any, table string, inWhere *types.Map,
	generalRollback bool, exists bool) (int64, string, error) {
	cost, tableID, _, err := b7OldWriteRows(sc, fields, ivalues, table, inWhere, generalRollback, exists)
	return cost, tableID, err
}

func b7OldWriteRows(sc *SmartContract, fields []string, ivalues []any,
	table string, inWhere *types.Map, generalRollback bool, exists bool) (int64, string, *sqldb.OneRow, error) {

	var (
		cost    int64
		logData map[string]string
		rows    *sqldb.OneRow
	)
	logger := sc.GetLogger()
	if generalRollback && sc.BlockHeader == nil {
		logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("Block is undefined")
		return 0, ``, nil, fmt.Errorf(`it is impossible to write to DB when Block is undefined`)
	}
	for i, field := range fields {
		fields[i] = strings.ToLower(field)
	}
	sqlBuilder := &qb.SQLQueryBuilder{
		Entry:        logger,
		Table:        table,
		Fields:       fields,
		FieldValues:  ivalues,
		Where:        inWhere,
		TxEcoID:      sc.TxSmart.EcosystemID,
		KeyTableChkr: sqldb.KeyTableChecker{},
	}

	if err := setInterfaceRefs(sqlBuilder); err != nil {
		return 0, "", nil, err
	}

	queryCoster := querycost.GetQueryCoster(querycost.FormulaQueryCosterType)
	if exists {
		selectQuery, selectArgs, err := sqlBuilder.GetSelectExpr()
		if err != nil {
			logger.WithError(err).Error("on getting sql select statement")
			return 0, "", nil, err
		}

		selectCost, err := queryCoster.QueryCost(sc.DbTransaction, selectQuery, selectArgs...)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table, "query": selectQuery, "fields": fields, "values": ivalues, "where": inWhere}).Log(dbLevel(err), "getting query total cost")
			return 0, "", nil, err
		}
		rows = sc.DbTransaction.GetOneRowTransaction(selectQuery, selectArgs...)
		logData, err = rows.String()
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": selectQuery}).Log(dbLevel(err), "getting one row transaction")
			return 0, "", nil, err
		}
		cost += selectCost
		if len(logData) == 0 {
			logger.WithFields(log.Fields{"type": consts.NotFound, "err": errUpdNotExistRecord, "table": table, "fields": fields, "values": shortString(fmt.Sprintf("%+v", ivalues), 100), "where": inWhere, "query": shortString(selectQuery, 100)}).Error("updating for not existing record")
			return 0, "", nil, errUpdNotExistRecord
		}
		if sqlBuilder.IsEmptyWhere() {
			logger.WithFields(log.Fields{"type": consts.NotFound,
				"error": errWhereUpdate}).Error("update without where")
			return 0, "", nil, errWhereUpdate
		}
	}
	var rollDataHashStr string

	if !sqlBuilder.Where.IsEmpty() && len(logData) > 0 {
		updateExpr, updateArgs, err := sqlBuilder.GetSQLUpdateExpr(logData)
		if err != nil {
			logger.WithError(err).Error("on getting update expression for update")
			return 0, "", nil, err
		}

		whereExpr, whereArgs, err := sqlBuilder.GetSQLWhereExpr()
		if err != nil {
			logger.WithError(err).Error("on getting where expression for update")
			return 0, "", nil, err
		}
		args := append(updateArgs, whereArgs...)
		if !sc.ChildChain {
			updateQuery := `UPDATE "` + sqlBuilder.Table + `" SET ` + updateExpr + " " + whereExpr
			updateCost, err := queryCoster.QueryCost(sc.DbTransaction, updateQuery, args...)
			if err != nil {
				logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": updateQuery}).Log(dbLevel(err), "getting query total cost for update query")
				return 0, "", nil, err
			}
			cost += updateCost
		}
		rollDataHashStr = b7OldBoundStatement(`UPDATE "`+strings.Trim(sqlBuilder.Table, `"`)+`" SET `+updateExpr+" "+whereExpr, args)
		prevMaxID, maxIDCached := sc.DbTransaction.CachedMaxID(sqlBuilder.Table)
		err = sc.DbTransaction.Update(sqlBuilder.Table, updateExpr, whereExpr, args...)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": sqlBuilder.Table, "update": updateExpr, "where": whereExpr}).Log(dbLevel(err), "getting update query")
			return 0, "", nil, err
		}
		if maxIDCached && !setsID(fields) {
			sc.DbTransaction.KeepMaxID(sqlBuilder.Table, prevMaxID)
		}

		for i := 0; i < len(rows.List) && generalRollback; i++ {
			rollData := rows.List[i]
			rollbackInfoStr, err := b7OldRollBackInfoString(rollData)
			if err != nil {
				logger.WithError(err).Error("on generate rollback info string for update")
				return 0, "", nil, err
			}
			b7OldAddRollback(sc, sqlBuilder.Table, rollData["id"], rollbackInfoStr, rollDataHashStr)
		}

		var ids []string
		for _, result := range rows.List {
			if len(result["id"]) > 0 {
				ids = append(ids, result["id"])
			}
		}
		if len(ids) == 1 {
			sqlBuilder.SetTableID(ids[0])
		} else {
			sqlBuilder.SetTableID(strings.Join(ids, ","))
		}
		return cost, sqlBuilder.TableID(), rows, nil
	}

	insertQuery, insertArgs, err := sqlBuilder.GetSQLInsertQuery(sqldb.NextIDGetter{Tx: sc.DbTransaction})
	if err != nil {
		logger.WithError(err).Error("on build insert query")
		return 0, "", nil, err
	}

	insertCost, err := queryCoster.QueryCost(sc.DbTransaction, insertQuery, insertArgs...)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": insertQuery}).Log(dbLevel(err), "getting total query cost for insert query")
		return 0, "", nil, err
	}
	rollDataHashStr = b7OldBoundStatement(insertQuery, insertArgs)
	cost += insertCost
	prevMaxID, maxIDCached := sc.DbTransaction.CachedMaxID(sqlBuilder.Table)
	err = sc.DbTransaction.ExecSql(insertQuery, insertArgs...)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": insertQuery}).Log(dbLevel(err), "executing insert query")
		return 0, "", nil, err
	}
	if id, err := strconv.ParseInt(sqlBuilder.TableID(), 10, 64); maxIDCached && err == nil {
		sc.DbTransaction.KeepMaxID(sqlBuilder.Table, max(prevMaxID, id))
	}

	if generalRollback {
		var tid string
		tid = sqlBuilder.TableID()
		idNames := strings.SplitN(sqlBuilder.Table, `_`, 2)
		if len(idNames) == 2 {
			if sqlBuilder.KeyTableChkr.IsKeyTable(idNames[1]) {
				tid = sqlBuilder.TableID() + "," + sqlBuilder.GetEcosystem()
			}
		}
		b7OldAddRollback(sc, sqlBuilder.Table, tid, "", rollDataHashStr)
	}
	return cost, sqlBuilder.TableID(), nil, nil
}

func b7OldDBInsert(sc *SmartContract, tblname string, values *types.Map) (qcost int64, ret int64, err error) {
	tblname = qb.GetTableName(sc.TxSmart.EcosystemID, tblname)
	if tblname == platformParametersTable {
		return 0, 0, fmt.Errorf("platform parameters access denied")
	}
	if err = sc.AccessTable(tblname, "insert"); err != nil {
		return
	}
	var ind int
	var lastID string
	if ind, err = b7OldNumIndexes(sc.DbTransaction, tblname); err != nil {
		err = logErrorDB(err, "num indexes")
		return
	}
	params, val, err := mapToParams(values)
	if err != nil {
		return
	}
	if reflect.TypeOf(val[0]) == reflect.TypeOf([]any{}) {
		val = val[0].([]any)
	}
	qcost, lastID, err = b7OldWriteRowsCost(sc, params, val, tblname, nil, !sc.ChildChain && sc.Rollback, false)
	if ind > 0 {
		qcost *= int64(ind)
	}
	if err == nil {
		ret, _ = strconv.ParseInt(lastID, 10, 64)
	}
	return
}

func b7OldDBSelect(sc *SmartContract, tblname string, inColumns any, id int64, inOrder any,
	offset, limit int64, inWhere *types.Map, query any, group string, all bool) (int64, []any, error) {

	var (
		err     error
		rows    *sqldb.Rows
		perm    map[string]string
		columns []string
		order   string
	)
	columns, err = qb.GetColumns(inColumns)
	if err != nil {
		return 0, nil, err
	}
	tblname = qb.GetTableName(sc.TxSmart.EcosystemID, tblname)
	where, whereArgs, err := qb.GetWhere(tblname, inWhere)
	if err != nil {
		return 0, nil, err
	}
	if id != 0 {
		where, whereArgs = `id = ?`, []any{converter.Int64ToStr(id)}
		limit = 1
	}
	if limit == 0 {
		limit = 25
	}
	if limit < 0 || limit > consts.DBFindLimit {
		limit = consts.DBFindLimit
	}
	perm, err = sc.AccessTablePerm(tblname, `read`)
	if err != nil {
		return 0, nil, err
	}
	if err = sc.AccessColumns(tblname, &columns, false); err != nil {
		return 0, nil, err
	}
	sel := sqldb.Select{Table: tblname, Columns: PrepareColumns(columns), Where: where, Args: whereArgs,
		Offset: int(offset), Limit: int(limit)}

	//group + order => false
	//group + no order => true
	//no group + order => true
	//no group + no order => true
	order, err = qb.GetOrder(tblname, inOrder, true)
	if err != nil {
		return 0, nil, err
	}

	if len(group) != 0 {
		sel.Group = group
		if inOrder != nil {
			order, err = qb.GetOrder(tblname, inOrder, false)
			if err != nil {
				return 0, nil, err
			}
		}
	}
	sel.Order = order

	if q, ok := query.(string); ok && q != "" {
		sel.Columns = q
	}
	if all {
		sel.Offset, sel.Limit = 0, -1
	}
	rows, err = sc.DbTransaction.Query(&sel)
	if err != nil {
		return 0, nil, logErrorDB(err, fmt.Sprintf("selecting rows from table %s %s where %s order %s",
			tblname, PrepareColumns(columns), where, order))
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return 0, nil, logErrorDB(err, "getting rows columns")
	}
	values := make([][]byte, len(cols))
	scanArgs := make([]any, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	result := make([]any, 0, 50)
	for rows.Next() {
		err = rows.Scan(scanArgs...)
		if err != nil {
			return 0, nil, logErrorDB(err, "scanning next row")
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
	if perm != nil && len(perm[`filter`]) > 0 {
		fltResult, err := sc.VM.EvalIf(perm[`filter`], uint32(sc.TxSmart.EcosystemID),
			sc.getExtend(),
		)
		if err != nil {
			return 0, nil, err
		}
		if !fltResult {
			log.WithFields(log.Fields{"filter": perm["filter"]}).Error("Access denied")
			return 0, nil, errAccessDenied
		}
	}
	return 0, result, nil
}

func b7OldDBUpdateExt(sc *SmartContract, tblname string, where *types.Map,
	values *types.Map) (qcost int64, err error) {
	tblname = qb.GetTableName(sc.TxSmart.EcosystemID, tblname)
	if tblname == platformParametersTable {
		return 0, fmt.Errorf("platform parameters access denied")
	}
	if err = sc.AccessTable(tblname, "update"); err != nil {
		return
	}
	columns, val, err := mapToParams(values)
	if err != nil {
		return
	}
	if err = sc.AccessColumns(tblname, &columns, true); err != nil {
		return
	}
	qcost, _, err = b7OldWriteRowsCost(sc, columns, val, tblname, where, !sc.ChildChain && sc.Rollback, true)
	return
}

func b7OldUpdateKeyAmount(sc *SmartContract, sign string, sum decimal.Decimal, eco, id int64, where *types.Map) (decimal.Decimal, error) {
	_, _, prev, err := b7OldWriteRows(sc, []string{sign + "amount"}, []any{sum}, "1_keys", where, !sc.ChildChain && sc.Rollback, true)
	if err != nil {
		return decimal.Zero, err
	}
	if prev != nil && len(prev.List) == 1 && sum.Equal(sum.Truncate(0)) {
		if amount, err := decimal.NewFromString(prev.List[0]["amount"]); err == nil && amount.Equal(amount.Truncate(0)) {
			if sign == "-" {
				return amount.Sub(sum), nil
			}
			return amount.Add(sum), nil
		}
	}
	return sc.accountBalanceSingle(eco, id)
}

func b7OldPayTaxes(sc *SmartContract, pay *PaymentInfo, sum decimal.Decimal, t GasScenesType, comment string, status pbgo.TxInvokeStatusCode) error {
	if sum.IsZero() {
		return nil
	}
	var toID int64
	switch t {
	case GasScenesType_Reward, GasScenesType_Direct:
		toID = pay.ToID
	case GasScenesType_Combustion:
		toID = 0
	case GasScenesType_Taxes:
		toID = pay.TaxesID
	}
	var fromIDBalance decimal.Decimal
	var err error
	if pay.FromID != toID {
		if fromIDBalance, err = b7OldUpdateKeyAmount(sc, `-`, sum, pay.TokenEco, pay.FromID,
			types.LoadMap(map[string]any{
				`id`:        pay.FromID,
				`ecosystem`: pay.TokenEco,
			})); err != nil {
			return err
		}
		sc.FeeCredits = append(sc.FeeCredits, FeeCredit{Ecosystem: pay.TokenEco, Recipient: toID, Amount: sum})
	} else if fromIDBalance, err = sc.accountBalanceSingle(pay.TokenEco, pay.FromID); err != nil {
		return err
	}
	values := types.LoadMap(map[string]any{
		"sender_id":      pay.FromID,
		"sender_balance": fromIDBalance,
		"recipient_id":   toID,
		"amount":         sum,
		"comment":        comment,
		"status":         int64(status),
		"block_id":       sc.BlockHeader.BlockId,
		"txhash":         sc.Hash,
		"ecosystem":      pay.TokenEco,
		"type":           int64(t),
		"created_at":     sc.Timestamp,
	})
	if pay.FromID == toID {
		values.Set("recipient_balance", fromIDBalance)
	}
	if t == GasScenesType_Reward || t == GasScenesType_Direct {
		values.Set("value_detail", pay.Detail())
	}
	if t == GasScenesType_Combustion {
		values.Set("value_detail", pay.DetailCombustion())
	}
	_, _, err = b7OldWriteRowsCost(sc, values.Keys(), values.Values(), `1_history`, nil, !sc.ChildChain && sc.Rollback, false)
	return err
}

// b7OldIndexes are the numbers of indexes b7OldNumIndexes read, by transaction and table, as the
// cache of a transaction kept them
var b7OldIndexes = map[*sqldb.DbTransaction]map[string]int{}

// b7OldNumIndexes is NumIndexes as it read the number in pg_class once in each transaction
func b7OldNumIndexes(dbTx *sqldb.DbTransaction, tblname string) (int, error) {
	if n, ok := b7OldIndexes[dbTx][tblname]; ok {
		return n, nil
	}
	var indexes int64
	err := sqldb.GetDB(dbTx).Raw(fmt.Sprintf(`select count( i.relname) from pg_class t, pg_class i, pg_index ix, pg_attribute a
	 where t.oid = ix.indrelid and i.oid = ix.indexrelid and a.attrelid = t.oid and a.attnum = ANY(ix.indkey)
         and t.relkind = 'r' and t.relnamespace = current_schema()::regnamespace and t.relname = '%s'`, tblname)).Row().Scan(&indexes)
	if err != nil && err != gorm.ErrRecordNotFound {
		return 0, err
	}
	n := 0
	if err == nil {
		n = int(indexes - 1)
	}
	if b7OldIndexes[dbTx] == nil {
		b7OldIndexes[dbTx] = map[string]int{}
	}
	b7OldIndexes[dbTx][tblname] = n
	return n, nil
}

// b7OldAddRollback is addRollback as it was before the appenders
func b7OldAddRollback(sc *SmartContract, table, tableID, rollbackInfoStr, rollDataHashStr string) {
	rollbackTx := &types.RollbackTx{
		BlockId:   sc.BlockHeader.BlockId,
		TxHash:    sc.Hash,
		NameTable: table,
		TableId:   tableID,
		Data:      rollbackInfoStr,
		DataHash:  crypto.Hash([]byte(rollDataHashStr)),
	}
	sc.RollBackTx = append(sc.RollBackTx, rollbackTx)
}

// b7OldBoundStatement is qb.BoundStatement as it was before the appenders
func b7OldBoundStatement(query string, args []any) string {
	if len(args) == 0 {
		return query
	}
	return query + " " + fmt.Sprintf("%q", args)
}

// b7OldRollBackInfoString is SQLQueryBuilder.GenerateRollBackInfoString as it was before the
// appenders, for a prepared builder: there the table of a key table is 1_ and its name, which
// IsKeyTable holds for, so its clause of a key table which is not one never held. The queryBuilder
// tests compare the appender with the whole function, that clause too
func b7OldRollBackInfoString(logData map[string]string) (string, error) {
	rollbackInfo := make(map[string]string)
	for k, v := range logData {
		if k == `id` {
			continue
		}
		rollbackInfo[k] = v
	}
	jsonRollbackInfo, err := json.Marshal(rollbackInfo)
	if err != nil {
		return "", err
	}
	return string(jsonRollbackInfo), nil
}
