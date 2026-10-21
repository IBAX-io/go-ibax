/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package queryBuilder

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"

	log "github.com/sirupsen/logrus"
)

const (
	prefTimestamp      = "timestamp"
	prefTimestampSpace = "timestamp "
)

var (
	checkNowRE = regexp.MustCompile(`(now\s*\(\s*\)|localtime|current_date|current_time|current_timestamp)`)
	ErrNow     = errors.New(`it is prohibited to use current_time or current_date or localtime or current_timestamp sql keyword`)
)

// KeyTableChecker checks table
type KeyTableChecker interface {
	IsKeyTable(string) bool
}

type NextIDGetter interface {
	GetNextID(string) (int64, error)
}

type SQLQueryBuilder struct {
	*log.Entry
	tableID      string
	Table        string
	isKeyTable   bool
	prepared     bool
	keyEcosystem string
	keyName      string
	Fields       []string
	FieldValues  []any
	stringValues []string
	Where        *types.Map
	KeyTableChkr KeyTableChecker
	whereExpr    string
	whereArgs    []any
	TxEcoID      int64
}

func (b *SQLQueryBuilder) Prepare() error {
	if b.prepared {
		return nil
	}

	idNames := strings.SplitN(b.Table, `_`, 2)
	if len(idNames) == 2 {
		b.keyName = idNames[1]

		if b.KeyTableChkr.IsKeyTable(b.keyName) {
			b.isKeyTable = true
			if b.TxEcoID > 1 {
				if b.Table == "1_keys" {
					b.keyEcosystem = idNames[0]
				} else {
					b.keyEcosystem = converter.Int64ToStr(b.TxEcoID)
				}
			} else {
				b.keyEcosystem = idNames[0]
			}
			b.Table = `1_` + b.keyName

			if contains, ecosysIndx := isParamsContainsEcosystem(b.Fields, b.FieldValues); contains {
				if b.Where.IsEmpty() {
					b.keyEcosystem = fmt.Sprint(b.FieldValues[ecosysIndx])
				}
			} else {
				b.Fields = append(b.Fields, "ecosystem")
				b.FieldValues = append(b.FieldValues, b.keyEcosystem)
			}
		}
	}

	if err := b.normalizeValues(); err != nil {
		b.WithError(err).Error("on normalize field values")
		return err
	}

	values, err := converter.InterfaceSliceToStr(b.FieldValues)
	if err != nil {
		b.WithFields(log.Fields{"type": consts.ConversionError, "error": err}).Error("on convert field values to string")
		return err
	}

	b.stringValues = values
	b.prepared = true
	return nil
}

func (b *SQLQueryBuilder) SetTableID(id string) {
	b.tableID = id
}

func (b *SQLQueryBuilder) TableID() string {
	return b.tableID
}

// GetSelectExpr returns the select query and the values bound to its placeholders
func (b *SQLQueryBuilder) GetSelectExpr() (string, []any, error) {
	if err := b.Prepare(); err != nil {
		return "", nil, err
	}

	fieldsExpr, err := b.GetSQLSelectFieldsExpr()
	if err != nil {
		b.WithError(err).Error("on getting sql fields statement")
		return "", nil, err
	}

	whereExpr, whereArgs, err := b.GetSQLWhereExpr()
	if err != nil {
		b.WithError(err).Error("on getting sql where statement")
		return "", nil, err
	}
	return fmt.Sprintf(`SELECT %s FROM "%s" %s`, fieldsExpr, b.Table, whereExpr), whereArgs, nil
}

func (b *SQLQueryBuilder) GetSQLSelectFieldsExpr() (string, error) {
	if err := b.Prepare(); err != nil {
		return "", err
	}

	sqlFields := make([]string, 0, len(b.Fields)+1)
	sqlFields = append(sqlFields, "id")

	for i := range b.Fields {
		b.Fields[i] = strings.TrimSpace(strings.ToLower(b.Fields[i]))
		sqlFields = append(sqlFields, toSQLField(b.Fields[i]))
	}

	return strings.Join(sqlFields, ","), nil
}

// GetSQLWhereExpr returns the WHERE clause and the values bound to its placeholders
func (b *SQLQueryBuilder) GetSQLWhereExpr() (string, []any, error) {
	if err := b.Prepare(); err != nil {
		return "", nil, err
	}
	if b.Where.IsEmpty() {
		return "", nil, nil
	}
	if b.whereExpr != "" {
		return b.whereExpr, b.whereArgs, nil
	}
	if b.isKeyTable {
		if _, isEcosystem := b.Where.Get(`ecosystem`); !isEcosystem {
			b.Where.Set(`ecosystem`, converter.StrToInt64(b.keyEcosystem))
		}
	}
	where, args, err := GetWhere(b.Where)
	if err != nil {
		return ``, nil, err
	}
	if len(where) == 0 {
		return "", nil, nil
	}
	b.whereExpr, b.whereArgs = " WHERE "+where, args
	return b.whereExpr, b.whereArgs, nil
}

