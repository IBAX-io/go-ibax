/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

// The shapes of the statements of the contract database functions. The text of a statement is
// made by the query builder once for each shape, which the walks of the query builder write the
// key of; a statement of a known shape is made of the text of its shape and of the values the
// walks write, and checks its values as the builder does. A statement whose shape is not known,
// or whose values the builder would refuse, is made by the builder, which makes its errors; the
// shape of a statement made by the builder is kept when the walks wrote the same values.
//
// What the text of a shape depends on of the schema is the data types of the columns, so the
// shapes are made again when syspar replaces them (syspar.ColTypeVersion).

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	qb "github.com/IBAX-io/go-ibax/packages/storage/sqldb/queryBuilder"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb/querycost"
	"github.com/IBAX-io/go-ibax/packages/types"

	log "github.com/sirupsen/logrus"
)

// maxShapes bounds the shapes kept; all of them are dropped when it is reached
const maxShapes = 4096

// shape is what is made of the shape of a statement
type shape struct {
	// text is the text of the statement: the select of the rows of an update, an insert, an
	// update, or a select of DBSelect; stmt runs it
	text string
	stmt *sqldb.Statement
	// costType and costTable are what querycost reads of the text
	costType  querycost.QueryType
	costTable string
	// meta are what the builder reads of the columns of the fields of a write, and whereTypes the
	// data types that check the values of the where map
	meta       []qb.FieldMeta
	whereTypes []string
	// jsonCols are the jsonb columns of the SET list of an update
	jsonCols []string
	// sel is the select of DBSelect without its values, columns its columns before the check of
	// their access, and query tells the select has the columns of a query
	sel     sqldb.Select
	columns []string
	query   bool
}

// shapeBuilds counts the shapes kept. shapeHits counts the statements made of a known shape, when
// shapeCheck is set.
var shapeBuilds, shapeHits atomic.Int64

// shapeCache keeps the shapes by their key, for the version of the data types of the columns they
// were made with
type shapeCache struct {
	mu     sync.RWMutex
	gen    uint64
	shapes map[string]*shape
}

var shapes = &shapeCache{shapes: make(map[string]*shape)}

// get returns the shape of the key, nil when it is not kept for the current data types
func (c *shapeCache) get(key []byte) *shape {
	gen := syspar.ColTypeVersion()
	c.mu.RLock()
	var s *shape
	if c.gen == gen {
		s = c.shapes[string(key)]
	}
	c.mu.RUnlock()
	return s
}

// put keeps the shape of the key, made with the data types of the version gen, read before the
// shape was made
func (c *shapeCache) put(gen uint64, key []byte, s *shape) {
	if gen != syspar.ColTypeVersion() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case gen < c.gen:
		return
	case gen > c.gen, len(c.shapes) >= maxShapes:
		c.shapes = make(map[string]*shape)
		c.gen = gen
	}
	c.shapes[string(key)] = s
	shapeBuilds.Add(1)
}

// len is the number of the shapes kept
func (c *shapeCache) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.shapes)
}

// shapeWalk is the scratch of the shape of a statement of a contract database function
type shapeWalk struct {
	write qb.WriteWalk
	// sel is the walk of a select of DBSelect
	sel qb.WhereWalk
	// key is the key of the text of an update
	key []byte
	// the values of the statements
	whereArgs, setArgs, args []any
}

// acquireWalk returns the walk of a contract database function, which releaseWalk returns
func (sc *SmartContract) acquireWalk() *shapeWalk {
	if sc.walkDepth == len(sc.walks) {
		sc.walks = append(sc.walks, &shapeWalk{})
	}
	sw := sc.walks[sc.walkDepth]
	sc.walkDepth++
	return sw
}

func (sc *SmartContract) releaseWalk() {
	sc.walkDepth--
}

// sameArgs tells the values bound to two statements are the same strings
func sameArgs(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, ok := a[i].(string)
		y, ok2 := b[i].(string)
		if !ok || !ok2 || x != y {
			return false
		}
	}
	return true
}

// writeCandidate is what the walks made of a write whose shape is not known: it is kept with the
// text the builder makes when the builder binds the same values
type writeCandidate struct {
	gen        uint64
	meta       []qb.FieldMeta
	whereTypes []string
	whereArgs  []any
	setArgs    []any
}

