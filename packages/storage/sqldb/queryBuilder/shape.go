/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package queryBuilder

// The shapes of the statements of the builder. The shape of a statement is all its text depends
// on: the table, the fields as they are written, the class of each value, the structure of a where
// map. The text of a shape is made by the builder, once; the walks of this file write the key of
// the shape of a statement and the values bound to its placeholders, following the builder value
// by value, so that a statement of a known shape is made without the builder. The walks check the
// values as the builder does, and tell when a value fails a check: the builder, run then, makes
// the error.

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// MaxShapeFields is the most fields of a statement that has a shape
const MaxShapeFields = 64

// the classes of the string of a value, which are what the text of a statement depends on of a
// value: the builder writes an empty value of a byte column, the value NULL and a value with the
// prefix of a timestamp apart
const (
	classPlain byte = iota
	classNull
	classTimestamp
	classEmpty
)

func valueClass(s string) byte {
	switch {
	case len(s) == 0:
		return classEmpty
	case s == `NULL`:
		return classNull
	case strings.HasPrefix(s, prefTimestampSpace):
		return classTimestamp
	}
	return classPlain
}

// ValueString is the string Prepare makes of the value of a field, converter.InterfaceToStr
func ValueString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case decimal.Decimal:
		return x.String(), nil
	}
	return converter.InterfaceToStr(v)
}

// Sprint is fmt.Sprint of a value, the string the builder makes of a value of a where map
func Sprint(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case decimal.Decimal:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(v)
}

// appendKeyString appends a string to the key of a shape, with its length before it
func appendKeyString(key []byte, s string) []byte {
	key = binary.AppendUvarint(key, uint64(len(s)))
	return append(key, s...)
}

// WhereValue is a value of a where map, as the builder visits them: the string it binds, the
// column whose data type checks it, whether the builder checks it, and whether the builder binds
// it. The builder visits and checks the values of the conditions before an operator that ends the
// conditions of its map, but binds only the values of the operator.
type WhereValue struct {
	S      string
	Column string
	Check  bool
	Bind   bool
}

// WhereWalk writes the shape of a where map and its values
type WhereWalk struct {
	Key    []byte
	Values []WhereValue
}

// Where appends to the key the shape of the where map of GetWhere and to the values those of the
// map. It tells false for a map GetWhere fails on whatever its values: a map of an empty $in or
// $nin, whose false result GetWhere gives as an error.
func (w *WhereWalk) Where(m *types.Map) bool {
	return !w.where(``, m)
}

// whereKey is the key of a where map as getWhere reads it, without allocating for a key of the
// lower case ASCII letters, digits, _ and $, which it reads as it is
func whereKey(key string) string {
	for i := 0; i < len(key); i++ {
		c := key[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '$') {
			return PrepareWhere(converter.Sanitize(strings.ToLower(key), `->$`))
		}
	}
	return key
}

// unbind marks the values from start as values the builder checks without binding them
func (w *WhereWalk) unbind(start int) {
	for i := start; i < len(w.Values); i++ {
		w.Values[i].Bind = false
	}
}

func (w *WhereWalk) value(s, column string, check bool) {
	w.Values = append(w.Values, WhereValue{S: s, Column: column, Check: check, Bind: true})
}