// GetSQLUpdateExpr returns the SET list of the update and the values bound to its placeholders
func (b *SQLQueryBuilder) GetSQLUpdateExpr(logData map[string]string) (string, []any, error) {
	if err := b.Prepare(); err != nil {
		return "", nil, err
	}

	expressions := make([]string, 0, len(b.Fields))
	args := make([]any, 0, len(b.Fields))
	jsonFields := make(map[string]map[string]string)

	for i := 0; i < len(b.Fields); i++ {
		if b.isKeyTable && b.Fields[i] == "ecosystem" {
			continue
		}

		if strings.Contains(b.Fields[i], `->`) {
			colfield := strings.Split(b.Fields[i], `->`)
			if len(colfield) == 2 {
				if jsonFields[colfield[0]] == nil {
					jsonFields[colfield[0]] = make(map[string]string)
				}
				jsonFields[colfield[0]][colfield[1]] = b.stringValues[i]
				continue
			}
		}

		field, value := b.Fields[i], b.stringValues[i]
		switch {
		case syspar.IsByteColumn(b.Table, field) && len(value) != 0:
			expressions = append(expressions, field+"="+sqlHexExpr)
			args = append(args, hex.EncodeToString([]byte(value)))
		case field[:1] == "+" || field[:1] == "-":
			expressions = append(expressions, field[1:]+"="+field[1:]+field[:1]+`?::numeric`)
			args = append(args, value)
		case value == `NULL`:
			expressions = append(expressions, field+"= NULL")
		case strings.HasPrefix(field, prefTimestampSpace):
			expressions = append(expressions, field[len(prefTimestampSpace):]+`= to_timestamp(?)`)
			args = append(args, value)
		case strings.HasPrefix(value, prefTimestampSpace):
			expressions = append(expressions, field+`= ?::timestamp`)
			args = append(args, value[len(prefTimestampSpace):])
		default:
			expressions = append(expressions, `"`+field+`"=?`)
			args = append(args, value)
		}
	}
	var jsonColumnArr []string
	for name := range jsonFields {
		jsonColumnArr = append(jsonColumnArr, name)
	}
	sort.Strings(jsonColumnArr)
	for _, name := range jsonColumnArr {
		val := jsonFields[name]
		var initial string
		out, err := json.Marshal(val)
		if err != nil {
			log.WithFields(log.Fields{"error": err, "type": consts.JSONMarshallError}).Error("marshalling update columns for jsonb")
			return "", nil, err
		}

		if len(logData[name]) > 0 && logData[name] != `NULL` {
			initial = name
		} else {
			initial = `'{}'`
		}

		expressions = append(expressions, fmt.Sprintf(`%s=%s::jsonb || ?::jsonb`, name, initial))
		args = append(args, string(out))
	}

	return strings.Join(expressions, ","), args, nil
}