// writeRows is selectiveLoggingAndUpd, which tells too the rows an update changed as they were
// before it: their id and the columns of the fields, nil for an insert
func (sc *SmartContract) writeRows(fields []string, ivalues []any,
	table string, inWhere *types.Map, generalRollback bool, exists bool) (int64, string, *sqldb.OneRow, error) {
	logger := sc.GetLogger()
	if generalRollback && sc.BlockHeader == nil {
		logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("Block is undefined")
		return 0, ``, nil, fmt.Errorf(`it is impossible to write to DB when Block is undefined`)
	}
	for i, field := range fields {
		fields[i] = strings.ToLower(field)
	}
	sw := sc.acquireWalk()
	defer sc.releaseWalk()
	w := &sw.write
	gen := syspar.ColTypeVersion()
	if !w.Walk(sqldb.KeyTableChecker{}, table, fields, ivalues, inWhere, sc.TxSmart.EcosystemID, exists) ||
		w.IsKeyTable && interfaceTables[w.KeyName] || exists && !w.WalkWhere(inWhere) {
		return sc.writeBuilt(fields, ivalues, table, inWhere, generalRollback, exists, nil)
	}
	sh := shapes.get(w.Key)
	var (
		m          []qb.FieldMeta
		whereTypes []string
		ok         bool
	)
	if sh != nil {
		m = sh.meta
	} else {
		m = w.Meta()
	}
	if ok = w.Decode(m, ivalues); ok {
		if exists {
			if sh != nil {
				whereTypes = sh.whereTypes
			} else {
				whereTypes = qb.WhereTypes(w.Table, w.Values)
			}
			if ok = qb.CheckWhere(w.Values, whereTypes); ok {
				sw.setArgs, ok = w.SetArgs(m, sw.setArgs[:0])
			}
		} else {
			sw.setArgs, ok = w.InsertArgs(m, sw.setArgs[:0])
		}
	}
	switch {
	case !ok:
		return sc.writeBuilt(fields, ivalues, table, inWhere, generalRollback, exists, nil)
	case sh == nil:
		cand := &writeCandidate{gen: gen, meta: m, whereTypes: whereTypes, setArgs: sw.setArgs}
		if exists {
			cand.whereArgs = qb.AppendWhereArgs(nil, w.Values)
		}
		return sc.writeBuilt(fields, ivalues, table, inWhere, generalRollback, exists, cand)
	}
	if shapeCheck {
		shapeHits.Add(1)
	}
	if exists {
		return sc.updateShape(sw, sh, table, fields, ivalues, inWhere, generalRollback)
	}
	return sc.insertShape(sw, sh, table, fields, ivalues, generalRollback)
}