// where walks a where map of getWhere whose operators apply to the column. It tells true where
// getWhere gives errWhereFalse: an empty $in or $nin, or a $and or $or one of whose maps gives
// it. getWhere then drops the values of the map, and those of the maps around it up to a column
// or the top, where it stops as the walk does. A map that tells true has unbound all its values
// already. Whether a map gives errWhereFalse is a function of the key, as is the text then.
func (w *WhereWalk) where(column string, m *types.Map) bool {
	w.Key = append(w.Key, '{')
	start := len(w.Values)
	for link := m.First(); link != nil; link = link.Next() {
		raw, v := link.Key(), link.Value()
		w.Key = appendKeyString(w.Key, raw)
		key := whereKey(raw)
		switch key {
		case `$like`, `$end`, `$begin`, `$ilike`, `$iend`, `$ibegin`:
			w.unbind(start)
			prefix, suffix := `%`, `%`
			switch key {
			case `$end`, `$iend`:
				suffix = ``
			case `$begin`, `$ibegin`:
				prefix = ``
			}
			w.value(prefix+Sprint(v)+suffix, ``, false)
			w.Key = append(w.Key, '}')
			return false
		case `$and`, `$or`:
			list, ok := v.([]any)
			if !ok {
				w.Key = append(w.Key, 'x')
				continue
			}
			w.Key = append(w.Key, 'L')
			w.Key = binary.AppendUvarint(w.Key, uint64(len(list)))
			for _, item := range list {
				if im, ok := item.(*types.Map); ok {
					if w.where(column, im) {
						w.unbind(start)
						w.Key = append(w.Key, '}')
						return true
					}
				} else {
					w.Key = append(w.Key, '_')
				}
			}
		case `$in`, `$nin`:
			w.unbind(start)
			empty := false
			switch x := v.(type) {
			case string:
				if len(x) > 0 {
					w.Key = append(w.Key, 's', '1')
					w.value(x, column, true)
				} else {
					w.Key = append(w.Key, 's', '0')
					empty = true
				}
			case []any:
				w.Key = append(w.Key, 'a')
				w.Key = binary.AppendUvarint(w.Key, uint64(len(x)))
				for _, item := range x {
					w.value(Sprint(item), column, true)
				}
				empty = len(x) == 0
			default:
				w.Key = append(w.Key, 'x')
			}
			w.Key = append(w.Key, '}')
			return empty
		case `$eq`, `$neq`, `$gt`, `$gte`, `$lt`, `$lte`:
			w.unbind(start)
			w.value(Sprint(v), column, true)
			w.Key = append(w.Key, '}')
			return false
		default:
			keyColumn := ``
			if !strings.Contains(key, `>`) && len(key) > 0 {
				keyColumn = key
			}
			switch x := v.(type) {
			case []any:
				w.Key = append(w.Key, 'a')
				w.Key = binary.AppendUvarint(w.Key, uint64(len(x)))
				for _, item := range x {
					if im, ok := item.(*types.Map); ok {
						// getWhere takes errWhereFalse of the map of a column as its condition
						w.where(keyColumn, im)
					} else {
						w.Key = append(w.Key, 'v')
						w.value(Sprint(item), keyColumn, true)
					}
				}
			case *types.Map:
				w.where(keyColumn, x)
			case []byte:
				w.Key = append(w.Key, 'b')
				w.value(hex.EncodeToString(x), ``, false)
			default:
				if s := Sprint(x); s == `$isnull` {
					w.Key = append(w.Key, 'n')
				} else {
					w.Key = append(w.Key, 'v')
					w.value(s, keyColumn, true)
				}
			}
		}
	}
	w.Key = append(w.Key, '}')
	return false
}

// WhereTypes are the data types of the columns that check the values of a where map
func WhereTypes(table string, values []WhereValue) []string {
	dataTypes := make([]string, len(values))
	for i, v := range values {
		if v.Check && len(v.Column) > 0 {
			dataTypes[i] = syspar.ColumnDataType(table, v.Column)
		}
	}
	return dataTypes
}

// CheckWhere checks the values of a where map with the data types of their columns, as the
// builder does
func CheckWhere(values []WhereValue, dataTypes []string) bool {
	for i, v := range values {
		if v.Check && checkDateTimeValue(dataTypes[i], v.S) != nil {
			return false
		}
	}
	return true
}

// AppendWhereArgs appends the values the builder binds of a where map
func AppendWhereArgs(args []any, values []WhereValue) []any {
	for _, v := range values {
		if v.Bind {
			args = append(args, v.S)
		}
	}
	return args
}

// FieldMeta is what the builder reads of the data type of the column of a field: whether Prepare
// decodes a hex string of it, whether its non empty value is bound as hex, and the data type
// that checks its value
type FieldMeta struct {
	Decode bool
	Byte   bool
	Type   string
}

// WriteWalk writes the shape of a statement of writeRows, an update with the select of the rows it
// changes or an insert, and the values bound to its placeholders
type WriteWalk struct {
	WhereWalk
	// Table is the table of the statement, as Prepare names it, KeyName its name without the
	// ecosystem when it is a table of the first ecosystem
	Table, KeyName string
	IsKeyTable     bool
	// KeyEcosystem is the ecosystem of a statement on a table of the first ecosystem
	KeyEcosystem string
	// Eco tells that Prepare adds the field ecosystem with the value KeyEcosystem
	Eco    bool
	Exists bool
	Fields []string
	// Strs are the strings of the values, and of the ecosystem Prepare adds
	Strs []string
	// TableID is the id an insert gives, when a field gives it
	TableID string
	HasID   bool
}

// Field is the field i, of those of Prepare
func (w *WriteWalk) Field(i int) string {
	if i == len(w.Fields) {
		return `ecosystem`
	}
	return w.Fields[i]
}

// NumFields is the number of the fields of Prepare
func (w *WriteWalk) NumFields() int {
	if w.Eco {
		return len(w.Fields) + 1
	}
	return len(w.Fields)
}

