/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package dataquery

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Where is a filter: column → value for equality (null: is null), column → {"$op": value}, and
// "$and" / "$or" → a list of filters. The operators: $eq $neq $gt $gte $lt $lte, $in $nin (a list),
// $like $begin $end and their case-insensitive $ilike $ibegin $iend (a string).
type Where = map[string]any

// Order is the order of the rows: {column: "asc" | "desc"} entries, the first the most significant
type Order = []map[string]string

const (
	// The most conditions one filter holds, and how deep $and / $or nest
	maxConditions = 256
	maxDepth      = 16
)

var (
	identifier = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
	pathKey    = regexp.MustCompile(`^[a-z0-9_]+$`)
)

// schema is what a query may name: the table's columns with their types, those the reader may read
type schema interface {
	column(name string) (t Type, withZone bool, found bool)
	readable(name string) error
}

// statement collects the SQL of a query and the values it binds, in the order of their $n
type statement struct {
	schema     schema
	args       []any
	conditions int
}

func (s *statement) param(value string) string {
	s.args = append(s.args, value)
	return "$" + strconv.Itoa(len(s.args))
}

// ref is a column, or a text value at a JSON path in a json column (column->key->key)
type ref struct {
	column   string
	path     []string
	t        Type
	withZone bool
}

func (r ref) sql() string {
	quoted := `"` + r.column + `"`
	if len(r.path) == 0 {
		return quoted
	}
	return `jsonb_extract_path_text(` + quoted + `, '` + strings.Join(r.path, `', '`) + `')`
}

// The SQL that reads the column or path as its JSON value in a result
func (r ref) selectSQL() string {
	if len(r.path) == 0 {
		return selectText(r.sql(), r.t, r.withZone)
	}
	return `CAST(jsonb_extract_path("` + r.column + `", '` + strings.Join(r.path, `', '`) + `') AS text)`
}

// resultType is the type of the column or path in a result: a path reads JSON
func (r ref) resultType() Type {
	if len(r.path) > 0 {
		return TypeJSON
	}
	return r.t
}

// resolve checks a column or path the query names: the column must be the table's, readable by the
// reader, and a path must be in a json column, of keys of [a-z0-9_]
func (s *statement) resolve(name string) (ref, error) {
	parts := strings.Split(name, "->")
	if !identifier.MatchString(parts[0]) {
		return ref{}, ErrColumn(name)
	}
	t, withZone, found := s.schema.column(parts[0])
	if !found {
		return ref{}, ErrColumn(name)
	}
	r := ref{column: parts[0], path: parts[1:], t: t, withZone: withZone}
	if len(r.path) > 0 {
		if t != TypeJSON {
			return ref{}, ErrColumn(name)
		}
		for _, key := range r.path {
			if !pathKey.MatchString(key) {
				return ref{}, ErrColumn(name)
			}
		}
		// A value at a path compares as text
		r.t = TypeText
	}
	if err := s.schema.readable(parts[0]); err != nil {
		return ref{}, err
	}
	return r, nil
}

var comparisons = map[string]string{
	"$eq": "=", "$neq": "<>", "$gt": ">", "$gte": ">=", "$lt": "<", "$lte": "<=",
}