// updateShape runs an update of a known shape, whose values are checked
func (sc *SmartContract) updateShape(sw *shapeWalk, sh *shape, table string, fields []string, ivalues []any,
	inWhere *types.Map, generalRollback bool) (int64, string, *sqldb.OneRow, error) {
	logger := sc.GetLogger()
	w := &sw.write
	sw.whereArgs = qb.AppendWhereArgs(sw.whereArgs[:0], w.Values)
	selectCost, err := querycost.Formula.Cost(sc.DbTransaction, sh.costType, sh.costTable)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": w.Table, "query": sh.text, "fields": fields, "values": ivalues, "where": inWhere}).Log(dbLevel(err), "getting query total cost")
		return 0, "", nil, err
	}
	rows := sc.DbTransaction.OneRowStatement(sh.stmt, sw.whereArgs...)
	logData, err := rows.String()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": sh.text}).Log(dbLevel(err), "getting one row transaction")
		return 0, "", nil, err
	}
	cost := selectCost
	if len(logData) == 0 {
		logger.WithFields(log.Fields{"type": consts.NotFound, "err": errUpdNotExistRecord, "table": w.Table, "fields": fields, "values": shortString(fmt.Sprintf("%+v", ivalues), 100), "where": inWhere, "query": shortString(sh.text, 100)}).Error("updating for not existing record")
		return 0, "", nil, errUpdNotExistRecord
	}
	sw.args = append(append(sw.args[:0], sw.setArgs...), sw.whereArgs...)
	sw.key = append(append(sw.key[:0], 'U'), w.Key...)
	for _, col := range sh.jsonCols {
		if qb.HasJSONValue(logData, col) {
			sw.key = append(sw.key, 1)
		} else {
			sw.key = append(sw.key, 0)
		}
	}
	us := shapes.get(sw.key)
	if us == nil || shapeCheck {
		gen := syspar.ColTypeVersion()
		b, err := sc.buildUpdate(table, fields, ivalues, inWhere, logData)
		if err != nil {
			logger.WithError(err).Error("on getting update expression for update")
			return 0, "", nil, err
		}
		switch {
		case !sameArgs(b.args, sw.args) || b.selectText != sh.text || us != nil && us.text != b.text:
			// never: the statement of the builder is run, and its shape is not kept
			if shapeCheck {
				panic(fmt.Sprintf("shape of update %q %v is not the statement %q %v", usText(us), sw.args, b.text, b.args))
			}
			us = &shape{text: b.text, stmt: sqldb.NewStatement(b.text)}
			sw.args = append(sw.args[:0], b.args...)
			if us.costType, us.costTable, err = querycost.Parse(b.text); err != nil {
				return 0, "", nil, err
			}
		case us == nil:
			us = &shape{text: b.text, stmt: sqldb.NewStatement(b.text)}
			if us.costType, us.costTable, err = querycost.Parse(b.text); err != nil {
				return 0, "", nil, err
			}
			shapes.put(gen, sw.key, us)
		}
	}
	if !sc.ChildChain {
		updateCost, err := querycost.Formula.Cost(sc.DbTransaction, us.costType, us.costTable)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": us.text}).Log(dbLevel(err), "getting query total cost for update query")
			return 0, "", nil, err
		}
		cost += updateCost
	}
	var dataHash []byte
	if generalRollback && len(rows.List) > 0 {
		dataHash = sc.boundHash(us.text, sw.args)
	}
	prevMaxID, maxIDCached := sc.DbTransaction.CachedMaxID(w.Table)
	if err = sc.DbTransaction.ExecStatement(us.stmt, sw.args...); err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": w.Table, "query": us.text}).Log(dbLevel(err), "getting update query")
		return 0, "", nil, err
	}
	if maxIDCached && !setsID(fields) {
		sc.DbTransaction.KeepMaxID(w.Table, prevMaxID)
	}
	for i := 0; i < len(rows.List) && generalRollback; i++ {
		rollData := rows.List[i]
		sc.writeBuf, sc.writeKeys = qb.AppendRollBackData(sc.writeBuf[:0], sc.writeKeys, sqldb.KeyTableChecker{},
			w.Table, w.IsKeyTable, rollData)
		addRollback(sc, w.Table, rollData["id"], string(sc.writeBuf), dataHash)
	}
	return cost, rowIDs(rows), rows, nil
}

func usText(s *shape) string {
	if s == nil {
		return ""
	}
	return s.text
}

// rowIDs is the id of the rows an update changed, the ids joined with commas when they are more
func rowIDs(rows *sqldb.OneRow) string {
	var ids []string
	for _, result := range rows.List {
		if len(result["id"]) > 0 {
			ids = append(ids, result["id"])
		}
	}
	if len(ids) == 1 {
		return ids[0]
	}
	return strings.Join(ids, ",")
}

// insertShape runs an insert of a known shape, whose values are checked
func (sc *SmartContract) insertShape(sw *shapeWalk, sh *shape, table string, fields []string, ivalues []any,
	generalRollback bool) (int64, string, *sqldb.OneRow, error) {
	logger := sc.GetLogger()
	w := &sw.write
	sw.args = append(sw.args[:0], sw.setArgs...)
	tableID := w.TableID
	if !w.HasID {
		id, err := sc.DbTransaction.GetNextID(w.Table)
		if err != nil {
			logger.WithError(err).Error("on build insert query")
			return 0, "", nil, err
		}
		tableID = converter.Int64ToStr(id)
		sw.args = append(sw.args, tableID)
	}
	if shapeCheck {
		b, err := sc.buildInsert(table, fields, ivalues)
		if err != nil || b.text != sh.text || !sameArgs(b.args[:len(sw.setArgs)], sw.setArgs) ||
			len(b.args) != len(sw.args) || w.HasID && b.tableID != tableID {
			panic(fmt.Sprintf("shape of insert %q %v is not the statement %q %v: %v", b.text, b.args, sh.text, sw.args, err))
		}
	}
	insertCost, err := querycost.Formula.Cost(sc.DbTransaction, sh.costType, sh.costTable)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": sh.text}).Log(dbLevel(err), "getting total query cost for insert query")
		return 0, "", nil, err
	}
	var dataHash []byte
	if generalRollback {
		dataHash = sc.boundHash(sh.text, sw.args)
	}
	prevMaxID, maxIDCached := sc.DbTransaction.CachedMaxID(w.Table)
	if err = sc.DbTransaction.ExecStatement(sh.stmt, sw.args...); err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": sh.text}).Log(dbLevel(err), "executing insert query")
		return 0, "", nil, err
	}
	keepInsertMaxID(sc, w.Table, tableID, prevMaxID, maxIDCached)
	if generalRollback {
		tid := tableID
		if w.IsKeyTable {
			tid = tableID + "," + w.KeyEcosystem
		}
		addRollback(sc, w.Table, tid, "", dataHash)
	}
	return insertCost, tableID, nil, nil
}

