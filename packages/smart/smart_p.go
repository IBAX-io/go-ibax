/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/language"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	qb "github.com/IBAX-io/go-ibax/packages/storage/sqldb/queryBuilder"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
	"github.com/IBAX-io/go-ibax/packages/utils/metric"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

const (
	nBindWallet          = "BindWallet"
	nUnbindWallet        = "UnbindWallet"
	nEditColumn          = "EditColumn"
	nEditContract        = "EditContract"
	nEditEcosystemName   = "EditEcosystemName"
	nEditLang            = "EditLang"
	nEditLangJoint       = "EditLangJoint"
	nEditTable           = "EditTable"
	nImport              = "Import"
	nNewColumn           = "NewColumn"
	nNewContract         = "NewContract"
	nNewEcosystem        = "NewEcosystem"
	nNewLang             = "NewLang"
	nNewLangJoint        = "NewLangJoint"
	nNewTable            = "NewTable"
	nNewTableJoint       = "NewTableJoint"
	nNewUser             = "NewUser"
	nBlockReward         = "BlockReward"
	nCallDelayedContract = "CallDelayedContract"
)

//SignRes contains the data of the signature
type SignRes struct {
	Param string `json:"name"`
	Text  string `json:"text"`
}

// TxSignJSON is a structure for additional signs of transaction
type TxSignJSON struct {
	ForSign string    `json:"forsign"`
	Field   string    `json:"field"`
	Title   string    `json:"title"`
	Params  []SignRes `json:"params"`
}

func getCost(name string) int64 {
	if price, ok := syspar.GetPriceExec(utils.ToSnakeCase(name)); ok {
		return price
	}
	return -1
}

// UpdateSysParam updates the system parameter
func UpdateSysParam(sc *SmartContract, name, value, conditions string) (int64, error) {
	var (
		fields []string
		values []interface{}
	)
	par := &sqldb.SystemParameter{}
	found, err := par.Get(name)
	if err != nil {
		return 0, logErrorDB(err, "system parameter get")
	}
	if !found {
		return 0, logErrorf(eParamNotFound, name, consts.NotFound, "system parameter get")
	}
	cond := par.Conditions
	if len(cond) > 0 && !sc.taxes {
		ret, err := sc.EvalIf(cond)
		if err != nil {
			return 0, logError(err, consts.EvalError, "evaluating conditions")
		}
		if !ret {
			return 0, logErrorShort(errAccessDenied, consts.AccessDenied)
		}
	}
	if len(value) > 0 {
		var (
			ok, checked bool
			list        [][]string
		)
		ival := converter.StrToInt64(value)
	check:
		switch name {
		case syspar.GapsBetweenBlocks:
			ok = ival > 0 && ival < 86400
		case syspar.RbBlocks1,
			syspar.NumberNodes:
			ok = ival > 0 && ival < 1000
		case syspar.TaxesSize,
			syspar.PriceCreateRate,
			syspar.PriceTxSize,
			syspar.BlockReward:
			ok = ival >= 0
		case syspar.MaxBlockSize,
			syspar.MaxTxSize,
			syspar.MaxTxCount,
			syspar.MaxColumns,
			syspar.MaxIndexes,
			syspar.MaxBlockUserTx,
			syspar.MaxTxFuel,
			syspar.MaxBlockFuel,
			syspar.MaxForsignSize:
			ok = ival > 0
		case syspar.FuelRate,
			syspar.TaxesWallet:
			if err := unmarshalJSON([]byte(value), &list, `system param`); err != nil {
				return 0, err
			}
			for _, item := range list {
				if len(item) != 2 || converter.StrToInt64(item[0]) <= 0 ||
					(name == syspar.FuelRate && converter.StrToInt64(item[1]) <= 0) ||
					(name == syspar.TaxesWallet && converter.StrToInt64(item[1]) == 0) {
					break check
				}
			}
			checked = true
		case syspar.HonorNodes:
			var fnodes []*syspar.HonorNode
			if err := json.Unmarshal([]byte(value), &fnodes); err != nil {
				break check
			}
			if len(fnodes) > 1 {
				if err = syspar.DuplicateHonorNode(fnodes); err != nil {
					return 0, logErrorValue(err, consts.InvalidObject, err.Error(), value)
				}
			}
			checked = len(fnodes) > 0
		default:
			if strings.HasPrefix(name, `extend_cost_`) || strings.HasSuffix(name, `_price`) {
				ok = ival >= 0
				break
			}
			checked = true
		}
		if !checked && (!ok || converter.Int64ToStr(ival) != value) {
			return 0, logErrorValue(errInvalidValue, consts.InvalidObject, errInvalidValue.Error(),
				value)
		}
		fields = append(fields, "value")
		values = append(values, value)
	}
	if len(conditions) > 0 {
		if err := script.CompileEval(conditions, 0); err != nil {
			return 0, logErrorValue(err, consts.EvalError, "compiling eval", conditions)
		}
		fields = append(fields, "conditions")
		values = append(values, conditions)
	}
	if len(fields) == 0 {
		return 0, logErrorShort(errEmpty, consts.EmptyObject)
	}
	_, _, err = sc.update(fields, values, "1_system_parameters", "id", par.ID)
	if err != nil {
		return 0, err
	}
	err = syspar.SysUpdate(sc.DbTransaction)
	if err != nil {
		return 0, logErrorDB(err, "updating syspar")
	}
	sc.SysUpdate = true
	return 0, nil
}

