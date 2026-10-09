/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package dataquery

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Type is a column type as the API names it
type Type string

const (
	TypeText      Type = "text"
	TypeNumber    Type = "number"
	TypeDouble    Type = "double"
	TypeMoney     Type = "money"
	TypeJSON      Type = "json"
	TypeTimestamp Type = "timestamp"
	TypeBytes     Type = "bytes"
)

// TypeOf names the type of a PostgreSQL column (information_schema data_type)
func TypeOf(dataType string) Type {
	switch {
	case dataType == "bigint" || dataType == "integer" || dataType == "smallint":
		return TypeNumber
	case dataType == "numeric":
		return TypeMoney
	case dataType == "double precision" || dataType == "real":
		return TypeDouble
	case dataType == "jsonb" || dataType == "json" || dataType == "boolean":
		return TypeJSON
	case strings.HasPrefix(dataType, "timestamp") || dataType == "date":
		return TypeTimestamp
	case dataType == "bytea":
		return TypeBytes
	default:
		return TypeText
	}
}

// Column is a column of a result: its name, or the JSON path it reads, and its type
type Column struct {
	Name string `json:"name"`
	Type Type   `json:"type"`
}

const maxSafeInteger = 1<<53 - 1

var (
	integerText = regexp.MustCompile(`^-?[0-9]+$`)
	decimalText = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)
	hexText     = regexp.MustCompile(`^(?:[0-9a-f]{2})*$`)
)

// selectText is how a column of a type is read: as text, decoded by Decode
func selectText(expr string, t Type, withZone bool) string {
	switch t {
	case TypeTimestamp:
		if withZone {
			expr += ` AT TIME ZONE 'UTC'`
		}
		return `to_char(` + expr + `, 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`
	case TypeBytes:
		return `encode(` + expr + `, 'hex')`
	default:
		return `CAST(` + expr + ` AS text)`
	}
}

// Decode turns the text of a cell into its JSON value: integers within 2^53 and doubles as numbers,
// larger integers and money as decimal strings, json as itself, timestamps as ISO 8601 in UTC,
// bytes as lowercase hex
func Decode(text *string, t Type) any {
	if text == nil {
		return nil
	}
	value := *text
	switch t {
	case TypeNumber:
		if n, err := strconv.ParseInt(value, 10, 64); err == nil && n >= -maxSafeInteger && n <= maxSafeInteger {
			return n
		}
		return value
	case TypeDouble:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return f
	case TypeJSON:
		if !json.Valid([]byte(value)) {
			return nil
		}
		return json.RawMessage(value)
	default:
		return value
	}
}

// bind checks a value given for a column of a type, and returns its text with the SQL that reads
// the text ($n) as that type
func bind(value any, t Type) (text string, sql func(param string) string, ok bool) {
	asText := func(param string) string { return `CAST(` + param + ` AS text)` }
	cast := func(sqlType string) func(string) string {
		return func(param string) string { return `CAST(CAST(` + param + ` AS text) AS ` + sqlType + `)` }
	}
	switch t {
	case TypeNumber:
		text, ok = numberText(value, integerText)
		return text, cast("bigint"), ok
	case TypeMoney:
		text, ok = numberText(value, decimalText)
		return text, cast("numeric"), ok
	case TypeDouble:
		if n, isNumber := value.(json.Number); isNumber {
			if _, err := n.Float64(); err == nil {
				return n.String(), cast("double precision"), true
			}
		}
		return "", nil, false
	case TypeTimestamp:
		s, isString := value.(string)
		if !isString {
			return "", nil, false
		}
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
			if at, err := time.Parse(layout, s); err == nil {
				return at.UTC().Format("2006-01-02T15:04:05.999999"), cast("timestamp"), true
			}
		}
		return "", nil, false
	case TypeBytes:
		s, isString := value.(string)
		if !isString || !hexText.MatchString(s) {
			return "", nil, false
		}
		return s, func(param string) string { return `decode(CAST(` + param + ` AS text), 'hex')` }, true
	case TypeJSON:
		data, err := json.Marshal(value)
		if err != nil {
			return "", nil, false
		}
		return string(data), cast("jsonb"), true
	default:
		switch v := value.(type) {
		case string:
			return v, asText, true
		case json.Number:
			return v.String(), asText, true
		}
		return "", nil, false
	}
}

func numberText(value any, pattern *regexp.Regexp) (string, bool) {
	var text string
	switch v := value.(type) {
	case json.Number:
		text = v.String()
	case string:
		text = v
	default:
		return "", false
	}
	return text, pattern.MatchString(text)
}