// built is a statement the builder made
type built struct {
	selectText, text string
	args             []any
	tableID          string
}

// newBuilder is the builder of a statement of writeRows on copies of the fields and the values
func (sc *SmartContract) newBuilder(table string, fields []string, ivalues []any, inWhere *types.Map) *qb.SQLQueryBuilder {
	return &qb.SQLQueryBuilder{
		Entry:        sc.GetLogger(),
		Table:        table,
		Fields:       slices.Clone(fields),
		FieldValues:  slices.Clone(ivalues),
		Where:        inWhere,
		TxEcoID:      sc.TxSmart.EcosystemID,
		KeyTableChkr: sqldb.KeyTableChecker{},
	}
}

// buildUpdate makes with the builder the select of the rows of an update and the update of the
// rows, with the values of the rows before it
func (sc *SmartContract) buildUpdate(table string, fields []string, ivalues []any, inWhere *types.Map,
	logData map[string]string) (built, error) {
	var r built
	b := sc.newBuilder(table, fields, ivalues, inWhere)
	selectText, _, err := b.GetSelectExpr()
	if err != nil {
		return r, err
	}
	updateExpr, updateArgs, err := b.GetSQLUpdateExpr(logData)
	if err != nil {
		return r, err
	}
	whereExpr, whereArgs, err := b.GetSQLWhereExpr()
	if err != nil {
		return r, err
	}
	r.selectText = selectText
	r.text = `UPDATE "` + b.Table + `" SET ` + updateExpr + " " + whereExpr
	r.args = append(updateArgs, whereArgs...)
	return r, nil
}

// zeroID is the id getter of the inserts the builder makes to be compared: their id is 0
type zeroID struct{}

func (zeroID) GetNextID(string) (int64, error) { return 0, nil }

// buildInsert makes with the builder an insert, whose id is 0 when no field gives it
func (sc *SmartContract) buildInsert(table string, fields []string, ivalues []any) (built, error) {
	var r built
	b := sc.newBuilder(table, fields, ivalues, nil)
	text, args, err := b.GetSQLInsertQuery(zeroID{})
	if err != nil {
		return r, err
	}
	r.text, r.args, r.tableID = text, args, b.TableID()
	return r, nil
}

// keepInsertMaxID keeps the highest id of the table the transaction knows after an insert of the
// id
func keepInsertMaxID(sc *SmartContract, table, tableID string, prevMaxID int64, maxIDCached bool) {
	if id, err := strconv.ParseInt(tableID, 10, 64); maxIDCached && err == nil {
		sc.DbTransaction.KeepMaxID(table, max(prevMaxID, id))
	}
}