// SysParamString returns the value of the system parameter
func SysParamString(name string) string {
	return syspar.SysString(name)
}

// SysParamInt returns the value of the system parameter
func SysParamInt(name string) int64 {
	return syspar.SysInt64(name)
}

// SysFuel returns the fuel rate
func SysFuel(state int64) string {
	return syspar.GetFuelRate(state)
}

// Int converts the value to a number
func Int(v interface{}) (int64, error) {
	return converter.ValueToInt(v)
}

// Str converts the value to a string
func Str(v interface{}) (ret string) {
	if v == nil {
		return
	}
	switch val := v.(type) {
	case float64:
		ret = fmt.Sprintf(`%f`, val)
	default:
		ret = fmt.Sprintf(`%v`, val)
	}
	return
}

// Money converts the value into a numeric type for money
func Money(v interface{}) (decimal.Decimal, error) {
	return script.ValueToDecimal(v)
}

func MoneyDiv(d1, d2 interface{}) string {
	val1, _ := script.ValueToDecimal(d1)
	val2, _ := script.ValueToDecimal(d2)
	return val1.Div(val2).Mul(decimal.New(1, 2)).StringFixed(0)
}

// Float converts the value to float64
func Float(v interface{}) (ret float64) {
	return script.ValueToFloat(v)
}

// Join is joining input with separator
func Join(input []interface{}, sep string) string {
	var ret string
	for i, item := range input {
		if i > 0 {
			ret += sep
		}
		ret += fmt.Sprintf(`%v`, item)
	}
	return ret
}

// Split splits the input string to array
func Split(input, sep string) []interface{} {
	out := strings.Split(input, sep)
	result := make([]interface{}, len(out))
	for i, val := range out {
		result[i] = reflect.ValueOf(val).Interface()
	}
	return result
}

// PubToID returns a numeric identifier for the public key specified in the hexadecimal form.
func PubToID(hexkey string) int64 {
	pubkey, err := crypto.HexToPub(hexkey)
	if err != nil {
		logErrorValue(err, consts.CryptoError, "decoding hexkey to string", hexkey)
		return 0
	}
	return crypto.Address(pubkey)
}

// Replace replaces old substrings to new substrings
func CheckSign(pub, data, sign string) (bool, error) {
	pk, err := hex.DecodeString(pub)
	if err != nil {
		return false, err
	}
	s, err := hex.DecodeString(sign)
	if err != nil {
		return false, err
	}
	pk = crypto.CutPub(pk)
	return crypto.CheckSign(pk, []byte(data), s)
}

// Replace replaces old substrings to new substrings
func CheckNumberChars(data string) bool {
	dat := []byte(data)
	dl := len(dat)
	for i := 0; i < dl; i++ {
		d := dat[i]
		if (d >= 0x30 && d <= 0x39) || (d >= 0x41 && d <= 0x5A) || (d >= 0x61 && d <= 0x7A) {
		} else {
			return false
		}
	}
	return true
}

// HexToBytes converts the hexadecimal representation to []byte
func HexToBytes(hexdata string) ([]byte, error) {
	return hex.DecodeString(hexdata)
}

// LangRes returns the language resource
func LangRes(sc *SmartContract, idRes string) string {
	ret, _ := language.LangText(sc.DbTransaction, idRes, int(sc.TxSmart.EcosystemID), sc.TxSmart.Lang)
	return ret
}

