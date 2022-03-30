/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package querycost

import (
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

type QueryCosterType int

const (
	ExplainQueryCosterType        QueryCosterType = iota
	ExplainAnalyzeQueryCosterType QueryCosterType = iota
	FormulaQueryCosterType        QueryCosterType = iota
)

type QueryCoster interface {
	QueryCost(*sqldb.DbTransaction, string, ...any) (int64, error)
}

type ExplainQueryCoster struct {
}

func (*ExplainQueryCoster) QueryCost(transaction *sqldb.DbTransaction, query string, args ...any) (int64, error) {
	return explainQueryCost(transaction, true, query, args...)
}

type ExplainAnalyzeQueryCoster struct {
}

func (*ExplainAnalyzeQueryCoster) QueryCost(transaction *sqldb.DbTransaction, query string, args ...any) (int64, error) {
	return explainQueryCost(transaction, true, query, args...)
}

func GetQueryCoster(tp QueryCosterType) QueryCoster {
	switch tp {
	case ExplainQueryCosterType:
		return &ExplainQueryCoster{}
	case ExplainAnalyzeQueryCosterType:
		return &ExplainAnalyzeQueryCoster{}
	case FormulaQueryCosterType:
		return &FormulaQueryCoster{&DBCountQueryRowCounter{}}
	}
	return nil
}
