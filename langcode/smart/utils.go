/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"encoding/json"
	"fmt"

	"ibax.io/store/utils"

	"ibax.io/vm"

	"ibax.io/common/consts"
	"ibax.io/common/types"
	"ibax.io/store"

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
	condition := store.GetAccessExec(utils.ToSnakeCase(funcName))

	if err := Eval(sc, condition); err != nil {
		err = fmt.Errorf(eAccessContract, funcName, condition)
		return logError(err, consts.IncorrectCallingContract, err.Error())
	}

	return nil
}

func FillTxData(fieldInfos []*vm.FieldInfo, params map[string]interface{}) (map[string]interface{}, error) {
	txData := make(map[string]interface{})
	for _, fitem := range fieldInfos {
		var (
			v     interface{}
			ok    bool
			err   error
			index = fitem.Name
		)

		if _, ok := params[index]; !ok {
			if fitem.ContainsTag(vm.TagOptional) {
				txData[index] = getFieldDefaultValue(fitem.Original)
				continue
			}
			return nil, fmt.Errorf(eParamNotFound, index)
		}

		switch fitem.Original {
		case vm.DtBool:
			if v, ok = params[index].(bool); !ok {
				err = fmt.Errorf("Invalid bool type")
				break
			}
		case vm.DtFloat:
			switch val := params[index].(type) {
			case float64:
				v = val
			case uint64:
				v = float64(val)
			case int64:
				v = float64(val)
			default:
				err = fmt.Errorf("Invalid float type")
				break
			}
		case vm.DtInt, vm.DtAddress:
			switch t := params[index].(type) {
			case int64:
				v = t
			case uint64:
				v = int64(t)
			default:
				err = fmt.Errorf("Invalid int type")
			}
		case vm.DtMoney:
			var s string
			if s, ok = params[index].(string); !ok {
				err = fmt.Errorf("Invalid money type")
				break
			}
			v, err = decimal.NewFromString(s)
			if err != nil {
				break
			}
		case vm.DtString:
			if v, ok = params[index].(string); !ok {
				err = fmt.Errorf("Invalid string type")
				break
			}
		case vm.DtBytes:
			if v, ok = params[index].([]byte); !ok {
				err = fmt.Errorf("Invalid bytes type")
				break
			}
		case vm.DtArray:
			if v, ok = params[index].([]interface{}); !ok {
				err = fmt.Errorf("Invalid array type")
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
		case vm.DtMap:
			var val map[interface{}]interface{}
			if val, ok = params[index].(map[interface{}]interface{}); !ok {
				err = fmt.Errorf("Invalid map type")
				break
			}
			imap := make(map[string]interface{})
			for ikey, ival := range val {
				imap[fmt.Sprint(ikey)] = ival
			}
			v = types.LoadMap(imap)
		case vm.DtFile:
			var val map[interface{}]interface{}
			if val, ok = params[index].(map[interface{}]interface{}); !ok {
				err = fmt.Errorf("Invalid file type")
				break
			}

			if v, ok = types.NewFileFromMap(val); !ok {
				err = fmt.Errorf("Invalid attrs of file")
				break
			}
		}
		if err != nil {
			return nil, fmt.Errorf("Invalid param '%s': %s", index, err)
		}

		if _, ok = txData[fitem.Name]; !ok {
			txData[fitem.Name] = v
		}
	}

	if len(txData) != len(fieldInfos) {
		return nil, fmt.Errorf("Invalid number of parameters")
	}

	return txData, nil
}

func getFieldDefaultValue(fieldType uint32) interface{} {
	switch fieldType {
	case vm.DtBool:
		return false
	case vm.DtFloat:
		return float64(0)
	case vm.DtInt, vm.DtAddress:
		return int64(0)
	case vm.DtMoney:
		return decimal.New(0, consts.MoneyDigits)
	case vm.DtString:
		return ""
	case vm.DtBytes:
		return []byte{}
	case vm.DtArray:
		return []interface{}{}
	case vm.DtMap:
		return types.NewMap()
	case vm.DtFile:
		return types.NewFile()
	}
	return nil
}
