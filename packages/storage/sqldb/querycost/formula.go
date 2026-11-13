/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package querycost

import (
	"errors"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

const (
	Select = "select"
	Insert = "insert"
	Update = "update"
	Delete = "delete"

	Set  = "set"
	From = "from"
	Into = "into"

	Quote  = `"`
	Lparen = "("
)

const (
	SelectCost = 1
	UpdateCost = 1
	InsertCost = 1
	DeleteCost = 1

	SelectRowCoeff = 0.0001
	InsertRowCoeff = 0.0001
	DeleteRowCoeff = 0.0001
	UpdateRowCoeff = 0.0001
)

var FromStatementMissingError = errors.New("FROM statement missing")
var DeleteMinimumThreeFieldsError = errors.New("DELETE query must consist minimum of 3 fields")
var SetStatementMissingError = errors.New("SET statement missing")
var IntoStatementMissingError = errors.New("INTO statement missing")
var UnknownQueryTypeError = errors.New("Unknown query type")

func strSliceIndex(fields []string, fieldToFind string) (index int) {
	for i, field := range fields {
		if field == fieldToFind {
			index = i
			break
		}
	}
	return
}

type TableRowCounter interface {
	RowCount(*sqldb.DbTransaction, string) (int64, error)
}

type DBCountQueryRowCounter struct {
}

func (d *DBCountQueryRowCounter) RowCount(transaction *sqldb.DbTransaction, tableName string) (int64, error) {
	count, err := transaction.GetRecordsCountTx(tableName, ``)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": tableName}).Error("Getting record count from table")
	}
	return count, err
}

type FormulaQueryCoster struct {
	rowCounter TableRowCounter
}

type QueryType interface {
	GetTableName() (string, error)
	CalculateCost(int64) int64
}

type SelectQueryType string

func (s SelectQueryType) GetTableName() (string, error) {
	queryFields := strings.Fields(string(s))
	fromFieldIndex := strSliceIndex(queryFields, From)
	if fromFieldIndex == 0 {
		return "", nil
	}
	return strings.Trim(queryFields[fromFieldIndex+1], Quote), nil
}

func (s SelectQueryType) CalculateCost(rowCount int64) int64 {
	return SelectCost + int64(SelectRowCoeff*float64(rowCount))
}

type UpdateQueryType string

func (s UpdateQueryType) GetTableName() (string, error) {
	queryFields := strings.Fields(string(s))
	setFieldIndex := strSliceIndex(queryFields, Set)
	if setFieldIndex == 0 {
		return "", SetStatementMissingError
	}
	return strings.Trim(queryFields[setFieldIndex-1], Quote), nil
}

func (s UpdateQueryType) CalculateCost(rowCount int64) int64 {
	return UpdateCost + int64(UpdateRowCoeff*float64(rowCount))
}

type InsertQueryType string

func (s InsertQueryType) GetTableName() (string, error) {
	queryFields := strings.Fields(string(s))
	intoFieldIndex := strSliceIndex(queryFields, Into)
	if intoFieldIndex == 0 {
		return "", IntoStatementMissingError
	}
	tableNameValuesField := queryFields[intoFieldIndex+1]
	tableName := ""
	lparenIndex := strings.Index(tableNameValuesField, Lparen)
	if lparenIndex > 0 {
		tableName = tableNameValuesField[:lparenIndex]
	} else {
		tableName = tableNameValuesField
	}
	return strings.Trim(tableName, Quote), nil
}

func (s InsertQueryType) CalculateCost(rowCount int64) int64 {
	return InsertCost
}

type DeleteQueryType string

func (s DeleteQueryType) GetTableName() (string, error) {
	queryFields := strings.Fields(string(s))
	fromFieldIndex := strSliceIndex(queryFields, From)
	if fromFieldIndex == 0 {
		return "", FromStatementMissingError
	}
	// DELETE FROM table is minimum
	if len(queryFields) < 3 {
		return "", DeleteMinimumThreeFieldsError
	}
	return strings.Trim(queryFields[fromFieldIndex+1], Quote), nil
}

func (s DeleteQueryType) CalculateCost(rowCount int64) int64 {
	return DeleteCost + int64(DeleteRowCoeff*float64(rowCount))
}

// Parse returns the type of the query and the table it costs the rows of, as QueryCost reads them.
// The type keeps no text of the query.
func Parse(query string) (QueryType, string, error) {
	cleanedQuery := strings.TrimSpace(strings.ToLower(query))
	var queryType QueryType
	switch {
	case strings.HasPrefix(cleanedQuery, Select):
		queryType = SelectQueryType(cleanedQuery)
	case strings.HasPrefix(cleanedQuery, Insert):
		queryType = InsertQueryType(cleanedQuery)
	case strings.HasPrefix(cleanedQuery, Update):
		queryType = UpdateQueryType(cleanedQuery)
	case strings.HasPrefix(cleanedQuery, Delete):
		queryType = DeleteQueryType(cleanedQuery)
	default:
		log.WithFields(log.Fields{"type": consts.ParseError, "query": query}).Error("parsing sql query")
		return nil, "", UnknownQueryTypeError
	}
	tableName, err := queryType.GetTableName()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ParseError, "query": query, "error": err}).Error("getting table name from sql query")
		return nil, "", err
	}
	switch queryType.(type) {
	case SelectQueryType:
		queryType = SelectQueryType("")
	case InsertQueryType:
		queryType = InsertQueryType("")
	case UpdateQueryType:
		queryType = UpdateQueryType("")
	case DeleteQueryType:
		queryType = DeleteQueryType("")
	}
	return queryType, tableName, nil
}

// Cost returns the cost of a query of the type on the table, which Parse returns of the query
func (f *FormulaQueryCoster) Cost(transaction *sqldb.DbTransaction, queryType QueryType, tableName string) (int64, error) {
	rowCount, err := f.rowCounter.RowCount(transaction, tableName)
	if err != nil {
		return 0, err
	}
	return queryType.CalculateCost(rowCount), nil
}

// Formula is the coster of the formula of the rows of the table of a query
var Formula = &FormulaQueryCoster{&DBCountQueryRowCounter{}}

func (f *FormulaQueryCoster) QueryCost(transaction *sqldb.DbTransaction, query string, args ...any) (int64, error) {
	queryType, tableName, err := Parse(query)
	if err != nil {
		return 0, err
	}
	return f.Cost(transaction, queryType, tableName)
}
