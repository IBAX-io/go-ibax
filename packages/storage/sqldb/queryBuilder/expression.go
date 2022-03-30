package queryBuilder

import (
	"fmt"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"
)

func GetOrder(tblname string, inOrder any) (string, error) {
	var (
		orders           []string
		defaultSortOrder = map[string]string{
			`keys`:    "ecosystem,id",
			`members`: "ecosystem,id",
		}
	)
	cols := types.NewMap()

	sanitize := func(in string, value any) {
		in = converter.Sanitize(strings.ToLower(in), ``)
		if len(in) > 0 {
			cols.Set(in, true)
			in = `"` + in + `"`
			if fmt.Sprint(value) == `-1` {
				in += ` desc`
			} else if fmt.Sprint(value) == `1` {
				in += ` asc`
			} else {
				in += ` asc`
			}
			orders = append(orders, in)
		} else {
			orders = append(orders, `"id" asc`)
		}
	}

	if v, ok := defaultSortOrder[tblname[2:]]; ok {
		for _, item := range strings.Split(v, `,`) {
			cols.Set(item, false)
		}
	} else {
		cols.Set(`id`, false)
	}
	switch v := inOrder.(type) {
	case string:
		sanitize(v, nil)
	case *types.Map:
		for _, ikey := range v.Keys() {
			item, _ := v.Get(ikey)
			sanitize(ikey, item)
		}
	case map[string]any:
		for ikey, item := range v {
			sanitize(ikey, item)
		}
	case []any:
		for _, item := range v {
			switch param := item.(type) {
			case string:
				sanitize(param, nil)
			case *types.Map:
				for _, ikey := range param.Keys() {
					item, _ := param.Get(ikey)
					sanitize(ikey, item)
				}
			case map[string]any:
				for key, value := range param {
					sanitize(key, value)
				}
			}
		}
	}
	for _, key := range cols.Keys() {
		if state, found := cols.Get(key); !found || !state.(bool) {
			orders = append(orders, key)
		}
	}
	if err := CheckNow(orders...); err != nil {
		return ``, err
	}
	return strings.Join(orders, `,`), nil
}

func GetColumns(inColumns any) ([]string, error) {
	var columns []string

	switch v := inColumns.(type) {
	case string:
		if len(v) > 0 {
			columns = strings.Split(v, `,`)
		}
	case []any:
		for _, name := range v {
			switch col := name.(type) {
			case string:
				columns = append(columns, col)
			}
		}
	}
	if len(columns) == 0 {
		columns = []string{`*`}
	}
	for i, v := range columns {
		columns[i] = converter.Sanitize(strings.ToLower(v), `*->`)
	}
	if err := CheckNow(columns...); err != nil {
		return nil, err
	}
	return columns, nil
}

func GetTableName(ecosystem int64, tblname string) string {
	return converter.ParseTable(tblname, ecosystem)
}