// Walk writes the key of the shape of the fields and values of the statement writeRows makes of
// the fields, which are in lower case, and the values, and the strings of the values. It tells
// false for a statement the walk does not make: one of more than MaxShapeFields fields, of a field
// the builder does not write, of a value it does not convert, an update without condition or an
// insert with one. The where map of an update is walked by WalkWhere.
func (w *WriteWalk) Walk(chk KeyTableChecker, table string, fields []string, values []any, where *types.Map,
	txEcoID int64, exists bool) bool {
	*w = WriteWalk{WhereWalk: WhereWalk{Key: w.Key[:0], Values: w.Values[:0]}, Strs: w.Strs[:0],
		Table: table, Fields: fields, Exists: exists}
	if len(fields) > MaxShapeFields || len(values) != len(fields) || exists == where.IsEmpty() {
		return false
	}
	w.Key = append(w.Key, 'W')
	if exists {
		w.Key = append(w.Key, 'u')
	} else {
		w.Key = append(w.Key, 'i')
	}
	w.Key = appendKeyString(w.Key, table)
	if i := strings.IndexByte(table, '_'); i >= 0 && chk.IsKeyTable(table[i+1:]) {
		prefix := table[:i]
		w.IsKeyTable, w.KeyName = true, table[i+1:]
		switch {
		case txEcoID <= 1, table == `1_keys`:
			w.KeyEcosystem = prefix
		default:
			w.KeyEcosystem = strconv.FormatInt(txEcoID, 10)
		}
		if prefix != `1` {
			w.Table = `1_` + w.KeyName
		}
		if index := getFieldIndex(fields, `ecosystem`); index >= 0 && converter.StrToInt64(Sprint(values[index])) > 0 {
			if where.IsEmpty() {
				w.KeyEcosystem = Sprint(values[index])
			}
			w.Key = append(w.Key, 'c')
		} else {
			w.Eco = true
			w.Key = append(w.Key, 'e')
		}
	}
	for _, v := range values {
		s, err := ValueString(v)
		if err != nil {
			return false
		}
		w.Strs = append(w.Strs, s)
	}
	if w.Eco {
		w.Strs = append(w.Strs, w.KeyEcosystem)
	}
	w.Key = binary.AppendUvarint(w.Key, uint64(len(fields)))
	for i := 0; i < w.NumFields(); i++ {
		field := w.Field(i)
		if len(strings.TrimSpace(field)) == 0 {
			return false
		}
		w.Key = appendKeyString(w.Key, field)
		w.Key = append(w.Key, valueClass(w.Strs[i]))
	}
	return true
}

// WalkWhere writes the shape of the where map of an update and its values. A condition on the
// ecosystem is set in the map of a table of the first ecosystem that has none, as the builder
// sets it.
func (w *WriteWalk) WalkWhere(where *types.Map) bool {
	if w.IsKeyTable {
		if _, ok := where.Get(`ecosystem`); !ok {
			where.Set(`ecosystem`, converter.StrToInt64(w.KeyEcosystem))
		}
	}
	return w.Where(where)
}

// Meta is what the builder reads of the data types of the columns of the fields
func (w *WriteWalk) Meta() []FieldMeta {
	meta := make([]FieldMeta, w.NumFields())
	for i := range meta {
		field := w.Field(i)
		meta[i].Decode = syspar.IsByteColumn(w.Table, field)
		if w.Exists {
			field = strings.TrimSpace(field)
		}
		meta[i].Byte = syspar.IsByteColumn(w.Table, field)
		meta[i].Type = syspar.ColumnDataType(w.Table, field)
	}
	return meta
}

// Decode replaces a hex string of a byte column with its bytes, as Prepare does, in the values of
// the caller when Prepare replaces them there: when it does not add the field ecosystem to the
// values, or adds it in their capacity. It tells false when the bytes of a value are of another
// class than its hex, such as the hex of NULL, as the statement is then of another shape.
func (w *WriteWalk) Decode(meta []FieldMeta, values []any) bool {
	same := true
	inPlace := !w.Eco || cap(values) > len(values)
	for i, m := range meta {
		if !m.Decode {
			continue
		}
		var s string
		if i < len(values) {
			v, ok := values[i].(string)
			if !ok {
				continue
			}
			s = v
		} else {
			s = w.KeyEcosystem
		}
		if b, err := hex.DecodeString(s); err == nil {
			w.Strs[i] = string(b)
			if valueClass(w.Strs[i]) != valueClass(s) {
				same = false
			}
			if i < len(values) && inPlace {
				values[i] = b
			}
		}
	}
	return same
}

// jsonField splits a field col->key of a jsonb column, as the builder does
func jsonField(field string) (string, string, bool) {
	if !strings.Contains(field, `->`) {
		return ``, ``, false
	}
	col, key, ok := strings.Cut(field, `->`)
	if !ok || strings.Contains(key, `->`) {
		return ``, ``, false
	}
	return col, key, true
}