// writeBuilt is writeRows of a statement the builder makes. The shape of the statement is kept
// when the walks made the candidate of the same values.
func (sc *SmartContract) writeBuilt(fields []string, ivalues []any, table string, inWhere *types.Map,
	generalRollback bool, exists bool, cand *writeCandidate) (int64, string, *sqldb.OneRow, error) {
	var (
		cost    int64
		logData map[string]string
		rows    *sqldb.OneRow
	)
	logger := sc.GetLogger()
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
	var (
		selectQuery string
		selectArgs  []any
		selectType  querycost.QueryType
		selectTable string
	)
	if exists {
		var err error
		selectQuery, selectArgs, err = sqlBuilder.GetSelectExpr()
		if err != nil {
			logger.WithError(err).Error("on getting sql select statement")
			return 0, "", nil, err
		}
		selectCost, err := func() (int64, error) {
			var err error
			if selectType, selectTable, err = querycost.Parse(selectQuery); err != nil {
				return 0, err
			}
			return querycost.Formula.Cost(sc.DbTransaction, selectType, selectTable)
		}()
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
	var dataHash []byte

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
		updateQuery := `UPDATE "` + sqlBuilder.Table + `" SET ` + updateExpr + " " + whereExpr
		if cand != nil {
			sc.keepUpdateShape(cand, sqlBuilder, logData, selectQuery, selectArgs, selectType, selectTable,
				updateQuery, updateArgs, whereExpr)
		}
		args := append(updateArgs, whereArgs...)
		if !sc.ChildChain {
			updateCost, err := querycost.Formula.QueryCost(sc.DbTransaction, updateQuery, args...)
			if err != nil {
				logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": updateQuery}).Log(dbLevel(err), "getting query total cost for update query")
				return 0, "", nil, err
			}
			cost += updateCost
		}
		if generalRollback && len(rows.List) > 0 {
			dataHash = sc.boundHash(updateQuery, args)
		}
		prevMaxID, maxIDCached := sc.DbTransaction.CachedMaxID(sqlBuilder.Table)
		err = sc.DbTransaction.ExecSql(updateQuery, args...)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": sqlBuilder.Table, "update": updateExpr, "where": whereExpr}).Log(dbLevel(err), "getting update query")
			return 0, "", nil, err
		}
		if maxIDCached && !setsID(fields) {
			sc.DbTransaction.KeepMaxID(sqlBuilder.Table, prevMaxID)
		}

		for i := 0; i < len(rows.List) && generalRollback; i++ {
			rollData := rows.List[i]
			sc.writeBuf, sc.writeKeys = sqlBuilder.AppendRollBackInfo(sc.writeBuf[:0], sc.writeKeys, rollData)
			addRollback(sc, sqlBuilder.Table, rollData["id"], string(sc.writeBuf), dataHash)
		}
		sqlBuilder.SetTableID(rowIDs(rows))
		return cost, sqlBuilder.TableID(), rows, nil
	}

	insertQuery, insertArgs, err := sqlBuilder.GetSQLInsertQuery(sqldb.NextIDGetter{Tx: sc.DbTransaction})
	if err != nil {
		logger.WithError(err).Error("on build insert query")
		return 0, "", nil, err
	}
	insertType, insertTable, err := querycost.Parse(insertQuery)
	var insertCost int64
	if err == nil {
		insertCost, err = querycost.Formula.Cost(sc.DbTransaction, insertType, insertTable)
	}
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": insertQuery}).Log(dbLevel(err), "getting total query cost for insert query")
		return 0, "", nil, err
	}
	if cand != nil && !exists {
		sc.keepInsertShape(cand, insertQuery, insertArgs, insertType, insertTable)
	}
	if generalRollback {
		dataHash = sc.boundHash(insertQuery, insertArgs)
	}
	cost += insertCost
	prevMaxID, maxIDCached := sc.DbTransaction.CachedMaxID(sqlBuilder.Table)
	err = sc.DbTransaction.ExecSql(insertQuery, insertArgs...)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "query": insertQuery}).Log(dbLevel(err), "executing insert query")
		return 0, "", nil, err
	}
	keepInsertMaxID(sc, sqlBuilder.Table, sqlBuilder.TableID(), prevMaxID, maxIDCached)

	if generalRollback {
		var tid string
		tid = sqlBuilder.TableID()
		idNames := strings.SplitN(sqlBuilder.Table, `_`, 2)
		if len(idNames) == 2 {
			if sqlBuilder.KeyTableChkr.IsKeyTable(idNames[1]) {
				tid = sqlBuilder.TableID() + "," + sqlBuilder.GetEcosystem()
			}
		}
		addRollback(sc, sqlBuilder.Table, tid, "", dataHash)
	}
	return cost, sqlBuilder.TableID(), nil, nil
}