// NewLang creates new language
func CreateLanguage(sc *SmartContract, name, trans string) (id int64, err error) {
	if err := validateAccess(sc, "CreateLanguage"); err != nil {
		return 0, err
	}
	idStr := converter.Int64ToStr(sc.TxSmart.EcosystemID)
	if err = language.UpdateLang(int(sc.TxSmart.EcosystemID), name, trans); err != nil {
		return 0, err
	}
	if _, id, err = DBInsert(sc, `@1languages`, types.LoadMap(map[string]interface{}{"name": name,
		"ecosystem": idStr, "res": trans})); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("inserting new language")
		return 0, err
	}
	return id, nil
}

// EditLanguage edits language
func EditLanguage(sc *SmartContract, id int64, name, trans string) error {
	if err := validateAccess(sc, "EditLanguage"); err != nil {
		return err
	}
	if err := language.UpdateLang(int(sc.TxSmart.EcosystemID), name, trans); err != nil {
		return err
	}
	if _, err := DBUpdate(sc, `@1languages`, id,
		types.LoadMap(map[string]interface{}{"name": name, "res": trans})); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("inserting new language")
		return err
	}
	return nil
}

// GetContractByName returns id of the contract with this name
func GetContractByName(sc *SmartContract, name string) int64 {
	contract := VMGetContract(sc.VM, name, uint32(sc.TxSmart.EcosystemID))
	if contract == nil {
		return 0
	}
	info := contract.Info()
	if info == nil {
		return 0
	}

	return info.Owner.TableID
}

// GetContractById returns the name of the contract with this id
func GetContractById(sc *SmartContract, id int64) string {
	_, ret, err := DBSelect(sc, "contracts", "value", id, `id`, 0, 1, nil, "", "", false)
	if err != nil || len(ret) != 1 {
		logErrorDB(err, "getting contract name")
		return ``
	}

	re := regexp.MustCompile(`(?is)^\s*contract\s+([\d\w_]+)\s*{`)
	var val string
	if v, found := ret[0].(*types.Map).Get("value"); found {
		val = v.(string)
	}
	names := re.FindStringSubmatch(val)
	if len(names) != 2 {
		return ``
	}
	return names[1]
}

// EvalCondition gets the condition and check it
func EvalCondition(sc *SmartContract, table, name, condfield string) error {
	tableName := converter.ParseTable(table, sc.TxSmart.EcosystemID)
	query := `SELECT ` + converter.EscapeName(condfield) + ` FROM "` + tableName + `" WHERE name = ? and ecosystem = ?`
	conditions, err := sc.DbTransaction.Single(query, name, sc.TxSmart.EcosystemID).String()
	if err != nil {
		return logErrorDB(err, "executing single query")
	}
	if len(conditions) == 0 {
		return logErrorfShort(eRecordNotFound, name, consts.NotFound)
	}
	return Eval(sc, conditions)
}

// Replace replaces old substrings to new substrings
func Replace(s, old, new string) string {
	return strings.Replace(s, old, new, -1)
}