// appendJSONArgs appends the values of the jsonb columns, as the builder binds them: the object of
// the keys of each column, the columns in order
func appendJSONArgs(args []any, jsonFields map[string]map[string]string) ([]any, bool) {
	cols := make([]string, 0, len(jsonFields))
	for col := range jsonFields {
		cols = append(cols, col)
	}
	slices.Sort(cols)
	for _, col := range cols {
		out, err := json.Marshal(jsonFields[col])
		if err != nil {
			return args, false
		}
		args = append(args, string(out))
	}
	return args, true
}

// SetArgs appends the values the builder binds to the SET list of an update, and tells false
// when a value fails a check of the builder. The values are decoded.
func (w *WriteWalk) SetArgs(meta []FieldMeta, args []any) ([]any, bool) {
	var jsonFields map[string]map[string]string
	for i := 0; i < w.NumFields(); i++ {
		field, value := strings.TrimSpace(w.Field(i)), w.Strs[i]
		if w.IsKeyTable && field == `ecosystem` {
			continue
		}
		if col, key, ok := jsonField(field); ok {
			if jsonFields == nil {
				jsonFields = make(map[string]map[string]string)
			}
			if jsonFields[col] == nil {
				jsonFields[col] = make(map[string]string)
			}
			jsonFields[col][key] = value
			continue
		}
		switch {
		case meta[i].Byte && len(value) != 0:
			args = append(args, hex.EncodeToString([]byte(value)))
		case field[0] == '+' || field[0] == '-':
			args = append(args, value)
		case value == `NULL`:
		case strings.HasPrefix(field, prefTimestampSpace):
			if checkUnixTime(value) != nil {
				return args, false
			}
			args = append(args, value)
		case strings.HasPrefix(value, prefTimestampSpace):
			if checkDateTimeValue(`timestamp`, value[len(prefTimestampSpace):]) != nil {
				return args, false
			}
			args = append(args, value[len(prefTimestampSpace):])
		default:
			if checkDateTimeValue(meta[i].Type, value) != nil {
				return args, false
			}
			args = append(args, value)
		}
	}
	return appendJSONArgs(args, jsonFields)
}

// InsertArgs appends the values the builder binds to an insert but the id it takes when no field
// gives it, and tells false when a value fails a check of the builder. The values are decoded.
func (w *WriteWalk) InsertArgs(meta []FieldMeta, args []any) ([]any, bool) {
	var jsonFields map[string]map[string]string
	for i := 0; i < w.NumFields(); i++ {
		field, value := w.Field(i), w.Strs[i]
		if field == `id` {
			w.HasID, w.TableID = true, value
		}
		if col, key, ok := jsonField(field); ok {
			if jsonFields == nil {
				jsonFields = make(map[string]map[string]string)
			}
			if jsonFields[col] == nil {
				jsonFields[col] = make(map[string]string)
			}
			jsonFields[col][key] = value
			continue
		}
		switch {
		case meta[i].Byte && len(value) != 0:
			args = append(args, hex.EncodeToString([]byte(value)))
		case value == `NULL`:
		case strings.HasPrefix(field, prefTimestampSpace):
			if checkUnixTime(value) != nil {
				return args, false
			}
			args = append(args, value)
		case strings.HasPrefix(value, prefTimestampSpace):
			if checkDateTimeValue(`timestamp`, value[len(prefTimestampSpace):]) != nil {
				return args, false
			}
			args = append(args, value[len(prefTimestampSpace):])
		default:
			if checkDateTimeValue(meta[i].Type, value) != nil {
				return args, false
			}
			args = append(args, value)
		}
	}
	return appendJSONArgs(args, jsonFields)
}

// AppendRollBackData appends to dst the data of the rollback record of a row an update changes of
// the table, as the builder of a prepared statement on the table writes it (AppendRollBackInfo)
func AppendRollBackData(dst []byte, keys []string, chk KeyTableChecker, table string, isKeyTable bool,
	logData map[string]string) ([]byte, []string) {
	b := SQLQueryBuilder{Table: table, isKeyTable: isKeyTable, KeyTableChkr: chk}
	return b.AppendRollBackInfo(dst, keys, logData)
}

// JSONColumns are the jsonb columns the SET list of an update sets keys of, in order: the text of
// the list depends on whether the row has a value of each
func (w *WriteWalk) JSONColumns() []string {
	var cols []string
	for i := 0; i < w.NumFields(); i++ {
		field := strings.TrimSpace(w.Field(i))
		if w.IsKeyTable && field == `ecosystem` {
			continue
		}
		if col, _, ok := jsonField(field); ok && !slices.Contains(cols, col) {
			cols = append(cols, col)
		}
	}
	slices.Sort(cols)
	return cols
}

// HasJSONValue tells whether the SET list of an update sets the keys of the jsonb column on its
// value, rather than on an empty object: the row has a value of it
func HasJSONValue(logData map[string]string, col string) bool {
	return len(logData[col]) > 0 && logData[col] != `NULL`
}