// keepUpdateShape keeps the shape of an update the builder made, and the shape of its text with
// the values of the row before it, when the walks made the same values
func (sc *SmartContract) keepUpdateShape(cand *writeCandidate, b *qb.SQLQueryBuilder, logData map[string]string,
	selectQuery string, selectArgs []any, selectType querycost.QueryType, selectTable string,
	updateQuery string, updateArgs []any, whereExpr string) {
	sw := sc.walks[sc.walkDepth-1]
	w := &sw.write
	if len(whereExpr) == 0 || !sameArgs(selectArgs, cand.whereArgs) || !sameArgs(updateArgs, cand.setArgs) {
		if shapeCheck {
			panic(fmt.Sprintf("walk of update %q %v %q %v is not the builder: %v %v", selectQuery, selectArgs,
				updateQuery, updateArgs, cand.whereArgs, cand.setArgs))
		}
		return
	}
	updateType, updateTable, err := querycost.Parse(updateQuery)
	if err != nil {
		return
	}
	sh := &shape{text: selectQuery, stmt: sqldb.NewStatement(selectQuery), costType: selectType, costTable: selectTable, meta: cand.meta,
		whereTypes: cand.whereTypes, jsonCols: w.JSONColumns()}
	sw.key = append(append(sw.key[:0], 'U'), w.Key...)
	for _, col := range sh.jsonCols {
		if qb.HasJSONValue(logData, col) {
			sw.key = append(sw.key, 1)
		} else {
			sw.key = append(sw.key, 0)
		}
	}
	shapes.put(cand.gen, sw.key, &shape{text: updateQuery, stmt: sqldb.NewStatement(updateQuery), costType: updateType, costTable: updateTable})
	shapes.put(cand.gen, w.Key, sh)
}

// keepInsertShape keeps the shape of an insert the builder made when the walks made the same
// values
func (sc *SmartContract) keepInsertShape(cand *writeCandidate, insertQuery string, insertArgs []any,
	insertType querycost.QueryType, insertTable string) {
	w := &sc.walks[sc.walkDepth-1].write
	args := insertArgs
	if !w.HasID && len(args) > 0 {
		args = args[:len(args)-1]
	}
	if !sameArgs(args, cand.setArgs) {
		if shapeCheck {
			panic(fmt.Sprintf("walk of insert %q %v is not the builder: %v", insertQuery, insertArgs, cand.setArgs))
		}
		return
	}
	shapes.put(cand.gen, w.Key, &shape{text: insertQuery, stmt: sqldb.NewStatement(insertQuery), costType: insertType, costTable: insertTable,
		meta: cand.meta})
}

// selectKey writes the key of the shape of a select of DBSelect, of the table its name resolved,
// and the values of its where map. It tells false for a select the walk does not make.
func (sw *shapeWalk) selectKey(tblname string, inColumns any, id int64, inOrder any, offset, limit int64,
	inWhere *types.Map, query any, group string, all bool) bool {
	w := &sw.sel
	w.Key, w.Values = append(w.Key[:0], 'S'), w.Values[:0]
	w.Key = appendKeyString(w.Key, tblname)
	switch v := inColumns.(type) {
	case string:
		w.Key = appendKeyString(append(w.Key, 's'), v)
	case []any:
		w.Key = binary.AppendUvarint(append(w.Key, 'a'), uint64(len(v)))
		for _, name := range v {
			if col, ok := name.(string); ok {
				w.Key = appendKeyString(append(w.Key, 's'), col)
			} else {
				w.Key = append(w.Key, '_')
			}
		}
	default:
		w.Key = append(w.Key, 'o')
	}
	if id != 0 {
		w.Key = append(w.Key, 'i')
		limit = 1
	} else {
		w.Key = append(w.Key, '0')
	}
	if limit == 0 {
		limit = 25
	}
	if limit < 0 || limit > consts.DBFindLimit {
		limit = consts.DBFindLimit
	}
	off, lim := int(offset), int(limit)
	if all {
		off, lim = 0, -1
	}
	w.Key = binary.AppendVarint(w.Key, int64(off))
	w.Key = binary.AppendVarint(w.Key, int64(lim))
	if !sw.orderKey(inOrder) {
		return false
	}
	w.Key = appendKeyString(w.Key, group)
	if q, ok := query.(string); ok && q != "" {
		w.Key = appendKeyString(append(w.Key, 'q'), q)
	} else {
		w.Key = append(w.Key, 'n')
	}
	return w.Where(inWhere)
}