// CreateEcosystem creates a new ecosystem
func CreateEcosystem(sc *SmartContract, wallet int64, name string) (int64, error) {
	if err := validateAccess(sc, "CreateEcosystem"); err != nil {
		return 0, err
	}

	var sp sqldb.StateParameter
	sp.SetTablePrefix(`1`)
	found, err := sp.Get(sc.DbTransaction, `founder_account`)
	if err != nil {
		return 0, logErrorDB(err, "getting founder")
	}

	if !found || len(sp.Value) == 0 {
		return 0, logErrorShort(errFounderAccount, consts.NotFound)
	}

	id, err := sc.DbTransaction.GetNextID("1_ecosystems")
	if err != nil {
		return 0, logErrorDB(err, "generating next ecosystem id")
	}

	appID, err := sc.DbTransaction.GetNextID("1_applications")
	if err != nil {
		return 0, logErrorDB(err, "generating next application id")
	}

	if err = sqldb.ExecSchemaEcosystem(sc.DbTransaction, int(id), wallet, name, converter.StrToInt64(sp.Value), appID); err != nil {
		return 0, logErrorDB(err, "executing ecosystem schema")
	}

	idStr := converter.Int64ToStr(id)
	if err := LoadContract(sc.DbTransaction, id); err != nil {
		return 0, err
	}
	if !sc.CLB {
		if err := SysRollback(sc, SysRollData{Type: "NewEcosystem", ID: id}); err != nil {
			return 0, err
		}
	}

	sc.FullAccess = true

	if _, _, err = DBInsert(sc, "@1parameters", types.LoadMap(map[string]interface{}{
		"name": "ecosystem_wallet", "value": "0", "conditions": `ContractConditions("AdminCondition")`,
		"ecosystem": idStr,
	})); err != nil {
		return 0, logErrorDB(err, "inserting system parameter")
	}

	if _, _, err = DBInsert(sc, "@1applications", types.LoadMap(map[string]interface{}{
		"name":       "System",
		"conditions": `ContractConditions("MainCondition")`,
		"ecosystem":  id,
	})); err != nil {
		return 0, logErrorDB(err, "inserting application")
	}
	if _, _, err = DBInsert(sc, `@1pages`, types.LoadMap(map[string]interface{}{"ecosystem": idStr,
		"name": "default_page", "app_id": appID, "value": SysParamString("default_ecosystem_page"),
		"menu": "default_menu", "conditions": `ContractConditions("DeveloperCondition")`})); err != nil {
		return 0, logErrorDB(err, "inserting default page")
	}
	if _, _, err = DBInsert(sc, `@1menu`, types.LoadMap(map[string]interface{}{"ecosystem": idStr,
		"name": "default_menu", "value": SysParamString("default_ecosystem_menu"), "title": "default", "conditions": `ContractConditions("DeveloperCondition")`})); err != nil {
		return 0, logErrorDB(err, "inserting default page")
	}

	var (
		ret []interface{}
		pub string
	)
	_, ret, err = DBSelect(sc, "@1keys", "pub", wallet, `id`, 0, 1, nil, "", "", false)
	if err != nil {
		return 0, logErrorDB(err, "getting pub key")
	}

	if Len(ret) > 0 {
		if v, found := ret[0].(*types.Map).Get("pub"); found {
			pub = v.(string)
		}
	}
	if _, _, err := DBInsert(sc, `@1keys`, types.LoadMap(map[string]interface{}{
		"id":        wallet,
		"account":   converter.AddressToString(wallet),
		"pub":       pub,
		"ecosystem": idStr,
	})); err != nil {
		return 0, logErrorDB(err, "inserting key")
	}

	sc.FullAccess = false
	// because of we need to know which ecosystem to rollback.
	// All tables will be deleted so it's no need to rollback data from tables
	if _, _, err := DBInsert(sc, "@1ecosystems", types.LoadMap(map[string]interface{}{
		"id":   id,
		"name": name,
	})); err != nil {
		return 0, logErrorDB(err, "insert new ecosystem to stat table")
	}
	return id, err
}

// EditEcosysName set newName for ecosystem
func EditEcosysName(sc *SmartContract, sysID int64, newName string) error {
	if err := validateAccess(sc, "EditEcosysName"); err != nil {
		return err
	}

	_, err := DBUpdate(sc, "@1ecosystems", sysID,
		types.LoadMap(map[string]interface{}{"name": newName}))
	return err
}

// Size returns the length of the string
func Size(s string) int64 {
	return int64(len(s))
}

// Substr returns the substring of the string
func Substr(s string, off int64, slen int64) string {
	ilen := int64(len(s))
	if off < 0 || slen < 0 || off > ilen {
		return ``
	}
	if off+slen > ilen {
		return s[off:]
	}
	return s[off : off+slen]
}

// BndWallet sets wallet_id to current wallet and updates value in vm
func BndWallet(sc *SmartContract, tblid int64, state int64) error {
	if err := validateAccess(sc, "BindWallet"); err != nil {
		log.Error("BindWallet access denied")
		return err
	}

	if _, _, err := sc.update([]string{"wallet_id"}, []interface{}{sc.TxSmart.KeyID}, "1_contracts", "id", tblid); err != nil {
		log.WithFields(log.Fields{"error": err, "contract_id": tblid}).Error("on updating contract wallet")
		return err
	}

	return SetContractWallet(sc, tblid, state, sc.TxSmart.KeyID)
}

// UnbndWallet sets Active status of the contract in smartVM
func UnbndWallet(sc *SmartContract, tblid int64, state int64) error {
	if err := validateAccess(sc, "UnbindWallet"); err != nil {
		return err
	}

	if _, _, err := sc.update([]string{"wallet_id"}, []interface{}{0}, "1_contracts", "id", tblid); err != nil {
		log.WithFields(log.Fields{"error": err, "contract_id": tblid}).Error("on updating contract wallet")
		return err
	}

	return SetContractWallet(sc, tblid, state, 0)
}

