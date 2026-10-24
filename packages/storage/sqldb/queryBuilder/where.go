/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package queryBuilder

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"
)

var (
	errWhereFalse = errors.New(`false result`)
	whereColRE    = regexp.MustCompile(`;([\w\-]+)`)
)

func PrepareWhere(where string) string {
	whereStr := where
	where = strings.Replace(where, "->", ";", -1)
	whereSlice := whereColRE.FindAllStringSubmatchIndex(where, -1)
	startWhere := 0
	out := ``
	for i := 0; i < len(whereSlice); i++ {
		slice := whereSlice[i]
		if len(slice) != 4 {
			continue
		}
		if i < len(whereSlice)-1 && slice[1] == whereSlice[i+1][0] {
			colsWhere := []string{where[slice[2]:slice[3]]}
			from := slice[0]
			for i < len(whereSlice)-1 && slice[1] == whereSlice[i+1][0] {
				i++
				slice = whereSlice[i]
				if len(slice) != 4 {
					break
				}
				colsWhere = append(colsWhere, where[slice[2]:slice[3]])
			}
			out += fmt.Sprintf(`%s::jsonb#>>'{%s}'`, where[startWhere:from], strings.Join(colsWhere, `,`))
			startWhere = slice[3]
		} else {
			out += fmt.Sprintf(`%s->>'%s'`, where[startWhere:slice[0]], where[slice[2]:slice[3]])
			startWhere = slice[3]
		}
	}
	if len(out) > 0 {
		return out + where[startWhere:]
	}
	return whereStr
}

// GetWhere makes the condition of a where map on the table. The values are never put in the
// condition: each one is a ? placeholder and the values are returned in the order of their
// placeholders, as strings, so a database string literal is what each value is compared as. A value
// compared with a date or time column must be a literal.
func GetWhere(table string, inWhere *types.Map) (string, []any, error) {
	return getWhere(table, ``, inWhere)
}

// getWhere makes the condition of a where map whose operators apply to the column, empty when they
// apply to no column of the table
func getWhere(table, column string, inWhere *types.Map) (string, []any, error) {
	var (
		where string
		cond  []string
		args  []any
	)
	if inWhere == nil {
		inWhere = types.NewMap()
	}
	str := func(value any) string {
		return fmt.Sprint(value)
	}
	dataType := ``
	if len(column) > 0 {
		dataType = syspar.ColumnDataType(table, column)
	}
	oper := func(action string, v any) (string, []any, error) {
		if err := checkDateTimeValue(dataType, str(v)); err != nil {
			return ``, nil, err
		}
		return action + ` ?`, []any{str(v)}, nil
	}
	like := func(action, prefix, suffix string, v any) (string, []any, error) {
		return action + ` ?`, []any{prefix + str(v) + suffix}, nil
	}
	in := func(action string, v any) (string, []any, error) {
		var list []any
		switch value := v.(type) {
		case string:
			if len(value) > 0 {
				list = append(list, value)
			}
		case []any:
			for _, ival := range value {
				list = append(list, str(ival))
			}
		default:
			return ``, nil, nil
		}
		for _, ival := range list {
			if err := checkDateTimeValue(dataType, ival.(string)); err != nil {
				return ``, nil, err
			}
		}
		if len(list) == 0 {
			return `false`, nil, errWhereFalse
		}
		return action + ` (` + strings.TrimSuffix(strings.Repeat(`?,`, len(list)), `,`) + `)`, list, nil
	}
	logic := func(action string, v any) (string, []any, error) {
		value, ok := v.([]any)
		if !ok {
			return ``, nil, nil
		}
		var (
			list  []string
			largs []any
		)
		for _, ival := range value {
			if avalue, ok := ival.(*types.Map); ok {
				where, wargs, err := getWhere(table, column, avalue)
				if err != nil {
					return ``, nil, err
				}
				if len(where) > 0 {
					list = append(list, where)
					largs = append(largs, wargs...)
				}
			}
		}
		if len(list) == 0 {
			return ``, nil, nil
		}
		return `(` + strings.Join(list, ` `+action+` `) + `)`, largs, nil
	}
	for _, key := range inWhere.Keys() {
		v, _ := inWhere.Get(key)
		key = PrepareWhere(converter.Sanitize(strings.ToLower(key), `->$`))
		switch key {
		case `$like`:
			return like(`like`, `%`, `%`, v)
		case `$end`:
			return like(`like`, `%`, ``, v)
		case `$begin`:
			return like(`like`, ``, `%`, v)
		case `$ilike`:
			return like(`ilike`, `%`, `%`, v)
		case `$iend`:
			return like(`ilike`, `%`, ``, v)
		case `$ibegin`:
			return like(`ilike`, ``, `%`, v)
		case `$and`, `$or`:
			icond, iargs, err := logic(key[1:], v)
			if err != nil {
				return ``, nil, err
			}
			cond = append(cond, icond)
			args = append(args, iargs...)
		case `$in`:
			return in(`in`, v)
		case `$nin`:
			return in(`not in`, v)
		case `$eq`:
			return oper(`=`, v)
		case `$neq`:
			return oper(`!=`, v)
		case `$gt`:
			return oper(`>`, v)
		case `$gte`:
			return oper(`>=`, v)
		case `$lt`:
			return oper(`<`, v)
		case `$lte`:
			return oper(`<=`, v)
		default:
			keyColumn := ``
			if !strings.Contains(key, `>`) && len(key) > 0 {
				keyColumn = key
				key = `"` + key + `"`
			}
			keyDataType := ``
			if len(keyColumn) > 0 {
				keyDataType = syspar.ColumnDataType(table, keyColumn)
			}
			switch value := v.(type) {
			case []any:
				var acond []string
				for _, iarr := range value {
					switch avalue := iarr.(type) {
					case *types.Map:
						ret, rargs, err := getWhere(table, keyColumn, avalue)
						if err == errWhereFalse {
							acond = append(acond, ret)
						} else {
							if err != nil {
								return ``, nil, err
							}
							acond = append(acond, `(`+key+` `+ret+`)`)
							args = append(args, rargs...)
						}
					default:
						if err := checkDateTimeValue(keyDataType, str(avalue)); err != nil {
							return ``, nil, err
						}
						acond = append(acond, key+` = ?`)
						args = append(args, str(avalue))
					}
				}
				if len(acond) > 0 {
					cond = append(cond, `(`+strings.Join(acond, ` and `)+`)`)
				}
			case *types.Map:
				ret, rargs, err := getWhere(table, keyColumn, value)
				if err == errWhereFalse {
					cond = append(cond, ret)
				} else {
					if err != nil {
						return ``, nil, err
					}
					cond = append(cond, `(`+key+` `+ret+`)`)
					args = append(args, rargs...)
				}
			case []byte:
				cond = append(cond, key+` = decode(?,'HEX')`)
				args = append(args, hex.EncodeToString(value))
			default:
				if ival := str(value); ival == `$isnull` {
					cond = append(cond, key+` is null`)
				} else {
					if err := checkDateTimeValue(keyDataType, ival); err != nil {
						return ``, nil, err
					}
					cond = append(cond, key+` = ?`)
					args = append(args, ival)
				}
			}
		}
	}
	if len(cond) > 0 {
		where = strings.Join(cond, ` and `)
		if err := CheckNow(where); err != nil {
			return ``, nil, err
		}
	}
	return where, args, nil
}
