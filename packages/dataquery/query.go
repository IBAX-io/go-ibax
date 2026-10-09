/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package dataquery

import (
	"strconv"
	"strings"
)

const (
	DefaultLimit = 25
	MaxLimit     = 1000
	MaxOffset    = 1_000_000
)

// Query is a read of rows: the columns or JSON paths (column->key) to read, every column the reader
// may read when none is named; the filter; the order; and the page
type Query struct {
	Ecosystem int64    `json:"ecosystem"`
	Columns   []string `json:"columns"`
	Where     Where    `json:"where"`
	Order     Order    `json:"order"`
	Limit     *int     `json:"limit"`
	Offset    int      `json:"offset"`
}

// Result is a page of rows: how many rows the filter matches, the columns read and their values
type Result struct {
	Count   int64            `json:"count"`
	Columns []Column         `json:"columns"`
	List    []map[string]any `json:"list"`
}

// RowResult is one row and the columns read
type RowResult struct {
	Columns []Column       `json:"columns"`
	Value   map[string]any `json:"value"`
}

// selection resolves the columns a read names: named ones must be readable, unnamed ones are those
// the reader may read
func (t *Table) selection(s *statement, names []string) ([]ref, error) {
	refs := make([]ref, 0, len(t.ordered))
	if len(names) == 0 {
		for _, name := range t.ordered {
			if t.readable(name) == nil {
				r, _ := s.resolve(name)
				refs = append(refs, r)
			}
		}
		if len(refs) == 0 {
			return nil, ErrAccessDenied(t.Name)
		}
		return refs, nil
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			return nil, ErrColumn(name)
		}
		seen[name] = true
		r, err := s.resolve(name)
		if err != nil {
			return nil, err
		}
		refs = append(refs, r)
	}
	return refs, nil
}

func names(refs []ref, given []string) []Column {
	columns := make([]Column, len(refs))
	for i, r := range refs {
		name := r.column
		if len(given) > 0 {
			name = given[i]
		}
		columns[i] = Column{Name: name, Type: r.resultType()}
	}
	return columns
}

func (t *Table) read(text string, args []any, columns []Column) ([]map[string]any, error) {
	rows, err := query(text, args...)
	if err != nil {
		return nil, queryFailed(err, t.physical)
	}
	defer rows.Close()
	list := []map[string]any{}
	cells := make([]*string, len(columns))
	targets := make([]any, len(columns))
	for i := range cells {
		targets[i] = &cells[i]
	}
	for rows.Next() {
		if err := rows.Scan(targets...); err != nil {
			return nil, queryFailed(err, t.physical)
		}
		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column.Name] = Decode(cells[i], column.Type)
		}
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, queryFailed(err, t.physical)
	}
	return list, nil
}

// Query reads a page of the table's rows
func (t *Table) Query(q Query) (*Result, error) {
	limit := DefaultLimit
	if q.Limit != nil {
		limit = *q.Limit
	}
	if limit < 1 || limit > MaxLimit {
		return nil, ErrLimit("limit " + strconv.Itoa(limit))
	}
	if q.Offset < 0 || q.Offset > MaxOffset {
		return nil, ErrLimit("offset " + strconv.Itoa(q.Offset))
	}
	s := &statement{schema: t}
	refs, err := t.selection(s, q.Columns)
	if err != nil {
		return nil, err
	}
	where, err := s.where(q.Where, 0)
	if err != nil {
		return nil, err
	}
	order, err := s.order(q.Order)
	if err != nil {
		return nil, err
	}
	filter := ` FROM ` + t.from() + ` WHERE ` + t.scope() + ` AND ` + where

	result := &Result{Columns: names(refs, q.Columns)}
	rows, err := query(`SELECT count(*)`+filter, s.args...)
	if err != nil {
		return nil, queryFailed(err, t.physical)
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&result.Count); err != nil {
			return nil, queryFailed(err, t.physical)
		}
	}
	rows.Close()

	selects := make([]string, len(refs))
	for i, r := range refs {
		selects[i] = r.selectSQL()
	}
	text := `SELECT ` + strings.Join(selects, ", ") + filter
	if order != "" {
		text += ` ORDER BY ` + order
	}
	text += ` LIMIT ` + strconv.Itoa(limit) + ` OFFSET ` + strconv.Itoa(q.Offset)
	if result.List, err = t.read(text, s.args, result.Columns); err != nil {
		return nil, err
	}
	return result, nil
}

// Row reads the row whose column (the id when none is given) holds a value
func (t *Table) Row(column, value string, columns []string) (*RowResult, error) {
	if column == "" {
		column = "id"
	}
	s := &statement{schema: t}
	refs, err := t.selection(s, columns)
	if err != nil {
		return nil, err
	}
	key, err := s.resolve(column)
	if err != nil {
		return nil, err
	}
	if len(key.path) > 0 {
		return nil, ErrColumn(column)
	}
	cond, err := s.compare(key, column, "$eq", value)
	if err != nil {
		return nil, err
	}
	selects := make([]string, len(refs))
	for i, r := range refs {
		selects[i] = r.selectSQL()
	}
	result := &RowResult{Columns: names(refs, columns)}
	list, err := t.read(`SELECT `+strings.Join(selects, ", ")+` FROM `+t.from()+` WHERE `+t.scope()+
		` AND `+cond+` LIMIT 1`, s.args, result.Columns)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound(t.Name + " " + value)
	}
	result.Value = list[0]
	return result, nil
}

// Sum adds up a number, money or double column over the rows a filter matches, as a decimal string
func (t *Table) Sum(column string, where Where) (string, error) {
	s := &statement{schema: t}
	r, err := s.resolve(column)
	if err != nil {
		return "", err
	}
	if len(r.path) > 0 || (r.t != TypeNumber && r.t != TypeMoney && r.t != TypeDouble) {
		return "", ErrColumn(column)
	}
	filter, err := s.where(where, 0)
	if err != nil {
		return "", err
	}
	rows, err := query(`SELECT CAST(COALESCE(sum(`+r.sql()+`), 0) AS text) FROM `+t.from()+` WHERE `+t.scope()+
		` AND `+filter, s.args...)
	if err != nil {
		return "", queryFailed(err, t.physical)
	}
	defer rows.Close()
	var sum string
	if rows.Next() {
		if err := rows.Scan(&sum); err != nil {
			return "", queryFailed(err, t.physical)
		}
	}
	return sum, rows.Err()
}