// CheckSignature checks the additional signatures for the contract
func CheckSignature(sc *SmartContract, i *map[string]interface{}, name string) error {
	state, name := converter.ParseName(name)
	sn := sqldb.Signature{}
	sn.SetTablePrefix(converter.Int64ToStr(int64(state)))
	_, err := sn.Get(name)
	if err != nil {
		return logErrorDB(err, "executing single query")
	}
	if len(sn.Value) == 0 {
		return nil
	}
	hexsign, err := hex.DecodeString((*i)[`Signature`].(string))
	if len(hexsign) == 0 || err != nil {
		return logError(errWrongSignature, consts.ConversionError, "converting signature to hex")
	}

	var sign TxSignJSON
	if err = unmarshalJSON([]byte(sn.Value), &sign, `unmarshalling sign`); err != nil {
		return err
	}
	wallet := (*i)[`key_id`].(int64)
	forsign := fmt.Sprintf(`%d,%d`, uint64((*i)[`time`].(int64)), uint64(wallet))
	for _, isign := range sign.Params {
		val := (*i)[isign.Param]
		if val == nil {
			val = ``
		}
		forsign += fmt.Sprintf(`,%v`, val)
	}

	CheckSignResult, err := utils.CheckSign(sc.PublicKeys, []byte(forsign), hexsign, true)
	if err != nil {
		return err
	}
	if !CheckSignResult {
		return logErrorfShort(eIncorrectSignature, forsign, consts.InvalidObject)
	}
	return nil
}

// DBSelectMetrics returns list of metrics by name and time interval
func DBSelectMetrics(sc *SmartContract, metric, timeInterval, aggregateFunc string) ([]interface{}, error) {
	if conf.Config.IsSupportingCLB() {
		return nil, ErrNotImplementedOnCLB
	}

	timeBlock := time.Unix(sc.TxSmart.Time, 0).Format(`2006-01-02 15:04:05`)
	result, err := sqldb.GetMetricValues(metric, timeInterval, aggregateFunc, timeBlock)
	if err != nil {
		return nil, logErrorDB(err, "get values of metric")
	}
	return result, nil
}

// DBCollectMetrics returns actual values of all metrics
// This function used to further store these values
func DBCollectMetrics(sc *SmartContract) []interface{} {
	if conf.Config.IsSupportingCLB() {
		return nil
	}

	c := metric.NewCollector(
		metric.CollectMetricDataForEcosystemTables,
		metric.CollectMetricDataForEcosystemTx,
	)
	return c.Values(sc.TxSmart.Time)
}

// JSONDecode converts json string to object
func JSONDecode(input string) (ret interface{}, err error) {
	err = unmarshalJSON([]byte(input), &ret, "unmarshalling json")
	ret = types.ConvertMap(ret)
	return
}

// JSONEncodeIdent converts object to json string
func JSONEncodeIndent(input interface{}, indent string) (string, error) {
	rv := reflect.ValueOf(input)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Struct && reflect.TypeOf(input).String() != `*types.Map` {
		return "", logErrorfShort(eTypeJSON, input, consts.TypeError)
	}
	var (
		b   []byte
		err error
	)
	if len(indent) == 0 {
		b, err = json.Marshal(input)
	} else {
		b, err = json.MarshalIndent(input, ``, indent)
	}
	if err != nil {
		return ``, logError(err, consts.JSONMarshallError, `marshalling json`)
	}
	out := string(b)
	out = strings.Replace(out, `\u003c`, `<`, -1)
	out = strings.Replace(out, `\u003e`, `>`, -1)
	out = strings.Replace(out, `\u0026`, `&`, -1)

	return out, nil
}

// JSONEncode converts object to json string
func JSONEncode(input interface{}) (string, error) {
	return JSONEncodeIndent(input, ``)
}

// Append syn for golang 'append' function
func Append(slice []interface{}, val interface{}) []interface{} {
	return append(slice, val)
}

func StringToAmount(amount string) decimal.Decimal {
	f, _ := strconv.ParseFloat(amount, 64)
	am, _ := Money(math.Pow10(consts.MoneyDigits) * f)
	return am
}

// RegexpMatch validates regexp
func RegexpMatch(str, reg string) bool {
	if strings.Contains(reg, `\u`) || strings.Contains(reg, `\U`) {
		var err error
		reg, err = strconv.Unquote(`"` + reg + `"`)
		if err != nil {
			return false
		}
	}
	re := regexp.MustCompile(reg)
	return re.MatchString(str)
}

func DBCount(sc *SmartContract, tableName string, inWhere *types.Map) (count int64, err error) {
	tblname := qb.GetTableName(sc.TxSmart.EcosystemID, tableName)
	where, err := qb.GetWhere(inWhere)
	if err != nil {
		return 0, err
	}
	err = sqldb.GetDB(sc.DbTransaction).Table(tblname).Where(where).Count(&count).Error
	return
}

func MathMod(x, y float64) float64 {
	return math.Mod(x, y)
}
