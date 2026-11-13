/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

// SingleResult is a structure for the single result
type SingleResult struct {
	result []byte
	err    error
}

// Single is retrieving single result
func (dbTx *DbTransaction) Single(query string, args ...any) *SingleResult {
	var result []byte
	err := dbTx.rowRaw(query, args...).Scan(&result)
	switch {
	case err == sql.ErrNoRows:
		return &SingleResult{[]byte(""), nil}
	case err != nil:
		return &SingleResult{[]byte(""), err}
	}
	return &SingleResult{result, nil}
}

// Int64 converts bytes to int64
func (r *SingleResult) Int64() (int64, error) {
	if r.err != nil {
		return 0, r.err
	}
	return converter.BytesToInt64(r.result), nil
}

// Int converts bytes to int
func (r *SingleResult) Int() (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	return converter.BytesToInt(r.result), nil
}

// Float64 converts string to float64
func (r *SingleResult) Float64() (float64, error) {
	if r.err != nil {
		return 0, r.err
	}
	return converter.StrToFloat64(string(r.result)), nil
}

// String returns string
func (r *SingleResult) String() (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return string(r.result), nil
}

// Bytes returns []byte
func (r *SingleResult) Bytes() ([]byte, error) {
	if r.err != nil {
		return []byte(""), r.err
	}
	return r.result, nil
}

// OneRow is storing one row result
type OneRow struct {
	result map[string]string
	List   []map[string]string
	err    error
}

// String is extracts result from OneRow as string
func (r *OneRow) String() (map[string]string, error) {
	if r.err != nil {
		return r.result, r.err
	}
	return r.result, nil
}

// Bytes is extracts result from OneRow as []byte
func (r *OneRow) Bytes() (map[string][]byte, error) {
	result := make(map[string][]byte)
	if r.err != nil {
		return result, r.err
	}
	for k, v := range r.result {
		result[k] = []byte(v)
	}
	return result, nil
}

// Int64 is extracts result from OneRow as int64
func (r *OneRow) Int64() (map[string]int64, error) {
	result := make(map[string]int64)
	if r.err != nil {
		return result, r.err
	}
	for k, v := range r.result {
		result[k] = converter.StrToInt64(v)
	}
	return result, nil
}

// Float64 is extracts result from OneRow as float64
func (r *OneRow) Float64() (map[string]float64, error) {
	result := make(map[string]float64)
	if r.err != nil {
		return result, r.err
	}
	for k, v := range r.result {
		result[k] = converter.StrToFloat64(v)
	}
	return result, nil
}

// Int is extracts result from OneRow as int
func (r *OneRow) Int() (map[string]int, error) {
	result := make(map[string]int)
	if r.err != nil {
		return result, r.err
	}
	for k, v := range r.result {
		result[k] = converter.StrToInt(v)
	}
	return result, nil
}

// GetAllTransaction is retrieve all query result rows
func (dbTx *DbTransaction) GetAllTransaction(query string, countRows int, args ...any) ([]map[string]string, error) {
	var rows *Rows
	var err error
	if countRows > 0 {
		var std *sql.Rows
		if std, err = GetDB(dbTx).Raw(query, args...).Limit(countRows).Rows(); err == nil {
			rows = StdRows(std)
		}
	} else {
		rows, err = dbTx.queryRaw(query, args...)
	}
	return allRows(query, args, rows, err, countRows)
}

// allRows is the result GetAllTransaction makes of the rows of the query, or of its error
func allRows(query string, args []any, rows *Rows, err error, countRows int) ([]map[string]string, error) {
	if err != nil {
		return nil, fmt.Errorf("%w in query %s %s", err, query, args)
	}
	defer rows.Close()
	result, err := getResult(rows, countRows)
	if err != nil {
		return nil, fmt.Errorf("%w in query %s %s", err, query, args)
	}
	return result, nil
}

// GetOneRowTransaction returns one row from transactions
func (dbTx *DbTransaction) GetOneRowTransaction(query string, args ...any) *OneRow {
	rows, err := dbTx.queryRaw(query, args...)
	return oneRow(query, args, rows, err)
}

// oneRow is the result GetOneRowTransaction makes of the rows of the query, or of its error
func oneRow(query string, args []any, rows *Rows, err error) *OneRow {
	result := make(map[string]string)
	all, err := allRows(query, args, rows, err, -1)
	if err != nil {
		return &OneRow{result: result, err: fmt.Errorf("%w in query %s %s", err, query, args)}
	}
	if len(all) == 0 {
		return &OneRow{result: result, err: nil}
	}
	return &OneRow{result: all[0], List: all, err: nil}
}

// GetOneRow returns one row
func (dbTx *DbTransaction) GetOneRow(query string, args ...any) *OneRow {
	return dbTx.GetOneRowTransaction(query, args...)
}

func (dbTx *DbTransaction) GetRows(tableName, columns string, offset, limit int) ([]map[string]string, error) {
	query := GetDB(dbTx).Table(tableName).Order("id").Offset(offset).Limit(limit)
	if len(columns) > 0 {
		columns = "id," + columns
		query = query.Select(columns)
	}
	rows, err := query.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return getResult(StdRows(rows), -1)
}

func GetResult(rows *sql.Rows) ([]map[string]string, error) {
	return getResult(StdRows(rows), -1)
}

// ListResult is a structure for the list result
type ListResult struct {
	result []string
	err    error
}

// String return the slice of strings
func (r *ListResult) String() ([]string, error) {
	if r.err != nil {
		return r.result, r.err
	}
	return r.result, nil
}

// GetList returns the result of the query as ListResult variable
func (dbTx *DbTransaction) GetList(query string, args ...any) *ListResult {
	var result []string
	all, err := dbTx.GetAllTransaction(query, -1, args...)
	if err != nil {
		return &ListResult{result, err}
	}
	for _, v := range all {
		for _, v2 := range v {
			result = append(result, v2)
		}
	}
	return &ListResult{result, nil}
}

func getResult(rows *Rows, countRows int) ([]map[string]string, error) {
	var result []map[string]string
	defer rows.Close()
	types, err := rows.DatabaseTypeNames()
	if err != nil {
		return nil, err
	}
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	// values are the columns of a row as database/sql scans them into a *[]byte each: nil for NULL
	values := make([][]byte, len(columns))
	r := 0
	for rows.Next() {
		if err = rows.ScanBytes(values); err != nil {
			return nil, err
		}
		var value string
		rez := make(map[string]string, len(columns))
		for i, col := range values {
			switch {
			case col == nil:
				value = "NULL"
			case types[i] == "BYTEA":
				value = hex.EncodeToString(col)
			default:
				value = string(col)
			}
			rez[columns[i]] = value
		}
		result = append(result, rez)
		r++
		if countRows != -1 && r >= countRows {
			break
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