// orderKey writes the key of the order of a select, false for an order of a map of Go, whose
// order is not that of the select
func (sw *shapeWalk) orderKey(inOrder any) bool {
	w := &sw.sel
	orderMap := func(m *types.Map) {
		w.Key = binary.AppendUvarint(append(w.Key, 'm'), uint64(m.Size()))
		for link := m.First(); link != nil; link = link.Next() {
			w.Key = appendKeyString(w.Key, link.Key())
			if qb.Sprint(link.Value()) == `-1` {
				w.Key = append(w.Key, 'd')
			} else {
				w.Key = append(w.Key, 'a')
			}
		}
	}
	switch v := inOrder.(type) {
	case nil:
		w.Key = append(w.Key, 'n')
	case string:
		w.Key = appendKeyString(append(w.Key, 's'), v)
	case *types.Map:
		orderMap(v)
	case map[string]any:
		return false
	case []any:
		w.Key = binary.AppendUvarint(append(w.Key, 'l'), uint64(len(v)))
		for _, item := range v {
			switch param := item.(type) {
			case string:
				w.Key = appendKeyString(append(w.Key, 's'), param)
			case *types.Map:
				orderMap(param)
			case map[string]any:
				return false
			default:
				w.Key = append(w.Key, '_')
			}
		}
	default:
		w.Key = append(w.Key, 'o')
	}
	return true
}

// appendKeyString appends a string to the key of a shape, with its length before it
func appendKeyString(key []byte, s string) []byte {
	key = binary.AppendUvarint(key, uint64(len(s)))
	return append(key, s...)
}

// selectBuild is the select of DBSelect the builder makes, with its columns before the check of
// their access
type selectBuild struct {
	sel     sqldb.Select
	columns []string
	query   bool
}