// GetSQLInsertQuery returns the insert query and the values bound to its placeholders
func (b *SQLQueryBuilder) GetSQLInsertQuery(idGetter NextIDGetter) (string, []any, error) {
	if err := b.Prepare(); err != nil {
		return "", nil, err
	}

	isID := false
	insFields := []string{}
	insValues := []string{}
	args := make([]any, 0, len(b.Fields)+1)
	jsonFields := make(map[string]map[string]string)

	for i := 0; i < len(b.Fields); i++ {
		if b.Fields[i] == `id` {
			isID = true
			b.tableID = b.stringValues[i]
		}

		if strings.Contains(b.Fields[i], `->`) {
			colfield := strings.Split(b.Fields[i], `->`)
			if len(colfield) == 2 {
				if jsonFields[colfield[0]] == nil {
					jsonFields[colfield[0]] = make(map[string]string)
				}
				jsonFields[colfield[0]][colfield[1]] = b.stringValues[i]
				continue
			}
		}

		insFields = append(insFields, toSQLField(b.Fields[i]))
		expr, arg := b.toSQLValue(b.stringValues[i], b.Fields[i])
		insValues = append(insValues, expr)
		if arg != nil {
			args = append(args, arg)
		}
	}

	var jsonColumnArr []string
	for name := range jsonFields {
		jsonColumnArr = append(jsonColumnArr, name)
	}
	sort.Strings(jsonColumnArr)
	for _, name := range jsonColumnArr {
		val := jsonFields[name]
		out, err := json.Marshal(val)
		if err != nil {
			log.WithFields(log.Fields{"error": err, "type": consts.JSONMarshallError}).Error("marshalling update columns for jsonb")
			return "", nil, err
		}
		insFields = append(insFields, name)
		insValues = append(insValues, `?::jsonb`)
		args = append(args, string(out))
	}

	if !isID {
		id, err := idGetter.GetNextID(b.Table)
		if err != nil {
			return "", nil, err
		}

		b.tableID = converter.Int64ToStr(id)
		insFields = append(insFields, `id`)
		insValues = append(insValues, `?`)
		args = append(args, b.tableID)
	}

	flds := strings.Join(insFields, ",")
	vls := strings.Join(insValues, ",")

	return fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s)`, b.Table, flds, vls), args, nil
}

func (b SQLQueryBuilder) GenerateRollBackInfoString(logData map[string]string) (string, error) {
	rollbackInfo := make(map[string]string)
	for k, v := range logData {
		var eco bool
		idNames := strings.SplitN(b.Table, `_`, 2)
		if len(idNames) == 2 {
			b.keyName = idNames[1]
			if b.KeyTableChkr.IsKeyTable(b.keyName) {
				eco = true
			}
		}
		if k == `id` || (b.isKeyTable && !eco) {
			continue
		}
		rollbackInfo[k] = v
	}

	jsonRollbackInfo, err := json.Marshal(rollbackInfo)
	if err != nil {
		b.Logger.WithFields(log.Fields{"type": consts.JSONMarshallError, "error": err}).Error("marshalling rollback info to json")
		return "", err
	}

	return string(jsonRollbackInfo), nil
}

// toSQLValue returns the expression of an inserted value and the value bound to its placeholder,
// nil when the expression has none
func (b SQLQueryBuilder) toSQLValue(rawValue, rawField string) (string, any) {
	if syspar.IsByteColumn(b.Table, rawField) && len(rawValue) != 0 {
		return sqlHexExpr, hex.EncodeToString([]byte(rawValue))
	}

	if rawValue == `NULL` {
		return `NULL`, nil
	}

	if strings.HasPrefix(rawField, prefTimestamp) {
		return `to_timestamp(?)`, rawValue
	}

	if strings.HasPrefix(rawValue, prefTimestamp) {
		return `?::timestamp`, rawValue[len(prefTimestampSpace):]
	}

	return `?`, rawValue
}

func (b SQLQueryBuilder) normalizeValues() error {
	for i, v := range b.FieldValues {
		switch val := v.(type) {
		case string:
			if strings.HasPrefix(strings.TrimSpace(val), prefTimestamp) {
				if err := CheckNow(val); err != nil {
					return err
				}
			}

			if len(b.Fields) > i && syspar.IsByteColumn(b.Table, b.Fields[i]) {
				if vbyte, err := hex.DecodeString(val); err == nil {
					b.FieldValues[i] = vbyte
				}
			}
		}
	}

	return nil
}

func isParamsContainsEcosystem(fields []string, ivalues []any) (bool, int) {
	ecosysIndx := getFieldIndex(fields, `ecosystem`)
	if ecosysIndx >= 0 && len(ivalues) > ecosysIndx && converter.StrToInt64(fmt.Sprint(ivalues[ecosysIndx])) > 0 {
		return true, ecosysIndx
	}

	return false, -1
}

// sqlHexExpr is the expression of a bytea value, bound as its hex
const sqlHexExpr = ` decode(?,'HEX')`

// BoundStatement returns a statement together with the values bound to its placeholders, the form
// in which a statement is hashed
func BoundStatement(query string, args []any) string {
	if len(args) == 0 {
		return query
	}
	return query + " " + fmt.Sprintf("%q", args)
}

func toSQLField(rawField string) string {
	if rawField[:1] == "+" || rawField[:1] == "-" {
		return rawField[1:]
	}

	if strings.HasPrefix(rawField, prefTimestampSpace) {
		return rawField[len(prefTimestampSpace):]
	}

	if strings.Contains(rawField, `->`) {
		return rawField[:strings.Index(rawField, `->`)]
	}

	return wrapString(rawField, `"`)
}

func wrapString(raw, wrapper string) string {
	return wrapper + raw + wrapper
}

// CheckNow allows check if the content contains postgres NOW()
func CheckNow(inputs ...string) error {
	for _, item := range inputs {
		if checkNowRE.Match([]byte(strings.ToLower(item))) {
			return ErrNow
		}
	}
	return nil
}

func getFieldIndex(fields []string, name string) int {
	for i, v := range fields {
		if strings.ToLower(v) == name {
			return i
		}
	}
	return -1
}

func (b *SQLQueryBuilder) GetEcosystem() string {
	return b.keyEcosystem
}

// KeyTable returns the name of the table without its ecosystem when it is a table of the first
// ecosystem, which keeps the rows of all the ecosystems, and "" otherwise. The builder must be
// prepared.
func (b *SQLQueryBuilder) KeyTable() string {
	if b.isKeyTable {
		return b.keyName
	}
	return ""
}

// FieldValue returns the value of a field of the prepared builder
func (b *SQLQueryBuilder) FieldValue(field string) (string, bool) {
	for i, name := range b.Fields {
		if name == field {
			return b.stringValues[i], true
		}
	}
	return "", false
}

// AddField adds a field and its value to the prepared builder
func (b *SQLQueryBuilder) AddField(field, value string) {
	b.Fields = append(b.Fields, field)
	b.FieldValues = append(b.FieldValues, value)
	b.stringValues = append(b.stringValues, value)
}

func (b *SQLQueryBuilder) IsEmptyWhere() bool {
	return len(b.whereExpr) == 0
}
