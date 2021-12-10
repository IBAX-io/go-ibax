/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"encoding/json"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"

	"github.com/shopspring/decimal"

	log "github.com/sirupsen/logrus"
)

func logError(err error, errType string, comment string) error {
	log.WithFields(log.Fields{"type": errType, "error": err}).Error(comment)
	return err
}

func logErrorf(pattern string, param interface{}, errType string, comment string) error {
	err := fmt.Errorf(pattern, param)
	log.WithFields(log.Fields{"type": errType, "error": err}).Error(comment)
	return err
}

func logErrorShort(err error, errType string) error {
	return logError(err, errType, err.Error())
}

func logErrorfShort(pattern string, param interface{}, errType string) error {
	return logErrorShort(fmt.Errorf(pattern, param), errType)
}

func logErrorValue(err error, errType string, comment, value string) error {
	log.WithFields(log.Fields{"type": errType, "error": err, "value": value}).Error(comment)
	return err
}

func logErrorDB(err error, comment string) error {
	return logError(err, consts.DBError, comment)
}

func unmarshalJSON(input []byte, v interface{}, comment string) (err error) {
	if err = json.Unmarshal(input, v); err != nil {
		return logErrorValue(err, consts.JSONUnmarshallError, comment, string(input))
	}
	return nil
}

func marshalJSON(v interface{}, comment string) (out []byte, err error) {
	out, err = json.Marshal(v)
	if err != nil {
		logError(err, consts.JSONMarshallError, comment)
	}
	return
}

func validateAccess(sc *SmartContract, funcName string) error {
	condition := syspar.GetAccessExec(utils.ToSnakeCase(funcName))

	if err := Eval(sc, condition); err != nil {
		err = fmt.Errorf(eAccessContract, funcName, condition)
		return logError(err, consts.IncorrectCallingContract, err.Error())
	}

	return nil
}

func FillTxData(fieldInfos []*script.FieldInfo, params map[string]interface{}) (map[string]interface{}, error) {
	txData := make(map[string]interface{})
	for _, fitem := range fieldInfos {
		var (
			v     interface{}
			ok    bool
			err   error
			index = fitem.Name
		)

		if _, ok := params[index]; !ok {
			if fitem.ContainsTag(script.TagOptional) {
				txData[index] = getFieldDefaultValue(fitem.Original)
				continue
			}
			return nil, fmt.Errorf(eParamNotFound, index)
		}

		switch fitem.Original {
		case script.DtBool:
			if v, ok = params[index].(bool); !ok {
				err = fmt.Errorf("invalid bool type")
				break
			}
		case script.DtFloat:
			switch val := params[index].(type) {
			case float64:
				v = val
			case uint64:
				v = float64(val)
			case int64:
				v = float64(val)
			default:
				err = fmt.Errorf("invalid float type")
				break
			}
		case script.DtInt, script.DtAddress:
			switch t := params[index].(type) {
			case int64:
				v = t
			case uint64:
				v = int64(t)
			default:
				err = fmt.Errorf("invalid int type")
			}
		case script.DtMoney:
			var s string
			if s, ok = params[index].(string); !ok {
				err = fmt.Errorf("invalid money type")
				break
			}
			v, err = decimal.NewFromString(s)
			if err != nil {
				break
			}
			if v.(decimal.Decimal).LessThan(decimal.New(1, 0)) ||
				v.(decimal.Decimal).
					Mod(decimal.New(1, 0)).
					GreaterThan(decimal.New(0, 0)) {
				err = fmt.Errorf("inconsistent with the smallest reference unit and its integer multiples")
				break
			}
		case script.DtString:
			if v, ok = params[index].(string); !ok {
				err = fmt.Errorf("invalid string type")
				break
			}
		case script.DtBytes:
			if v, ok = params[index].([]byte); !ok {
				err = fmt.Errorf("invalid bytes type")
				break
			}
		case script.DtArray:
			if v, ok = params[index].([]interface{}); !ok {
				err = fmt.Errorf("invalid array type")
				break
			}
			for i, subv := range v.([]interface{}) {
				switch val := subv.(type) {
				case map[interface{}]interface{}:
					imap := make(map[string]interface{})
					for ikey, ival := range val {
						imap[fmt.Sprint(ikey)] = ival
					}
					v.([]interface{})[i] = types.LoadMap(imap)
				}
			}
		case script.DtMap:
			var val map[interface{}]interface{}
			if val, ok = params[index].(map[interface{}]interface{}); !ok {
				err = fmt.Errorf("invalid map type")
				break
			}
			imap := make(map[string]interface{})
			for ikey, ival := range val {
				imap[fmt.Sprint(ikey)] = ival
			}
			v = types.LoadMap(imap)
		case script.DtFile:
			var val map[string]interface{}
			if val, ok = params[index].(map[string]interface{}); !ok {
				err = fmt.Errorf("invalid file type")
				break
			}

			if v, ok = types.NewFileFromMap(val); !ok {
				err = fmt.Errorf("invalid attrs of file")
				break
			}
		}
		if err != nil {
			return nil, fmt.Errorf("invalid param '%s': %w", index, err)
		}

		if _, ok = txData[fitem.Name]; !ok {
			txData[fitem.Name] = v
		}
	}

	if len(txData) != len(fieldInfos) {
		return nil, fmt.Errorf("invalid number of parameters")
	}

	return txData, nil
}

func getFieldDefaultValue(fieldType uint32) interface{} {
	switch fieldType {
	case script.DtBool:
		return false
	case script.DtFloat:
		return float64(0)
	case script.DtInt, script.DtAddress:
		return int64(0)
	case script.DtMoney:
		return decimal.New(0, consts.MoneyDigits)
	case script.DtString:
		return ""
	case script.DtBytes:
		return []byte{}
	case script.DtArray:
		return []interface{}{}
	case script.DtMap:
		return types.NewMap()
	case script.DtFile:
		return types.NewFile()
	}
	return nil
}
