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

	"github.com/IBAX-io/go-ibax/packages/pbgo"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/language"
	"github.com/IBAX-io/go-ibax/packages/migration"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	qb "github.com/IBAX-io/go-ibax/packages/storage/sqldb/queryBuilder"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
	"github.com/IBAX-io/go-ibax/packages/utils/metric"
	"github.com/pkg/errors"
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

// SignRes contains the data of the signature
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

// UpdatePlatformParam updates the system parameter
func UpdatePlatformParam(sc *SmartContract, name, value, conditions string) (int64, error) {
	var (
		fields []string
		values []any
	)
	par := &sqldb.PlatformParameter{}
	found, err := par.Get(sc.DbTransaction, name)
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
		if err := script.VMCompileEval(sc.VM, conditions, 0); err != nil {
			return 0, logErrorValue(err, consts.EvalError, "compiling eval", conditions)
		}
		fields = append(fields, "conditions")
		values = append(values, conditions)
	}
	if len(fields) == 0 {
		return 0, logErrorShort(errEmpty, consts.EmptyObject)
	}
	_, _, err = sc.update(fields, values, "1_platform_parameters", "id", par.ID)
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
func Int(v any) (int64, error) {
	return converter.ValueToInt(v)
}

// Str converts the value to a string
func Str(v any) (ret string) {
	if v == nil {
		return
	}
	return fmt.Sprintf(`%v`, v)
}

// Money converts the value into a numeric type for money
func Money(v any) (decimal.Decimal, error) {
	return script.ValueToDecimal(v)
}

// Float converts the value to float64
func Float(v any) (ret float64) {
	return script.ValueToFloat(v)
}

