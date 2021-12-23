/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package querycost

import (
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TestTableRowCounter struct {
}

const tableRowCount = 10000

func (t *TestTableRowCounter) RowCount(tx *sqldb.DbTransaction, tableName string) (int64, error) {
	if tableName == "small" {
		return tableRowCount, nil
	}
	return 0, errors.New("Unknown table")
}

type QueryCostByFormulaTestSuite struct {
	suite.Suite
	queryCoster QueryCoster
}

func (s *QueryCostByFormulaTestSuite) SetupTest() {
	s.queryCoster = &FormulaQueryCoster{&TestTableRowCounter{}}
}

func (s *QueryCostByFormulaTestSuite) TestQueryCostUnknownQueryType() {
	_, err := s.queryCoster.QueryCost(nil, "UNSELECT * FROM name")
	assert.Error(s.T(), err)
	assert.Equal(s.T(), err, UnknownQueryTypeError)
}

func (s *QueryCostByFormulaTestSuite) TestGetTableNameFromSelectNoTable() {
	tableName, err := SelectQueryType("select 3").GetTableName()
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), tableName, "")
}

func (s *QueryCostByFormulaTestSuite) TestGetTableNameFromSelect() {
	tableName, err := SelectQueryType("select a from keys where 3=1").GetTableName()
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), tableName, "keys")
	tableName, err = SelectQueryType(`select a,  b,  c from "1_keys" where 3=1`).GetTableName()
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), tableName, "1_keys")
}

func (s *QueryCostByFormulaTestSuite) TestGetTableNameFromInsertNoInto() {
	_, err := InsertQueryType(`insert "1_keys"(id) values (1)`).GetTableName()
	assert.Error(s.T(), err)
	assert.Equal(s.T(), err, IntoStatementMissingError)
}

func (s *QueryCostByFormulaTestSuite) TestGetTableNameFromInsert() {
	tableName, err := InsertQueryType("insert into keys(a,b,c) values (1,2,3)").GetTableName()
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), tableName, "keys")
	tableName, err = InsertQueryType(`insert into "1_keys" (a,b,c) values (1,2,3)`).GetTableName()
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), tableName, "1_keys")
}

func (s *QueryCostByFormulaTestSuite) TestGetTableNameFromUpdateNoSet() {
	_, err := UpdateQueryType(`update keys a = b where id = 1`).GetTableName()
	assert.Error(s.T(), err)
	assert.Equal(s.T(), err, SetStatementMissingError)
}

func (s *QueryCostByFormulaTestSuite) TestGetTableNameFromUpdate() {
	tableName, err := UpdateQueryType("update keys set a = 1 where id = 2").GetTableName()
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), tableName, "keys")
	tableName, err = UpdateQueryType(`update "1_keys" set a = 1`).GetTableName()
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), tableName, "1_keys")
}

func (s *QueryCostByFormulaTestSuite) TestGetTableNameFromDeleteNoFrom() {
	_, err := DeleteQueryType(`delete table where id = 1`).GetTableName()
	assert.Error(s.T(), err)
	assert.Equal(s.T(), err, FromStatementMissingError)
}

func (s *QueryCostByFormulaTestSuite) TestGetTableNameFromDeleteNoTable() {
	_, err := DeleteQueryType(`delete from`).GetTableName()
	assert.Error(s.T(), err)
	assert.Equal(s.T(), err, DeleteMinimumThreeFieldsError)
}

func (s *QueryCostByFormulaTestSuite) TestGetTableNameFromDelete() {
	tableName, err := DeleteQueryType("delete from keys").GetTableName()
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), tableName, "keys")
	tableName, err = DeleteQueryType(`delete from "1_keys" where a = 1`).GetTableName()
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), tableName, "1_keys")
}

func (s *QueryCostByFormulaTestSuite) TestQueryCostSelect() {
	cost, err := s.queryCoster.QueryCost(nil, "SELECT * FROM small WHERE id = ?", 3)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), cost, SelectQueryType("").CalculateCost(tableRowCount))
}

func (s *QueryCostByFormulaTestSuite) TestQueryCostUpdate() {
	cost, err := s.queryCoster.QueryCost(nil, "UPDATE small SET a = ?", 3)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), cost, UpdateQueryType("").CalculateCost(tableRowCount))
}

func (s *QueryCostByFormulaTestSuite) TestQueryCostDelete() {
	cost, err := s.queryCoster.QueryCost(nil, "DELETE FROM small")
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), cost, DeleteQueryType("").CalculateCost(tableRowCount))
}

func (s *QueryCostByFormulaTestSuite) TestQueryCostInsert() {
	cost, err := s.queryCoster.QueryCost(nil, "INSERT INTO small(a,b) VALUES (1,2)")
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), cost, InsertQueryType("").CalculateCost(tableRowCount))
}

func (s *QueryCostByFormulaTestSuite) TestQueryCostInsertWrongTable() {
	_, err := s.queryCoster.QueryCost(nil, "INSERT INTO unknown(a,b) VALUES (1,2)")
	assert.Error(s.T(), err)
}

func TestQueryCostFormula(t *testing.T) {
	suite.Run(t, new(QueryCostByFormulaTestSuite))
}