// buildSelect makes the select of DBSelect of the table, its name resolved. early is the error
// DBSelect returns before it checks the access to the table, late the error it returns after.
func buildSelect(tblname string, inColumns any, id int64, inOrder any, offset, limit int64,
	inWhere *types.Map, query any, group string, all bool) (b selectBuild, early, late error) {
	columns, err := qb.GetColumns(inColumns)
	if err != nil {
		return b, err, nil
	}
	where, whereArgs, err := qb.GetWhere(tblname, inWhere)
	if err != nil {
		return b, err, nil
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
	b.columns = columns
	b.sel = sqldb.Select{Table: tblname, Columns: PrepareColumns(columns), Where: where, Args: whereArgs,
		Offset: int(offset), Limit: int(limit)}
	//group + order => false
	//group + no order => true
	//no group + order => true
	//no group + no order => true
	order, err := qb.GetOrder(tblname, inOrder, true)
	if err != nil {
		return b, nil, err
	}
	if len(group) != 0 {
		b.sel.Group = group
		if inOrder != nil {
			if order, err = qb.GetOrder(tblname, inOrder, false); err != nil {
				return b, nil, err
			}
		}
	}
	b.sel.Order = order
	if q, ok := query.(string); ok && q != "" {
		b.sel.Columns = q
		b.query = true
	}
	if all {
		b.sel.Offset, b.sel.Limit = 0, -1
	}
	return b, nil, nil
}

// sameSlice tells two slices are the same: the check of the access to the columns of a select
// replaces the columns only when it drops some
func sameSlice(a, b []string) bool {
	return len(a) == len(b) && unsafe.SliceData(a) == unsafe.SliceData(b)
}

// DBSelect returns an array of values of the specified columns when there is selection of data 'offset', 'limit', 'where'
func DBSelect(sc *SmartContract, tblname string, inColumns any, id int64, inOrder any,
	offset, limit int64, inWhere *types.Map, query any, group string, all bool) (int64, []any, error) {
	tblname = qb.GetTableName(sc.TxSmart.EcosystemID, tblname)
	sw := sc.acquireWalk()
	defer sc.releaseWalk()
	gen := syspar.ColTypeVersion()
	keyed := sw.selectKey(tblname, inColumns, id, inOrder, offset, limit, inWhere, query, group, all)
	if keyed {
		if sh := shapes.get(sw.sel.Key); sh != nil && qb.CheckWhere(sw.sel.Values, sh.whereTypes) {
			if id != 0 {
				sw.args = append(sw.args[:0], converter.Int64ToStr(id))
			} else {
				sw.args = qb.AppendWhereArgs(sw.args[:0], sw.sel.Values)
			}
			return sc.selectShape(sw, sh, tblname, inColumns, id, inOrder, offset, limit, inWhere, query, group, all)
		}
	}
	b, early, late := buildSelect(tblname, inColumns, id, inOrder, offset, limit, inWhere, query, group, all)
	if early != nil {
		return 0, nil, early
	}
	perm, err := sc.AccessTablePerm(tblname, `read`)
	if err != nil {
		return 0, nil, err
	}
	columns := b.columns
	if err = sc.AccessColumns(tblname, &columns, false); err != nil {
		return 0, nil, err
	}
	if late != nil {
		return 0, nil, late
	}
	sel := b.sel
	if !b.query {
		sel.Columns = PrepareColumns(columns)
	}
	text := sel.SQL()
	if keyed {
		sc.keepSelectShape(gen, sw, b, sel, text, columns, id)
	}
	rows, err := sc.DbTransaction.QueryText(text, sel.Args)
	if err != nil {
		return 0, nil, logErrorDB(err, fmt.Sprintf("selecting rows from table %s %s where %s order %s",
			tblname, PrepareColumns(columns), sel.Where, sel.Order))
	}
	return sc.readSelect(rows, perm)
}

// keepSelectShape keeps the shape of a select the builder made when the walk made the same values
func (sc *SmartContract) keepSelectShape(gen uint64, sw *shapeWalk, b selectBuild, sel sqldb.Select, text string,
	columns []string, id int64) {
	whereTypes := qb.WhereTypes(b.sel.Table, sw.sel.Values)
	if !qb.CheckWhere(sw.sel.Values, whereTypes) ||
		id == 0 && !sameArgs(qb.AppendWhereArgs(nil, sw.sel.Values), b.sel.Args) {
		if shapeCheck {
			panic(fmt.Sprintf("walk of select %q %v is not the builder: %v", text, b.sel.Args, sw.sel.Values))
		}
		return
	}
	if !sameSlice(columns, b.columns) {
		sel = b.sel
		text = sel.SQL()
	}
	sh := &shape{text: text, stmt: sqldb.NewStatement(text), whereTypes: whereTypes, sel: b.sel, columns: b.columns, query: b.query}
	sh.sel.Args = nil
	shapes.put(gen, sw.sel.Key, sh)
}

// selectShape runs a select of DBSelect of a known shape, whose values are checked
func (sc *SmartContract) selectShape(sw *shapeWalk, sh *shape, tblname string, inColumns any, id int64,
	inOrder any, offset, limit int64, inWhere *types.Map, query any, group string, all bool) (int64, []any, error) {
	if shapeCheck {
		shapeHits.Add(1)
	}
	perm, err := sc.AccessTablePerm(tblname, `read`)
	if err != nil {
		return 0, nil, err
	}
	columns := sh.columns
	if err = sc.AccessColumns(tblname, &columns, false); err != nil {
		return 0, nil, err
	}
	text, stmt := sh.text, sh.stmt
	if !sameSlice(columns, sh.columns) && !sh.query {
		sel := sh.sel
		sel.Columns = PrepareColumns(columns)
		sel.Args = sw.args
		text = sel.SQL()
		stmt = nil
	}
	if shapeCheck {
		b, early, late := buildSelect(tblname, inColumns, id, inOrder, offset, limit, inWhere, query, group, all)
		sel := b.sel
		if !b.query {
			sel.Columns = PrepareColumns(columns)
		}
		if early != nil || late != nil || sel.SQL() != text || !sameArgs(sel.Args, sw.args) {
			panic(fmt.Sprintf("shape of select %q %v is not the select %q %v: %v %v", text, sw.args, sel.SQL(),
				sel.Args, early, late))
		}
	}
	var rows *sqldb.Rows
	if stmt != nil {
		rows, err = sc.DbTransaction.QueryStatement(stmt, sw.args)
	} else {
		rows, err = sc.DbTransaction.QueryText(text, sw.args)
	}
	if err != nil {
		return 0, nil, logErrorDB(err, fmt.Sprintf("selecting rows from table %s %s where %s order %s",
			tblname, PrepareColumns(columns), sh.sel.Where, sh.sel.Order))
	}
	return sc.readSelect(rows, perm)
}

// readSelect reads the rows of a select of DBSelect, which the filter of the permissions of the
// table allows
func (sc *SmartContract) readSelect(rows *sqldb.Rows, perm map[string]string) (int64, []any, error) {
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return 0, nil, logErrorDB(err, "getting rows columns")
	}
	// values are the columns of a row as database/sql scans them into a *[]byte each
	values := make([][]byte, len(cols))
	result := make([]any, 0, 50)
	for rows.Next() {
		err = rows.ScanBytes(values)
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