// Join is joining input with separator
func Join(input []any, sep string) string {
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
func Split(input, sep string) []any {
	out := strings.Split(input, sep)
	result := make([]any, len(out))
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
	return crypto.Verify(pk, []byte(data), s)
}

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
	if _, id, err = DBInsert(sc, `@1languages`, types.LoadMap(map[string]any{"name": name,
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
		types.LoadMap(map[string]any{"name": name, "res": trans})); err != nil {
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

	if err = sqldb.ExecSchemaEcosystem(sc.DbTransaction,
		migration.SqlData{
			Ecosystem: int(id),
			Wallet:    wallet,
			Name:      name,
			Founder:   converter.StrToInt64(sp.Value),
			AppID:     appID,
			Account:   converter.AddressToString(wallet),
		}); err != nil {
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

	if _, _, err = DBInsert(sc, "@1parameters", types.LoadMap(map[string]any{
		"name": "ecosystem_wallet", "value": "0", "conditions": `ContractConditions("DeveloperCondition")`,
		"ecosystem": idStr,
	})); err != nil {
		return 0, logErrorDB(err, "inserting system parameter")
	}

	if _, _, err = DBInsert(sc, "@1applications", types.LoadMap(map[string]any{
		"name":       "System",
		"conditions": `ContractConditions("MainCondition")`,
		"ecosystem":  id,
	})); err != nil {
		return 0, logErrorDB(err, "inserting application")
	}
	if _, _, err = DBInsert(sc, `@1pages`, types.LoadMap(map[string]any{"ecosystem": idStr,
		"name": "default_page", "app_id": appID, "value": SysParamString("default_ecosystem_page"),
		"menu": "default_menu", "conditions": `ContractConditions("DeveloperCondition")`})); err != nil {
		return 0, logErrorDB(err, "inserting default page")
	}
	if _, _, err = DBInsert(sc, `@1menu`, types.LoadMap(map[string]any{"ecosystem": idStr,
		"name": "default_menu", "value": SysParamString("default_ecosystem_menu"), "title": "default", "conditions": `ContractConditions("DeveloperCondition")`})); err != nil {
		return 0, logErrorDB(err, "inserting default menu")
	}

	var (
		ret []any
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
	if _, _, err := DBInsert(sc, `@1keys`, types.LoadMap(map[string]any{
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
	if _, _, err := DBInsert(sc, "@1ecosystems", types.LoadMap(map[string]any{
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
		types.LoadMap(map[string]any{"name": newName}))
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

	if _, _, err := sc.update([]string{"wallet_id"}, []any{sc.TxSmart.KeyID}, "1_contracts", "id", tblid); err != nil {
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

	if _, _, err := sc.update([]string{"wallet_id"}, []any{0}, "1_contracts", "id", tblid); err != nil {
		log.WithFields(log.Fields{"error": err, "contract_id": tblid}).Error("on updating contract wallet")
		return err
	}

	return SetContractWallet(sc, tblid, state, 0)
}

// CheckSignature checks the additional signatures for the contract
func CheckSignature(sc *SmartContract, i map[string]any, name string) error {
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
	hexsign, err := hex.DecodeString(i[`Signature`].(string))
	if len(hexsign) == 0 || err != nil {
		return logError(errWrongSignature, consts.ConversionError, "converting signature to hex")
	}

	var sign TxSignJSON
	if err = unmarshalJSON([]byte(sn.Value), &sign, `unmarshalling sign`); err != nil {
		return err
	}
	wallet := i[`key_id`].(int64)
	forsign := fmt.Sprintf(`%d,%d`, uint64(i[`time`].(int64)), uint64(wallet))
	for _, isign := range sign.Params {
		val := i[isign.Param]
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
func DBSelectMetrics(sc *SmartContract, metric, timeInterval, aggregateFunc string) ([]any, error) {
	if conf.Config.IsSupportingCLB() {
		return nil, ErrNotImplementedOnCLB
	}

	timeBlock := time.Unix(sc.Timestamp, 0).Format(`2006-01-02 15:04:05`)
	result, err := sqldb.GetMetricValues(metric, timeInterval, aggregateFunc, timeBlock)
	if err != nil {
		return nil, logErrorDB(err, "get values of metric")
	}
	return result, nil
}

// DBCollectMetrics returns actual values of all metrics
// This function used to further store these values
func DBCollectMetrics(sc *SmartContract) []any {
	if conf.Config.IsSupportingCLB() {
		return nil
	}

	c := metric.NewCollector(
		metric.CollectMetricDataForEcosystemTables,
		metric.CollectMetricDataForEcosystemTx,
	)
	return c.Values(sc.Timestamp)
}

// JSONDecode converts json string to object
func JSONDecode(input string) (ret any, err error) {
	err = unmarshalJSON([]byte(input), &ret, "unmarshalling json")
	ret = types.ConvertMap(ret)
	return
}

// JSONEncodeIndent converts object to json string
func JSONEncodeIndent(input any, indent string) (string, error) {
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
func JSONEncode(input any) (string, error) {
	return JSONEncodeIndent(input, ``)
}

// Append syn for golang 'append' function
func Append(slice []any, val any) []any {
	return append(slice, val)
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

func MathModDecimal(x, y decimal.Decimal) decimal.Decimal {
	return x.Mod(y)
}
func TransferSelf(sc *SmartContract, value string, source string, target string) (flag bool, err error) {
	fromID := sc.TxSmart.KeyID
	outputsMap := sc.OutputsMap
	txInputsMap := sc.TxInputsMap
	txOutputsMap := sc.TxOutputsMap
	//txHash := sc.Hash
	ecosystem := sc.TxSmart.EcosystemID
	blockId := sc.BlockHeader.BlockId
	//dbTx := sc.DbTransaction
	keyUTXO := sqldb.KeyUTXO{Ecosystem: ecosystem, KeyId: fromID}
	//sum, _ := decimal.NewFromString(value)
	payValue, _ := decimal.NewFromString(value)
	status := pbgo.TxInvokeStatusCode_SUCCESS
	var values *types.Map
	var balance decimal.Decimal
	if strings.EqualFold("UTXO", source) && strings.EqualFold("Account", target) {

		txInputs := sqldb.GetUnusedOutputsMap(keyUTXO, outputsMap)

		if len(txInputs) == 0 {
			return false, fmt.Errorf(eEcoCurrentBalance, converter.IDToAddress(fromID), ecosystem)
		}

		totalAmount := decimal.Zero
		for _, input := range txInputs {
			outputValue, _ := decimal.NewFromString(input.OutputValue)
			totalAmount = totalAmount.Add(outputValue)
		}

		if totalAmount.GreaterThanOrEqual(payValue) && payValue.GreaterThan(decimal.Zero) {
			flag = true // The transfer was successful
			//txOutputs = append(txOutputs, sqldb.SpentInfo{OutputKeyId: toID, OutputValue: value, BlockId: blockId, Ecosystem: ecosystem})
			totalAmount = totalAmount.Sub(payValue)
			if _, _, err := sc.updateWhere([]string{"+amount"}, []any{payValue}, "1_keys", types.LoadMap(map[string]any{"id": fromID, "ecosystem": ecosystem})); err != nil {
				return false, err
			}
			if balance, err = sc.accountBalanceSingle(ecosystem, fromID); err != nil {
				return false, err
			}
		} else {
			return false, fmt.Errorf(eEcoCurrentBalance, converter.IDToAddress(fromID), ecosystem)
		}

		// The change
		var txOutputs []sqldb.SpentInfo
		if totalAmount.GreaterThan(decimal.Zero) {
			txOutputs = append(txOutputs, sqldb.SpentInfo{OutputIndex: 0, OutputKeyId: fromID, OutputValue: totalAmount.String(), BlockId: blockId, Ecosystem: ecosystem, Type: consts.UTXO_Type_Self_UTXO}) // The change
		}
		if len(txInputs) > 0 {
			sqldb.PutAllOutputsMap(txInputs, txInputsMap)
		}
		if len(txOutputs) > 0 {
			sqldb.PutAllOutputsMap(txOutputs, txOutputsMap)
		}
		values = types.LoadMap(map[string]any{
			"sender_id":         fromID,
			"sender_balance":    balance,
			"recipient_id":      fromID,
			"recipient_balance": balance,
			"amount":            payValue,
			"comment":           source,
			"status":            int64(status),
			"block_id":          sc.BlockHeader.BlockId,
			"txhash":            sc.Hash,
			"ecosystem":         ecosystem,
			"type":              int64(GasScenesType_TransferSelf),
			"created_at":        sc.Timestamp,
		})
		_, _, err = sc.insert(values.Keys(), values.Values(), `1_history`)
		if err != nil {
			return false, err
		}
		sc.TxInputsMap = txInputsMap
		sc.TxOutputsMap = txOutputsMap
		return true, nil
	} else if strings.EqualFold("Account", source) && strings.EqualFold("UTXO", target) {

		var totalAmount decimal.Decimal
		var txOutputs []sqldb.SpentInfo
		if totalAmount, err = sc.accountBalanceSingle(ecosystem, fromID); err != nil {
			return false, err
		}
		if totalAmount.GreaterThanOrEqual(payValue) && payValue.GreaterThan(decimal.Zero) {
			flag = true // The transfer was successful
			txOutputs = append(txOutputs, sqldb.SpentInfo{OutputIndex: 0, OutputKeyId: fromID, OutputValue: value, BlockId: blockId, Ecosystem: ecosystem, Type: consts.UTXO_Type_Self_Account})
			totalAmount = totalAmount.Sub(payValue)
			if _, _, err = sc.updateWhere([]string{`-amount`}, []any{payValue}, "1_keys", types.LoadMap(map[string]any{`id`: fromID, `ecosystem`: ecosystem})); err != nil {
				return false, err
			}
			if balance, err = sc.accountBalanceSingle(ecosystem, fromID); err != nil {
				return false, err
			}
		} else {
			return false, fmt.Errorf(eEcoCurrentBalance, converter.IDToAddress(fromID), ecosystem)
		}
		if len(txOutputs) > 0 {
			sqldb.PutAllOutputsMap(txOutputs, txOutputsMap)
		}
		values = types.LoadMap(map[string]any{
			"sender_id":         fromID,
			"sender_balance":    balance,
			"recipient_id":      fromID,
			"recipient_balance": balance,
			"amount":            payValue,
			"comment":           source,
			"status":            int64(status),
			"block_id":          sc.BlockHeader.BlockId,
			"txhash":            sc.Hash,
			"ecosystem":         ecosystem,
			"type":              int64(GasScenesType_TransferSelf),
			"created_at":        sc.Timestamp,
		})
		_, _, err = sc.insert(values.Keys(), values.Values(), `1_history`)
		if err != nil {
			return false, err
		}
		sc.TxInputsMap = txInputsMap
		sc.TxOutputsMap = txOutputsMap
		return true, nil
	}
	return false, errors.New("transfer self fail")
}

func UtxoToken(sc *SmartContract, toID int64, value string) (flag bool, err error) {

	cache := sc.PrevSysPar
	getParams := func(name string) (map[int64]string, error) {
		res := make(map[int64]string)
		if len(cache[name]) > 0 {
			ifuels := make([][]string, 0)
			err = json.Unmarshal([]byte(cache[name]), &ifuels)
			if err != nil {
				log.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling params from json")
				return res, err
			}
			for _, item := range ifuels {
				if len(item) < 2 {
					continue
				}
				res[converter.StrToInt64(item[0])] = item[1]
			}
		}
		return res, nil
	}
	var fuels = make(map[int64]string)
	var wallets = make(map[int64]string)
	var expediteFee decimal.Decimal
	fuels, err = getParams(syspar.FuelRate)
	wallets, err = getParams(syspar.TaxesWallet)

	fromID := sc.TxSmart.KeyID
	outputsMap := sc.OutputsMap
	txInputsMap := sc.TxInputsMap
	txOutputsMap := sc.TxOutputsMap
	comPercents := sc.ComPercents
	//txHash := sc.Hash
	ecosystem := sc.TxSmart.EcosystemID
	blockId := sc.BlockHeader.BlockId
	//dbTx := sc.DbTransaction
	keyUTXO := sqldb.KeyUTXO{Ecosystem: ecosystem, KeyId: fromID}

	txInputs := sqldb.GetUnusedOutputsMap(keyUTXO, outputsMap)
	if len(txInputs) == 0 {
		return false, fmt.Errorf(eEcoCurrentBalance, converter.IDToAddress(fromID), ecosystem)
	}

	if expediteFee, err = expediteFeeBy(sc.TxSmart.Expedite, consts.MoneyDigits); err != nil {
		return false, err
	}
	totalAmount := decimal.Zero

	var txOutputs []sqldb.SpentInfo

	for _, input := range txInputs {
		outputValue, _ := decimal.NewFromString(input.OutputValue)
		totalAmount = totalAmount.Add(outputValue)
	}

	var outputIndex int32 = 0

	// taxes_size = 3
	TaxesSize := syspar.SysInt64(syspar.TaxesSize)

	// if : ecosystem = 2 ,rule : taxes ecosystem 1 and 2
	if ecosystem != consts.DefaultTokenEcosystem {
		// rule : taxes ecosystem 1
		{
			var txOutputs1 []sqldb.SpentInfo
			ecosystem1 := int64(consts.DefaultTokenEcosystem)
			keyUTXO1 := sqldb.KeyUTXO{Ecosystem: ecosystem1, KeyId: fromID}
			txInputs1 := sqldb.GetUnusedOutputsMap(keyUTXO1, outputsMap)
			if len(txInputs1) == 0 {
				return false, fmt.Errorf(eEcoCurrentBalance, converter.IDToAddress(fromID), ecosystem1)
			}
			totalAmount1 := decimal.Zero

			for _, input1 := range txInputs1 {
				outputValue1, _ := decimal.NewFromString(input1.OutputValue)
				totalAmount1 = totalAmount1.Add(outputValue1)
			}
			var money1 = decimal.Zero
			var fuelRate1 = decimal.Zero
			var taxes1 = decimal.Zero
			if ret, ok := fuels[ecosystem1]; ok {

				fuelRate1, err = decimal.NewFromString(ret)
				if err != nil {
					return false, err
				}
				//	ecosystem fuelRate /10 *( bit + len(input))
				money1 = fuelRate1.Div(decimal.NewFromInt(10)).Mul(decimal.NewFromInt(sc.TxSize).Add(decimal.NewFromInt(int64(len(txInputs1)))))
				// utxo ecosystem 1 expediteFee
				money1 = money1.Add(expediteFee)
				if money1.GreaterThan(totalAmount1) {
					money1 = totalAmount1
				}

				taxes1 = money1.Mul(decimal.NewFromInt(TaxesSize)).Div(decimal.New(100, 0)).Floor()

			}
			if money1.GreaterThan(decimal.Zero) && taxes1.GreaterThan(decimal.Zero) {
				if taxesWallet, ok := wallets[ecosystem1]; ok {
					taxesID := converter.StrToInt64(taxesWallet)

					flag = true
					// 97%
					txOutputs1 = append(txOutputs1, sqldb.SpentInfo{OutputIndex: outputIndex, OutputKeyId: sc.BlockHeader.KeyId, OutputValue: money1.Sub(taxes1).String(), BlockId: blockId, Ecosystem: ecosystem1, Type: consts.UTXO_Type_Packaging})
					outputIndex++
					// 3%
					txOutputs1 = append(txOutputs1, sqldb.SpentInfo{OutputIndex: outputIndex, OutputKeyId: taxesID, OutputValue: taxes1.String(), BlockId: blockId, Ecosystem: ecosystem1, Type: consts.UTXO_Type_Taxes})
					outputIndex++
					totalAmount1 = totalAmount1.Sub(money1)
				}
			}

			if totalAmount1.GreaterThan(decimal.Zero) {
				txOutputs1 = append(txOutputs1, sqldb.SpentInfo{OutputIndex: outputIndex, OutputKeyId: fromID, OutputValue: totalAmount1.String(), BlockId: blockId, Ecosystem: ecosystem1, Type: consts.UTXO_Type_Output}) // The change
				outputIndex++
			}

			if len(txInputs1) > 0 && len(txOutputs1) > 0 {
				sqldb.PutAllOutputsMap(txInputs1, txInputsMap)
				sqldb.PutAllOutputsMap(txOutputs1, txOutputsMap)
			}

		}
		// rule : taxes ecosystem 2
		{
			ecosystem2 := ecosystem
			var money2 = decimal.Zero
			var fuelRate2 = decimal.Zero
			var taxes2 = decimal.Zero
			ret, ok := fuels[ecosystem2]
			percent, hasPercent := comPercents[ecosystem2]
			if ok && hasPercent {

				fuelRate2, err = decimal.NewFromString(ret)
				if err != nil {
					return false, err
				}
				//	ecosystem fuelRate /10 *( bit + len(input))
				money2 = fuelRate2.Div(decimal.NewFromInt(10)).Mul(decimal.NewFromInt(sc.TxSize).Add(decimal.NewFromInt(int64(len(txInputs)))))

				if money2.GreaterThan(totalAmount) {
					money2 = totalAmount
				}
				percentMoney2 := decimal.Zero
				if percent > 0 && money2.GreaterThan(decimal.Zero) {
					percentMoney2 = money2.Mul(decimal.NewFromInt(percent)).Div(decimal.New(100, 0)).Floor()
					if percentMoney2.GreaterThan(decimal.Zero) {
						txOutputs = append(txOutputs, sqldb.SpentInfo{OutputIndex: outputIndex, OutputKeyId: 0, OutputValue: percentMoney2.String(), BlockId: blockId, Ecosystem: ecosystem2, Type: consts.UTXO_Type_Combustion})
						outputIndex++
						money2 = money2.Sub(percentMoney2)
						totalAmount = totalAmount.Sub(percentMoney2)
					}
				}
				taxes2 = money2.Mul(decimal.NewFromInt(TaxesSize)).Div(decimal.New(100, 0)).Floor()
			}
			if money2.GreaterThan(decimal.Zero) && taxes2.GreaterThan(decimal.Zero) {
				if taxesWallet, ok := wallets[ecosystem2]; ok {
					taxesID := converter.StrToInt64(taxesWallet)

					flag = true
					// 97%
					txOutputs = append(txOutputs, sqldb.SpentInfo{OutputIndex: outputIndex, OutputKeyId: sc.BlockHeader.KeyId, OutputValue: money2.Sub(taxes2).String(), BlockId: blockId, Ecosystem: ecosystem2, Type: consts.UTXO_Type_Packaging})
					outputIndex++
					// 3%
					txOutputs = append(txOutputs, sqldb.SpentInfo{OutputIndex: outputIndex, OutputKeyId: taxesID, OutputValue: taxes2.String(), BlockId: blockId, Ecosystem: ecosystem2, Type: consts.UTXO_Type_Taxes})
					outputIndex++
					totalAmount = totalAmount.Sub(money2)

				}
			}
		}

	}

	// if : ecosystem = 1 , rule : taxes ecosystem 1
	if ecosystem == consts.DefaultTokenEcosystem {
		ecosystem1 := int64(consts.DefaultTokenEcosystem)
		var money1 = decimal.Zero
		var fuelRate1 = decimal.Zero
		var taxes1 = decimal.Zero
		if ret, ok := fuels[ecosystem1]; ok {

			fuelRate1, err = decimal.NewFromString(ret)
			if err != nil {
				return false, err
			} else {
				//	ecosystem fuelRate /10 *( bit + len(input))
				money1 = fuelRate1.Div(decimal.NewFromInt(10)).Mul(decimal.NewFromInt(sc.TxSize).Add(decimal.NewFromInt(int64(len(txInputs)))))
				// utxo ecosystem 1 expediteFee
				money1 = money1.Add(expediteFee)
				if money1.GreaterThan(totalAmount) {
					money1 = totalAmount
				}
				taxes1 = money1.Mul(decimal.NewFromInt(TaxesSize)).Div(decimal.New(100, 0)).Floor()
			}
		}
		if money1.GreaterThan(decimal.Zero) && taxes1.GreaterThan(decimal.Zero) {
			if taxesWallet, ok := wallets[ecosystem1]; ok {
				taxesID := converter.StrToInt64(taxesWallet)

				flag = true
				// 97%
				txOutputs = append(txOutputs, sqldb.SpentInfo{OutputIndex: outputIndex, OutputKeyId: sc.BlockHeader.KeyId, OutputValue: money1.Sub(taxes1).String(), BlockId: blockId, Ecosystem: ecosystem1, Type: consts.UTXO_Type_Packaging})
				outputIndex++
				// 3%
				txOutputs = append(txOutputs, sqldb.SpentInfo{OutputIndex: outputIndex, OutputKeyId: taxesID, OutputValue: taxes1.String(), BlockId: blockId, Ecosystem: ecosystem1, Type: consts.UTXO_Type_Taxes})
				outputIndex++
				totalAmount = totalAmount.Sub(money1)

			}
		}

	}

	payValue, _ := decimal.NewFromString(value)
	if totalAmount.GreaterThanOrEqual(payValue) && payValue.GreaterThan(decimal.Zero) {
		flag = true // The transfer was successful
		txOutputs = append(txOutputs, sqldb.SpentInfo{OutputIndex: outputIndex, OutputKeyId: toID, OutputValue: value, BlockId: blockId, Ecosystem: ecosystem, Type: consts.UTXO_Type_Transfer})
		outputIndex++
		totalAmount = totalAmount.Sub(payValue)
	} else {
		flag = false
		err = fmt.Errorf(eEcoCurrentBalance, converter.IDToAddress(fromID), ecosystem)
	}

	// The change
	if totalAmount.GreaterThan(decimal.Zero) {
		txOutputs = append(txOutputs, sqldb.SpentInfo{OutputIndex: outputIndex, OutputKeyId: fromID, OutputValue: totalAmount.String(), BlockId: blockId, Ecosystem: ecosystem, Type: consts.UTXO_Type_Output}) // The change
		outputIndex++
	}
	if len(txInputs) > 0 && len(txOutputs) > 0 {
		sqldb.PutAllOutputsMap(txInputs, txInputsMap)
		sqldb.PutAllOutputsMap(txOutputs, txOutputsMap)
	}
	sc.TxInputsMap = txInputsMap
	sc.TxOutputsMap = txOutputsMap
	return flag, err
}
