/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package template

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/types"

	log "github.com/sirupsen/logrus"
)

const (
	columnTypeText     = "text"
	columnTypeLongText = "long_text"
	columnTypeBlob     = "blob"

	substringLength = 32

	errComma = `unexpected comma`
)

func dbfindExpressionBlob(column string) string {
	return fmt.Sprintf(`md5(%s) "%[1]s"`, column)
}

func dbfindExpressionLongText(column string) string {
	return fmt.Sprintf(`json_build_array(
		substr(%s, 1, %d),
		CASE WHEN length(%[1]s)>%[2]d THEN md5(%[1]s) END) "%[1]s"`, column, substringLength)
}

type valueLink struct {
	title string

	id     string
	table  string
	column string
	hash   string
}

func (vl *valueLink) link() string {
	if len(vl.hash) > 0 {
		return fmt.Sprintf("/data/%s/%s/%s/%s", vl.table, vl.id, vl.column, vl.hash)
	}
	return ""
}

func (vl *valueLink) marshal() (string, error) {
	b, err := json.Marshal(map[string]string{
		"title": vl.title,
		"link":  vl.link(),
	})
	if err != nil {
		log.WithFields(log.Fields{"type": consts.JSONMarshallError, "error": err}).Error("marshalling valueLink to JSON")
		return "", err
	}
	return string(b), nil
}

func trimString(in []rune) string {
	out := strings.TrimSpace(string(in))
	if len(out) > 0 && out[0] == '"' && out[len(out)-1] == '"' {
		out = out[1 : len(out)-1]
	}
	return out
}

func ParseObject(in []rune) (any, int, error) {
	var (
		ret            any
		key            string
		mapMode, quote bool
	)

	length := len(in)
	if in[0] == '[' {
		ret = make([]any, 0)
	} else if in[0] == '{' {
		ret = types.NewMap()
		mapMode = true
	}
	addEmptyKey := func() {
		if mapMode {
			ret.(*types.Map).Set(key, "")
		} else if len(key) > 0 {
			ret = append(ret.([]any), types.LoadMap(map[string]any{key: ``}))
		}
		key = ``
	}
	start := 1
	i := 1
	prev := ' '
main:
	for ; i < length; i++ {
		ch := in[i]
		if quote && ch != '"' {
			continue
		}
		switch ch {
		case ']':
			if !mapMode {
				break main
			}
		case '}':
			if mapMode {
				break main
			}
		case '{', '[':
			par, off, err := ParseObject(in[i:])
			if err != nil {
				return nil, i, err
			}
			if mapMode {
				if len(key) == 0 {
					switch v := par.(type) {
					case map[string]any:
						for ikey, ival := range v {
							ret.(*types.Map).Set(ikey, ival)
						}
					}
				} else {
					ret.(*types.Map).Set(key, par)
					key = ``
				}
			} else {
				if len(key) > 0 {
					par = types.LoadMap(map[string]any{key: par})
					key = ``
				}
				ret = append(ret.([]any), par)
			}
			i += off
			start = i + 1
		case '"':
			quote = !quote
		case ':':
			if len(key) == 0 {
				key = trimString(in[start:i])
				start = i + 1
			}
		case ',':
			val := trimString(in[start:i])
			if prev == ch {
				return nil, i, fmt.Errorf(errComma)
			}
			if len(val) == 0 && len(key) > 0 {
				addEmptyKey()
			}
			if len(val) > 0 {
				if mapMode {
					ret.(*types.Map).Set(key, val)
					key = ``
				} else {
					if len(key) > 0 {
						ret = append(ret.([]any), types.LoadMap(map[string]any{key: val}))
						key = ``
					} else {
						ret = append(ret.([]any), val)
					}
				}
			}
			start = i + 1
		}
		if ch != ' ' {
			prev = ch
		}
	}
	if prev == ',' {
		return nil, i, fmt.Errorf(errComma)
	}
	if start < i {
		if last := trimString(in[start:i]); len(last) > 0 {
			if mapMode {
				ret.(*types.Map).Set(key, last)
			} else {
				if len(key) > 0 {
					ret = append(ret.([]any), types.LoadMap(map[string]any{key: last}))
					key = ``
				} else {
					ret = append(ret.([]any), last)
				}
			}
		} else if len(key) > 0 {
			addEmptyKey()
		}
	}
	switch v := ret.(type) {
	case *types.Map:
		if v.Size() == 0 {
			ret = ``
		}
	case map[string]any:
		if len(v) == 0 {
			ret = ``
		}
	case []any:
		if len(v) == 0 {
			ret = ``
		}
	}
	return ret, i, nil
}