// like operators: the SQL operator, and whether the pattern is anchored at the start and the end
var likes = map[string]struct {
	op         string
	start, end bool
}{
	"$like": {"LIKE", false, false}, "$begin": {"LIKE", true, false}, "$end": {"LIKE", false, true},
	"$ilike": {"ILIKE", false, false}, "$ibegin": {"ILIKE", true, false}, "$iend": {"ILIKE", false, true},
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// where turns a filter into SQL, every value bound
func (s *statement) where(filter Where, depth int) (string, error) {
	if depth > maxDepth {
		return "", ErrWhere("too deep")
	}
	keys := make([]string, 0, len(filter))
	for key := range filter {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		var (
			part string
			err  error
		)
		switch key {
		case "$and", "$or":
			part, err = s.logic(key, filter[key], depth)
		default:
			part, err = s.condition(key, filter[key])
		}
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "TRUE", nil
	}
	return "(" + strings.Join(parts, " AND ") + ")", nil
}

func (s *statement) logic(key string, value any, depth int) (string, error) {
	list, ok := value.([]any)
	if !ok {
		return "", ErrWhere(key + " takes a list")
	}
	if len(list) == 0 {
		if key == "$and" {
			return "TRUE", nil
		}
		return "FALSE", nil
	}
	parts := make([]string, 0, len(list))
	for _, item := range list {
		filter, ok := item.(map[string]any)
		if !ok {
			return "", ErrWhere(key + " takes filters")
		}
		part, err := s.where(filter, depth+1)
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	joiner := " AND "
	if key == "$or" {
		joiner = " OR "
	}
	return "(" + strings.Join(parts, joiner) + ")", nil
}

func (s *statement) condition(name string, value any) (string, error) {
	r, err := s.resolve(name)
	if err != nil {
		return "", err
	}
	operators, isMap := value.(map[string]any)
	if !isMap {
		return s.compare(r, name, "$eq", value)
	}
	if len(operators) == 0 {
		return "", ErrWhere(name + " has no operator")
	}
	ops := make([]string, 0, len(operators))
	for op := range operators {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	parts := make([]string, 0, len(ops))
	for _, op := range ops {
		part, err := s.compare(r, name, op, operators[op])
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	return "(" + strings.Join(parts, " AND ") + ")", nil
}

func (s *statement) count() error {
	s.conditions++
	if s.conditions > maxConditions {
		return ErrWhere("too many conditions")
	}
	return nil
}

func (s *statement) compare(r ref, name, op string, value any) (string, error) {
	if err := s.count(); err != nil {
		return "", err
	}
	if sqlOp, ok := comparisons[op]; ok {
		if value == nil {
			switch op {
			case "$eq":
				return r.sql() + " IS NULL", nil
			case "$neq":
				return r.sql() + " IS NOT NULL", nil
			}
			return "", ErrValue(name)
		}
		if r.t == TypeJSON && op != "$eq" && op != "$neq" {
			return "", ErrWhere(op + " on json " + name)
		}
		text, sql, ok := bind(value, r.t)
		if !ok {
			return "", ErrValue(name)
		}
		return r.sql() + " " + sqlOp + " " + sql(s.param(text)), nil
	}
	if like, ok := likes[op]; ok {
		pattern, isString := value.(string)
		if !isString || r.t == TypeJSON || r.t == TypeBytes {
			return "", ErrValue(name)
		}
		pattern = likeEscaper.Replace(pattern)
		if !like.start {
			pattern = "%" + pattern
		}
		if !like.end {
			pattern += "%"
		}
		return "CAST(" + r.sql() + " AS text) " + like.op + " CAST(" + s.param(pattern) + ` AS text) ESCAPE '\'`, nil
	}
	if op == "$in" || op == "$nin" {
		list, isList := value.([]any)
		if !isList {
			return "", ErrValue(name)
		}
		if len(list) == 0 {
			if op == "$in" {
				return "FALSE", nil
			}
			return "TRUE", nil
		}
		items := make([]string, 0, len(list))
		for _, item := range list {
			if err := s.count(); err != nil {
				return "", err
			}
			text, sql, ok := bind(item, r.t)
			if item == nil || !ok {
				return "", ErrValue(name)
			}
			items = append(items, sql(s.param(text)))
		}
		in := " IN ("
		if op == "$nin" {
			in = " NOT IN ("
		}
		return r.sql() + in + strings.Join(items, ", ") + ")", nil
	}
	return "", ErrWhere("unknown operator " + op)
}

// order turns an order into SQL, ending with the id when the table has one and the order does not
// name it, so that pages never overlap
func (s *statement) order(order Order) (string, error) {
	parts := make([]string, 0, len(order)+1)
	byID := false
	for _, entry := range order {
		if len(entry) != 1 {
			return "", ErrWhere("an order entry names one column")
		}
		for name, direction := range entry {
			r, err := s.resolve(name)
			if err != nil {
				return "", err
			}
			switch strings.ToLower(direction) {
			case "asc":
				parts = append(parts, r.sql()+" ASC")
			case "desc":
				parts = append(parts, r.sql()+" DESC")
			default:
				return "", ErrWhere("order " + direction)
			}
			byID = byID || (r.column == "id" && len(r.path) == 0)
		}
	}
	if _, _, hasID := s.schema.column("id"); hasID && !byID {
		parts = append(parts, `"id" ASC`)
	}
	return strings.Join(parts, ", "), nil
}

// DecodeJSON reads a filter, an order or a request body, its numbers kept as written
func DecodeJSON(data []byte, v any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(v); err != nil {
		return ErrWhere("not JSON: " + err.Error())
	}
	if decoder.More() {
		return ErrWhere("not JSON: trailing data")
	}
	return nil
}
