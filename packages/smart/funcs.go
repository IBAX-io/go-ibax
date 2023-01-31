/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/sha3"

	"github.com/IBAX-io/go-ibax/packages/clbmanager"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/common/crypto/hashalgo"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/scheduler"
	"github.com/IBAX-io/go-ibax/packages/scheduler/contract"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	qb "github.com/IBAX-io/go-ibax/packages/storage/sqldb/queryBuilder"
	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

const (
	nodeBanNotificationHeader = "Your node was banned"
	historyLimit              = 250
	contractTxType            = 128
)

var (
	ErrNotImplementedOnCLB = errors.New("Contract not implemented on CLB")
)

var BOM = []byte{0xEF, 0xBB, 0xBF}

type permTable struct {
	Insert    string `json:"insert"`
	Update    string `json:"update"`
	NewColumn string `json:"new_column"`
	Read      string `json:"read,omitempty"`
	Filter    string `json:"filter,omitempty"`
}

type permColumn struct {
	Update string `json:"update"`
	Read   string `json:"read,omitempty"`
}

type TxInfo struct {
	Block    string         `json:"block,omitempty"`
	Contract string         `json:"contract,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
}

type TableInfo struct {
	Columns map[string]string
	Table   *sqldb.Table
}

type FlushInfo struct {
	ID   uint32            // id
	Prev *script.CodeBlock // previous item, nil if the new item has been appended
	Info *script.ObjInfo
	Name string // the name
}

func (finfo *FlushInfo) FlushVM() {
	if finfo.Prev == nil {
		if finfo.ID != uint32(len(script.GetVM().Children)-1) {
			//logger.WithFields(log.Fields{"type": consts.ContractError, "value": finfo.ID, "len": len(GetVM().Children) - 1}).Error("flush rollback")
		} else {
			script.GetVM().Children = script.GetVM().Children[:len(script.GetVM().Children)-1]
			delete(script.GetVM().Objects, finfo.Name)
		}
	} else {
		script.GetVM().Children[finfo.ID] = finfo.Prev
		script.GetVM().Objects[finfo.Name] = finfo.Info
	}
}

// NotifyInfo is used for sending delayed notifications
type NotifyInfo struct {
	Roles       bool // if true then UpdateRolesNotifications, otherwise UpdateNotifications
	EcosystemID int64
	List        []string
}

var (
	funcCallsDBP = map[string]struct{}{
		"DBInsert":              {},
		"DBUpdate":              {},
		"DBUpdatePlatformParam": {},
		"DBUpdateExt":           {},
		"DBSelect":              {},
	}
	writeFuncs = map[string]struct{}{
		"CreateColumn":          {},
		"CreateTable":           {},
		"DBInsert":              {},
		"DBUpdate":              {},
		"DBUpdatePlatformParam": {},
		"DBUpdateExt":           {},
		"CreateEcosystem":       {},
		"CreateContract":        {},
		"UpdateContract":        {},
		"CreateLanguage":        {},
		"EditLanguage":          {},
		"BindWallet":            {},
		"UnbindWallet":          {},
		"EditEcosysName":        {},
		"UpdateNodesBan":        {},
		"UpdateCron":            {},
		"CreateCLB":             {},
		"DeleteCLB":             {},
		"DelColumn":             {},
		"DelTable":              {},
	}
	// map for table name to parameter with conditions
	tableParamConditions = map[string]string{
		"pages":      "changing_page",
		"menu":       "changing_menu",
		"contracts":  "changing_contracts",
		"snippets":   "changing_snippets",
		"languages":  "changing_language",
		"tables":     "changing_tables",
		"parameters": "changing_parameters",
		"app_params": "changing_app_params",
	}
	typeToPSQL = map[string]string{
		`json`:      `jsonb`,
		`varchar`:   `varchar(102400)`,
		`character`: `character(1) NOT NULL DEFAULT '0'`,
		`number`:    `bigint NOT NULL DEFAULT '0'`,
		`datetime`:  `timestamp`,
		`double`:    `double precision`,
		`money`:     `decimal (30, 0) NOT NULL DEFAULT '0'`,
		`text`:      `text`,
		`bytea`:     `bytea NOT NULL DEFAULT '\x'`,
	}
)

// EmbedFuncs is extending vm with embedded functions
func EmbedFuncs(vt script.VMType) map[string]any {
	f := map[string]any{
		"AddressToId":                  converter.AddressToID,
		"ColumnCondition":              ColumnCondition,
		"Contains":                     strings.Contains,
		"ContractAccess":               ContractAccess,
		"RoleAccess":                   RoleAccess,
		"ContractConditions":           ContractConditions,
		"ContractName":                 contractName,
		"ValidateEditContractNewValue": ValidateEditContractNewValue,
		"CreateColumn":                 CreateColumn,
		"CreateTable":                  CreateTable,
		"DBInsert":                     DBInsert,
		"DBSelect":                     DBSelect,
		"DBUpdate":                     DBUpdate,
		"DBUpdatePlatformParam":        UpdatePlatformParam,
		"DBUpdateExt":                  DBUpdateExt,
		"EcosysParam":                  EcosysParam,
		"AppParam":                     AppParam,
		"SysParamString":               SysParamString,
		"SysParamInt":                  SysParamInt,
		"SysFuel":                      SysFuel,
		"Eval":                         Eval,
		"EvalCondition":                EvalCondition,
		"Float":                        Float,
		"GetContractByName":            GetContractByName,
		"GetContractById":              GetContractById,
		"HMac":                         HMac,
		"Join":                         Join,
		"JSONDecode":                   JSONDecode,
		"JSONEncode":                   JSONEncode,
		"JSONEncodeIndent":             JSONEncodeIndent,
		"IdToAddress":                  converter.IDToAddress,
		"Int":                          Int,
		"Len":                          Len,
		"Money":                        Money,
		"FormatMoney":                  FormatMoney,
		"PermColumn":                   PermColumn,
		"PermTable":                    PermTable,
		"Random":                       RandomInt,
		"RandomFloat":                  RandomFloat,
		"RandomDecimal":                RandomDecimal,
		"Split":                        Split,
		"Str":                          Str,
		"Substr":                       Substr,
		"Replace":                      Replace,
		"Size":                         Size,
		"PubToID":                      PubToID,
		"HexToBytes":                   HexToBytes,
		"LangRes":                      LangRes,
		"HasPrefix":                    strings.HasPrefix,
		"ValidateCondition":            ValidateCondition,
		"TrimSpace":                    strings.TrimSpace,
		"ToLower":                      strings.ToLower,
		"ToUpper":                      strings.ToUpper,
		"CreateEcosystem":              CreateEcosystem,
		"CreateContract":               CreateContract,
		"UpdateContract":               UpdateContract,
		"TableConditions":              TableConditions,
		"CreateLanguage":               CreateLanguage,
		"EditLanguage":                 EditLanguage,
		"BndWallet":                    BndWallet,
		"UnbndWallet":                  UnbndWallet,
		"RowConditions":                RowConditions,
		"DecodeBase64":                 DecodeBase64,
		"EncodeBase64":                 EncodeBase64,
		"Hash":                         Hash,
		"DoubleHash":                   crypto.DoubleHash,
		"EditEcosysName":               EditEcosysName,
		"GetColumnType":                GetColumnType,
		"GetType":                      GetType,
		"AllowChangeCondition":         AllowChangeCondition,
		"StringToBytes":                StringToBytes,
		"BytesToString":                BytesToString,
		"GetMapKeys":                   GetMapKeys,
		"SortedKeys":                   SortedKeys,
		"Append":                       Append,
		"EthereumAddress":              EthereumAddress,
		"GetLogTxCount":                GetLogTxCount,
		"GetHistory":                   GetHistory,
		"GetHistoryRow":                GetHistoryRow,
		"GetDataFromXLSX":              GetDataFromXLSX,
		"GetRowsCountXLSX":             GetRowsCountXLSX,
		"BlockTime":                    BlockTime,
		"IsObject":                     IsObject,
		"DateTime":                     DateTime,
		"UnixDateTime":                 UnixDateTime,
		"DateTimeLocation":             DateTimeLocation,
		"UnixDateTimeLocation":         UnixDateTimeLocation,
		"UpdateNotifications":          UpdateNotifications,
		"UpdateRolesNotifications":     UpdateRolesNotifications,
		"DelTable":                     DelTable,
		"DelColumn":                    DelColumn,
		"HexToPub":                     crypto.HexToPub,
		"PubToHex":                     PubToHex,
		"UpdateNodesBan":               UpdateNodesBan,
		"Log":                          Log,
		"Log10":                        Log10,
		"Pow":                          Pow,
		"Sqrt":                         Sqrt,
		"Round":                        Round,
		"Floor":                        Floor,
		"CheckCondition":               CheckCondition,
		"IsHonorNodeKey":               IsHonorNodeKey,
		"CheckSign":                    CheckSign,
		"CheckNumberChars":             CheckNumberChars,
		"DateFormat":                   Date,
		"RegexpMatch":                  RegexpMatch,
		"DBCount":                      DBCount,
		"MathMod":                      MathMod,
		"MathModDecimal":               MathModDecimal,
		"CreateView":                   CreateView,
	}
	switch vt {
	case script.VMType_CLB:
		f["HTTPRequest"] = HTTPRequest
		f["Date"] = Date
		f["HTTPPostJSON"] = HTTPPostJSON
		f["ValidateCron"] = ValidateCron
		f["UpdateCron"] = UpdateCron
	case script.VMType_CLBMaster:
		f["HTTPRequest"] = HTTPRequest
		f["Date"] = Date
		f["HTTPPostJSON"] = HTTPPostJSON
		f["ValidateCron"] = ValidateCron
		f["UpdateCron"] = UpdateCron
		f["CreateCLB"] = CreateCLB
		f["DeleteCLB"] = DeleteCLB
		f["StartCLB"] = StartCLB
		f["StopCLBProcess"] = StopCLBProcess
		f["GetCLBList"] = GetCLBList
	case script.VMType_Smart:
		f["GetBlock"] = GetBlock
	}
	return f
}

func accessContracts(sc *SmartContract, names ...string) bool {
	contract := sc.TxContract.StackCont[len(sc.TxContract.StackCont)-1].(string)

	for _, item := range names {
		if contract == `@1`+item {
			return true
		}
	}
	return false
}

// CompileContract is compiling contract
func CompileContract(sc *SmartContract, code string, state, id, token int64) (any, error) {
	if err := validateAccess(sc, "CompileContract"); err != nil {
		return nil, err
	}
	return sc.VM.CompileBlock([]rune(code), &script.OwnerInfo{StateID: uint32(state), WalletID: id, TokenID: token})
}

// ContractAccess checks whether the name of the executable contract matches one of the names listed in the parameters.
func ContractAccess(sc *SmartContract, names ...any) bool {
	if conf.Config.FuncBench {
		return true
	}

	for _, iname := range names {
		switch name := iname.(type) {
		case string:
			if len(name) > 0 {
				if name[0] != '@' {
					name = fmt.Sprintf(`@%d`, sc.TxSmart.EcosystemID) + name
				}
				for i := len(sc.TxContract.StackCont) - 1; i >= 0; i-- {
					contName := sc.TxContract.StackCont[i].(string)
					if strings.HasPrefix(contName, `@`) {
						if contName == name {
							return true
						}
						break
					}
				}
			}
		}
	}
	return false
}

// RoleAccess checks whether the name of the role matches one of the names listed in the parameters.
func RoleAccess(sc *SmartContract, ids ...any) (bool, error) {
	rolesList, err := sqldb.GetMemberRoles(sc.DbTransaction, sc.TxSmart.EcosystemID, sc.Key.AccountID)
	if err != nil {
		return false, err
	}

	rolesIndex := make(map[int64]bool)
	for _, id := range rolesList {
		rolesIndex[id] = true
	}

	for _, id := range ids {
		switch v := id.(type) {
		case int64:
			if rolesIndex[v] {
				return true, nil
			}
			break
		}
	}
	return false, nil
}

// ContractConditions calls the 'conditions' function for each of the contracts specified in the parameters
func ContractConditions(sc *SmartContract, names ...any) (bool, error) {
	for _, iname := range names {
		name := iname.(string)
		if len(name) > 0 {
			getContract := VMGetContract(sc.VM, name, uint32(sc.TxSmart.EcosystemID))
			if getContract == nil {
				getContract = VMGetContract(sc.VM, name, 0)
				if getContract == nil {
					return false, logErrorfShort(eUnknownContract, name, consts.NotFound)
				}
			}
			block := getContract.GetFunc(`conditions`)
			if block == nil {
				return true, nil
			}
			vars := sc.getExtend()
			//if sc.TxContract == nil {
			//	sc.TxContract = getContract
			//	sc.TxContract.Extend = vars
			//}
			if err := sc.AppendStack(name); err != nil {
				return false, err
			}
			_, err := script.VMRun(sc.VM, block, []any{}, vars, sc.Hash)
			if err != nil {
				return false, err
			}
			sc.PopStack(name)
		} else {
			return false, logError(errEmptyContract, consts.EmptyObject, "ContractConditions")
		}
	}
	return true, nil
}

func contractName(value string) (name string, err error) {
	var list []string

	list, err = script.ContractsList(value)
	if err == nil && len(list) > 0 {
		name = list[0]
	}
	return
}

func ValidateEditContractNewValue(sc *SmartContract, newValue, oldValue string) error {
	list, err := script.ContractsList(newValue)
	if err != nil {
		return err
	}
	curlist, err := script.ContractsList(oldValue)
	if err != nil {
		return err
	}
	if len(list) != len(curlist) {
		return errContractChange
	}
	for i := 0; i < len(list); i++ {
		var ok bool
		for j := 0; j < len(curlist); j++ {
			if curlist[j] == list[i] {
				ok = true
				break
			}
		}
		if !ok {
			return errNameChange
		}
	}
	return nil
}

func UpdateContract(sc *SmartContract, id int64, value, conditions string, recipient int64, tokenID string) error {
	if err := validateAccess(sc, "UpdateContract"); err != nil {
		return err
	}
	pars := make(map[string]any)
	ecosystemID := sc.TxSmart.EcosystemID
	var root any
	if len(value) > 0 {
		var err error
		root, err = CompileContract(sc, value, ecosystemID, recipient, converter.StrToInt64(tokenID))
		if err != nil {
			return err
		}
		pars["value"] = value
	}
	if len(conditions) > 0 {
		pars["conditions"] = conditions
	}

	if len(pars) > 0 {
		if !sc.CLB {
			if err := SysRollback(sc, SysRollData{Type: "EditContract", ID: id}); err != nil {
				return err
			}
		}
		if _, err := DBUpdate(sc, "@1contracts", id, types.LoadMap(pars)); err != nil {
			return err
		}
	}
	if len(value) > 0 {
		if err := FlushContract(sc, root, id); err != nil {
			return err
		}
	}
	return nil
}

func CreateContract(sc *SmartContract, name, value, conditions string, tokenEcosystem, appID int64) (int64, error) {
	if err := validateAccess(sc, "CreateContract"); err != nil {
		return 0, err
	}
	var id int64
	var err error
	isExists := GetContractByName(sc, name)
	if isExists != 0 {
		log.WithFields(log.Fields{"type": consts.ContractError, "name": name,
			"tableId": isExists}).Error("create existing contract")
		return 0, fmt.Errorf(eContractExist, script.StateName(uint32(sc.TxSmart.EcosystemID), name))
	}
	root, err := CompileContract(sc, value, sc.TxSmart.EcosystemID, 0, tokenEcosystem)
	if err != nil {
		return 0, err
	}
	_, id, err = DBInsert(sc, "@1contracts", types.LoadMap(map[string]any{
		"name":       name,
		"value":      value,
		"conditions": conditions,
		"wallet_id":  0,
		"token_id":   tokenEcosystem,
		"app_id":     appID,
		"ecosystem":  sc.TxSmart.EcosystemID,
	}))
	if err != nil {
		return 0, err
	}
	if err = FlushContract(sc, root, id); err != nil {
		return 0, err
	}
	if !sc.CLB {
		err = SysRollback(sc, SysRollData{Type: "NewContract", Data: value})
		if err != nil {
			return 0, err
		}
	}
	return id, nil
}

func getColumns(columns string) (colsSQL string, colout []byte, err error) {
	var (
		sqlColType string
		cols       []any
		out        []byte
	)
	if err = unmarshalJSON([]byte(columns), &cols, "columns from json"); err != nil {
		return
	}
	colperm := make(map[string]string)
	colList := make(map[string]bool)
	for _, icol := range cols {
		var data map[string]any
		switch v := icol.(type) {
		case string:
			if err = unmarshalJSON([]byte(v), &data, `columns permissions from json`); err != nil {
				return
			}
		default:
			data = v.(map[string]any)
		}
		colname := converter.EscapeSQL(strings.ToLower(data[`name`].(string)))
		if err = checkColumnName(colname); err != nil {
			return
		}
		if colList[colname] {
			err = errSameColumns
			return
		}

		sqlColType, err = columnType(data["type"].(string))
		if err != nil {
			return
		}

		colList[colname] = true
		colsSQL += `"` + colname + `" ` + sqlColType + " ,\n"
		condition := ``
		switch v := data[`conditions`].(type) {
		case string:
			condition = v
		case map[string]any:
			out, err = marshalJSON(v, `conditions to json`)
			if err != nil {
				return
			}
			condition = string(out)
		}
		colperm[colname] = condition
	}
	colout, err = marshalJSON(colperm, `columns to json`)
	return
}

// CreateView is creating smart contract view table
func CreateView(sc *SmartContract, vname, columns, where string, applicationID int64) (err error) {
	if err = validateAccess(sc, "CreateView"); err != nil {
		return
	}
	var (
		viewName, tables, wheres, colSQLs string
		colout, whsout                    []byte
	)

	viewName = qb.GetTableName(sc.TxSmart.EcosystemID, vname)
	var check = sqldb.Namer{}
	if check.HasExists(sc.DbTransaction, viewName) {
		return fmt.Errorf(eTableExists, vname)
	}

	colSQLs, colout, err = parseViewColumnSql(sc, columns)
	if err != nil {
		return err
	}
	tables, wheres, whsout, err = parseViewWhereSql(sc, where)
	if err != nil {
		return err
	}
	if err = sqldb.CreateView(sc.DbTransaction, viewName, tables, wheres, colSQLs); err != nil {
		return logErrorDB(err, "creating view table")
	}
	prefix, name := PrefixName(viewName)
	_, _, err = sc.insert([]string{`name`, `columns`, `wheres`, `app_id`,
		`ecosystem`}, []any{name, string(colout), string(whsout),
		applicationID, prefix}, `1_views`)
	if err != nil {
		return logErrorDB(err, "insert table info")
	}
	if !sc.CLB {
		if err = syspar.SysTableColType(sc.DbTransaction); err != nil {
			return logErrorDB(err, "updating sys table col type")
		}
		if err = SysRollback(sc, SysRollData{Type: "NewView", TableName: viewName}); err != nil {
			return err
		}
	}
	return
}

type ViewColSch struct {
	Table string `json:"table,omitempty"`
	Col   string `json:"col,omitempty"`
	Alias string `json:"alias,omitempty"`
}

func parseViewColumnSql(sc *SmartContract, columns string) (colsSQL string, colout []byte, err error) {
	var (
		cols, outarr []ViewColSch
		colList      = make(map[string]bool)
		has          = make(map[string]bool)
	)
	if err = unmarshalJSON([]byte(columns), &cols, "columns from json"); err != nil {
		return
	}
	if len(cols) == 0 {
		err = errors.New("columns is empty")
		return
	}
	for i, icol := range cols {
		var c ViewColSch
		tableName := converter.ParseTable(icol.Table, sc.TxSmart.EcosystemID)
		if !has[tableName] {
			if !sc.DbTransaction.HasTableOrView(tableName) {
				err = fmt.Errorf(eTableNotFound, tableName)
				return
			}
			has[tableName] = true
		}
		colname := converter.EscapeSQL(strings.ToLower(icol.Col))
		if err = checkColumnName(colname); err != nil {
			return
		}
		if colList[colname] {
			err = fmt.Errorf("column %s specified more than once", colname)
			return
		}
		c.Col = colname
		alias := converter.EscapeSQL(strings.ToLower(icol.Alias))
		if len(alias) > 0 {
			if err = checkColumnName(alias); err != nil {
				return
			}
			colname = colname + ` AS ` + alias
		}
		colList[colname] = true
		w := `"` + tableName + `".` + colname
		if len(cols)-1 != i {
			colsSQL += w + ",\n"
		} else {
			colsSQL += w
		}
		c.Table = tableName
		c.Alias = alias
		outarr = append(outarr, c)
	}
	colout, err = marshalJSON(outarr, `view columns to json`)
	return
}

type ViewWheSch struct {
	TableOne string `json:"table1,omitempty"`
	TableTwo string `json:"table2,omitempty"`
	ColOne   string `json:"col1,omitempty"`
	ColTwo   string `json:"col2,omitempty"`
	Compare  string `json:"compare,omitempty"`
}

func parseViewWhereSql(sc *SmartContract, columns string) (tabsSQL, whsSQL string, whsout []byte, err error) {
	var (
		cols, outarr []ViewWheSch
		tabList      = make(map[string]bool)
		tableArr     = make([]string, 0)
		has          = make(map[string]bool)
	)

	if err = unmarshalJSON([]byte(columns), &cols, "columns from json"); err != nil {
		return
	}
	if len(cols) == 0 {
		err = errors.New("columns is empty")
		return
	}
	removeRepeated := func(arr []string) (newArr []string) {
		newArr = make([]string, 0)
		sort.Strings(arr)
		for i := 0; i < len(arr); i++ {
			repeat := false
			for j := i + 1; j < len(arr); j++ {
				if arr[i] == arr[j] {
					repeat = true
					break
				}
			}
			if !repeat {
				newArr = append(newArr, arr[i])
			}
		}
		return newArr
	}
	for i, icol := range cols {
		tableName1 := converter.ParseTable(icol.TableOne, sc.TxSmart.EcosystemID)
		if !has[tableName1] {
			if !sc.DbTransaction.HasTableOrView(tableName1) {
				err = fmt.Errorf(eTableNotFound, tableName1)
				return
			}
			has[tableName1] = true
		}
		tableArr = append(tableArr, tableName1)
		tableName2 := converter.ParseTable(icol.TableTwo, sc.TxSmart.EcosystemID)
		if !has[tableName2] {
			if !sc.DbTransaction.HasTableOrView(tableName2) {
				err = fmt.Errorf(eTableNotFound, tableName2)
				return
			}
			has[tableName2] = true
		}
		tableArr = append(tableArr, tableName2)
		colName1 := converter.EscapeSQL(strings.ToLower(icol.ColOne))
		if err = checkColumnName(colName1); err != nil {
			return
		}
		colName2 := converter.EscapeSQL(strings.ToLower(icol.ColTwo))
		if err = checkColumnName(colName2); err != nil {
			return
		}
		compare := converter.EscapeSQL(strings.ToLower(icol.Compare))
		if len(compare) == 0 {
			err = errors.New("compare operator size is empty")
			return
		}
		w := `"` + tableName1 + `".` + colName1 + ` ` + compare + ` "` + tableName2 + `".` + colName2
		if len(cols)-1 != i {
			whsSQL += w + " AND "
		} else {
			whsSQL += w
		}
		var c ViewWheSch
		c.TableOne = tableName1
		c.TableTwo = tableName2
		c.ColOne = colName1
		c.ColTwo = colName2
		c.Compare = compare
		outarr = append(outarr, c)
	}
	arr := removeRepeated(tableArr)
	for i, tableName := range arr {
		if !tabList[tableName] {
			t := `"` + tableName + `"`
			if len(arr)-1 != i {
				tabsSQL += t + ","
			} else {
				tabsSQL += t
			}
			tabList[tableName] = true
		}
	}
	whsout, err = marshalJSON(outarr, `view wheres to json`)
	return
}

// CreateTable is creating smart contract table
func CreateTable(sc *SmartContract, name, columns, permissions string, applicationID int64) (err error) {
	if err := validateAccess(sc, "CreateTable"); err != nil {
		return err
	}

	if len(name) == 0 {
		return errTableEmptyName
	}

	if !converter.IsLatin(name) {
		return fmt.Errorf(eLatin, name)
	}

	if (name[0] >= '0' && name[0] <= '9') || name[0] == '@' {
		return errTableName
	}

	tableName := qb.GetTableName(sc.TxSmart.EcosystemID, name)
	if sc.DbTransaction.IsTable(tableName) {
		return fmt.Errorf(eTableExists, name)
	}

	colsSQL, colout, err := getColumns(columns)
	if err != nil {
		return err
	}

	if err = sqldb.CreateTable(sc.DbTransaction, tableName, strings.TrimRight(colsSQL, ",\n")); err != nil {
		return logErrorDB(err, "creating tables")
	}

	var perm permTable
	if err = unmarshalJSON([]byte(permissions), &perm, `permissions to json`); err != nil {
		return err
	}
	permout, err := marshalJSON(perm, `permissions to JSON`)
	if err != nil {
		return err
	}
	prefix, name := PrefixName(tableName)

	_, _, err = sc.insert([]string{`name`, `columns`, `permissions`, `conditions`, `app_id`,
		`ecosystem`}, []any{name, string(colout), string(permout),
		`ContractAccess("@1EditTable")`, applicationID, prefix}, `1_tables`)
	if err != nil {
		return logErrorDB(err, "insert table info")
	}
	if !sc.CLB {
		if err = syspar.SysTableColType(sc.DbTransaction); err != nil {
			return logErrorDB(err, "updating sys table col type")
		}
		if err = SysRollback(sc, SysRollData{Type: "NewTable", TableName: tableName}); err != nil {
			return err
		}
	}
	return nil
}

func columnType(colType string) (string, error) {
	if sqlColType, ok := typeToPSQL[colType]; ok {
		return sqlColType, nil
	}
	return ``, fmt.Errorf(eColumnType, colType)
}

func mapToParams(values *types.Map) (params []string, val []any, err error) {
	for _, key := range values.Keys() {
		v, _ := values.Get(key)
		params = append(params, converter.Sanitize(key, ` ->+`))
		val = append(val, v)
	}
	if len(params) == 0 {
		err = fmt.Errorf(`values are undefined`)
	}
	return
}

// DBInsert inserts a record into the specified database table
func DBInsert(sc *SmartContract, tblname string, values *types.Map) (qcost int64, ret int64, err error) {
	if tblname == "platform_parameters" {
		return 0, 0, fmt.Errorf("platform parameters access denied")
	}

	tblname = qb.GetTableName(sc.TxSmart.EcosystemID, tblname)
	if err = sc.AccessTable(tblname, "insert"); err != nil {
		return
	}
	var ind int
	var lastID string
	if ind, err = sc.DbTransaction.NumIndexes(tblname); err != nil {
		err = logErrorDB(err, "num indexes")
		return
	}
	params, val, err := mapToParams(values)
	if err != nil {
		return
	}
	if reflect.TypeOf(val[0]) == reflect.TypeOf([]any{}) {
		val = val[0].([]any)
	}
	qcost, lastID, err = sc.insert(params, val, tblname)
	if ind > 0 {
		qcost *= int64(ind)
	}
	if err == nil {
		ret, _ = strconv.ParseInt(lastID, 10, 64)
	}
	return
}

// PrepareColumns replaces jsonb fields -> in the list of columns for db selecting
// For example, name,doc->title => name,doc::jsonb->>'title' as "doc.title"
func PrepareColumns(columns []string) string {
	colList := make([]string, 0)
	for _, icol := range columns {
		if strings.Contains(icol, `->`) {
			colfield := strings.Split(icol, `->`)
			if len(colfield) == 2 {
				icol = fmt.Sprintf(`%s::jsonb->>'%s' as "%[1]s.%[2]s"`, colfield[0], colfield[1])
			} else {
				icol = fmt.Sprintf(`%s::jsonb#>>'{%s}' as "%[1]s.%[3]s"`, colfield[0],
					strings.Join(colfield[1:], `,`), strings.Join(colfield[1:], `.`))
			}
		} else if !strings.ContainsAny(icol, `:*>"`) {
			icol = `"` + icol + `"`
		}
		colList = append(colList, icol)
	}
	return strings.Join(colList, `,`)
}

// DBSelect returns an array of values of the specified columns when there is selection of data 'offset', 'limit', 'where'
func DBSelect(sc *SmartContract, tblname string, inColumns any, id int64, inOrder any,
	offset, limit int64, inWhere *types.Map, query any, group string, all bool) (int64, []any, error) {

	var (
		err     error
		rows    *sql.Rows
		perm    map[string]string
		columns []string
		order   string
	)
	columns, err = qb.GetColumns(inColumns)
	if err != nil {
		return 0, nil, err
	}
	tblname = qb.GetTableName(sc.TxSmart.EcosystemID, tblname)
	order, err = qb.GetOrder(tblname, inOrder)
	if err != nil {
		return 0, nil, err
	}
	where, err := qb.GetWhere(inWhere)
	if err != nil {
		return 0, nil, err
	}
	if id != 0 {
		where = fmt.Sprintf(`id='%d'`, id)
		limit = 1
	}
	if limit == 0 {
		limit = 25
	}
	if limit < 0 || limit > consts.DBFindLimit {
		limit = consts.DBFindLimit
	}
	perm, err = sc.AccessTablePerm(tblname, `read`)
	if err != nil {
		return 0, nil, err
	}
	if err = sc.AccessColumns(tblname, &columns, false); err != nil {
		return 0, nil, err
	}
	q := sqldb.GetDB(sc.DbTransaction).Table(tblname).Select(PrepareColumns(columns)).Where(where)

	if len(group) != 0 {
		q = q.Group(group)
	} else {
		q = q.Order(order)
	}
	if query != "" {
		q = q.Select(query)
	}
	if all {
		rows, err = q.Rows()
	} else {
		rows, err = q.Offset(int(offset)).Limit(int(limit)).Rows()
	}
	if err != nil {
		return 0, nil, logErrorDB(err, fmt.Sprintf("selecting rows from table %s %s where %s order %s",
			tblname, PrepareColumns(columns), where, order))
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return 0, nil, logErrorDB(err, "getting rows columns")
	}
	values := make([][]byte, len(cols))
	scanArgs := make([]any, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	result := make([]any, 0, 50)
	for rows.Next() {
		err = rows.Scan(scanArgs...)
		if err != nil {
			return 0, nil, logErrorDB(err, "scanning next row")
		}
		row := types.NewMap()
		for i, col := range values {
			var value string
			if col != nil {
				value = string(col)
			}
			row.Set(cols[i], value)
		}
		result = append(result, reflect.ValueOf(row).Interface())
	}
	if perm != nil && len(perm[`filter`]) > 0 {
		fltResult, err := sc.VM.EvalIf(perm[`filter`], uint32(sc.TxSmart.EcosystemID),
			sc.getExtend(),
		)
		if err != nil {
			return 0, nil, err
		}
		if !fltResult {
			log.WithFields(log.Fields{"filter": perm["filter"]}).Error("Access denied")
			return 0, nil, errAccessDenied
		}
	}
	return 0, result, nil
}

// DBUpdateExt updates the record in the specified table. You can specify 'where' query in params and then the values for this query
func DBUpdateExt(sc *SmartContract, tblname string, where *types.Map,
	values *types.Map) (qcost int64, err error) {
	if tblname == "platform_parameters" {
		return 0, fmt.Errorf("platform parameters access denied")
	}
	tblname = qb.GetTableName(sc.TxSmart.EcosystemID, tblname)
	if err = sc.AccessTable(tblname, "update"); err != nil {
		return
	}
	columns, val, err := mapToParams(values)
	if err != nil {
		return
	}
	if err = sc.AccessColumns(tblname, &columns, true); err != nil {
		return
	}
	qcost, _, err = sc.updateWhere(columns, val, tblname, where)
	return
}

// DBUpdate updates the item with the specified id in the table
func DBUpdate(sc *SmartContract, tblname string, id int64, values *types.Map) (qcost int64, err error) {
	return DBUpdateExt(sc, tblname, types.LoadMap(map[string]any{`id`: id}), values)
}

// EcosysParam returns the value of the specified parameter for the ecosystem
func EcosysParam(sc *SmartContract, name string) string {
	sp := &sqldb.StateParameter{}
	sp.SetTablePrefix(converter.Int64ToStr(sc.TxSmart.EcosystemID))
	_, err := sp.Get(nil, name)
	if err != nil {
		return logErrorDB(err, "getting ecosystem param").Error()
	}
	return sp.Value
}

// AppParam returns the value of the specified app parameter for the ecosystem
func AppParam(sc *SmartContract, app int64, name string, ecosystem int64) (string, error) {
	ap := &sqldb.AppParam{}
	ap.SetTablePrefix(converter.Int64ToStr(ecosystem))
	_, err := ap.Get(sc.DbTransaction, app, name)
	if err != nil {
		return ``, logErrorDB(err, "getting app param")
	}
	return ap.Value, nil
}

// Eval evaluates the condition
func Eval(sc *SmartContract, condition string) error {
	if len(condition) == 0 {
		return logErrorShort(errEmptyCond, consts.EmptyObject)
	}
	ret, err := sc.EvalIf(condition)
	if err != nil {
		return logError(err, consts.EvalError, "eval condition")
	}
	if !ret {
		return logErrorShort(errAccessDenied, consts.AccessDenied)
	}
	return nil
}

// CheckCondition evaluates the condition
func CheckCondition(sc *SmartContract, condition string) (bool, error) {
	if len(condition) == 0 {
		return false, nil
	}
	ret, err := sc.EvalIf(condition)
	if err != nil {
		return false, logError(err, consts.EvalError, "eval condition")
	}
	return ret, nil
}

// FlushContract is flushing contract
func FlushContract(sc *SmartContract, iroot any, id int64) error {
	if err := validateAccess(sc, "FlushContract"); err != nil {
		return err
	}
	root := iroot.(*script.CodeBlock)
	if id != 0 {
		if len(root.Children) != 1 || root.Children[0].Type != script.ObjectType_Contract {
			return errOneContract
		}
	}
	for i, item := range root.Children {
		if item.Type == script.ObjectType_Contract {
			root.Children[i].GetContractInfo().Owner.TableID = id
		}
	}
	for key, item := range root.Objects {
		if cur, ok := sc.VM.Objects[key]; ok {
			var id uint32
			switch item.Type {
			case script.ObjectType_Contract:
				id = cur.GetCodeBlock().GetContractInfo().ID
			case script.ObjectType_Func:
				id = cur.GetCodeBlock().GetFuncInfo().ID
			}
			sc.FlushRollback = append(sc.FlushRollback, &FlushInfo{
				ID:   id,
				Prev: sc.VM.Children[id],
				Info: cur,
				Name: key,
			})
		} else {
			sc.FlushRollback = append(sc.FlushRollback, &FlushInfo{
				ID:   uint32(len(sc.VM.Children)),
				Prev: nil,
				Info: nil,
				Name: key,
			})
		}

	}
	sc.VM.FlushBlock(root)
	return nil
}

// IsObject returns true if there is the specified contract
func IsObject(sc *SmartContract, name string, state int64) bool {
	return script.VMObjectExists(sc.VM, name, uint32(state))
}

// Len returns the length of the slice
func Len(in []any) int64 {
	if in == nil {
		return 0
	}
	return int64(len(in))
}

// PermTable is changing permission of table
func PermTable(sc *SmartContract, name, permissions string) error {
	if err := validateAccess(sc, "PermTable"); err != nil {
		return err
	}
	var perm permTable
	if err := unmarshalJSON([]byte(permissions), &perm, `table permissions to json`); err != nil {
		return err
	}
	permout, err := marshalJSON(perm, `permission list to json`)
	if err != nil {
		return err
	}

	name = converter.EscapeSQL(strings.ToLower(name))
	tbl := &sqldb.Table{}
	tbl.SetTablePrefix(converter.Int64ToStr(sc.TxSmart.EcosystemID))
	found, err := tbl.Get(sc.DbTransaction, name)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf(eTableNotFound, name)
	}
	_, _, err = sc.update([]string{`permissions`}, []any{string(permout)},
		`1_tables`, `id`, tbl.ID)
	return err
}

func columnConditions(sc *SmartContract, columns string) (err error) {
	var cols []any
	if err = unmarshalJSON([]byte(columns), &cols, "columns permissions from json"); err != nil {
		return
	}
	if len(cols) == 0 {
		return logErrorShort(errUndefColumns, consts.EmptyObject)
	}
	if len(cols) > syspar.GetMaxColumns() {
		return logErrorfShort(eManyColumns, syspar.GetMaxColumns(), consts.ParameterExceeded)
	}
	for _, icol := range cols {
		var data map[string]any
		switch v := icol.(type) {
		case string:
			if err = unmarshalJSON([]byte(v), &data, `columns permissions from json`); err != nil {
				return err
			}
		default:
			data = v.(map[string]any)
		}
		if data[`name`] == nil || data[`type`] == nil {
			return logErrorShort(errWrongColumn, consts.InvalidObject)
		}
		if len(typeToPSQL[data[`type`].(string)]) == 0 {
			return logErrorShort(errIncorrectType, consts.InvalidObject)
		}
		condition := ``
		switch v := data[`conditions`].(type) {
		case string:
			condition = v
		case map[string]any:
			out, err := marshalJSON(v, `conditions to json`)
			if err != nil {
				return err
			}
			condition = string(out)
		}
		perm, err := getPermColumns(condition)
		if err != nil {
			return logError(err, consts.EmptyObject, "Conditions is empty")
		}
		if len(perm.Update) == 0 {
			return logErrorShort(errConditionEmpty, consts.EmptyObject)
		}
		if err = script.VMCompileEval(sc.VM, perm.Update, uint32(sc.TxSmart.EcosystemID)); err != nil {
			return logError(err, consts.EvalError, "compile update conditions")
		}
		if len(perm.Read) > 0 {
			if err = script.VMCompileEval(sc.VM, perm.Read, uint32(sc.TxSmart.EcosystemID)); err != nil {
				return logError(err, consts.EvalError, "compile read conditions")
			}
		}
	}
	return nil
}

// TableConditions is contract func
func TableConditions(sc *SmartContract, name, columns, permissions string) (err error) {
	isEdit := len(columns) == 0
	name = strings.ToLower(name)
	if err := validateAccess(sc, "TableConditions"); err != nil {
		return err
	}

	prefix := converter.Int64ToStr(sc.TxSmart.EcosystemID)

	t := &sqldb.Table{}
	t.SetTablePrefix(prefix)
	exists, err := t.Get(sc.DbTransaction, name)
	if err != nil {
		return logErrorDB(err, "table exists")
	}
	if isEdit {
		if !exists {
			return logErrorfShort(eTableNotFound, name, consts.NotFound)
		}
	} else if exists {
		return logErrorfShort(eTableExists, name, consts.Found)
	}
	_, err = qb.GetColumns(name)
	if err != nil {
		return err
	}
	var perm permTable
	if err = unmarshalJSON([]byte(permissions), &perm, "permissions from json"); err != nil {
		return err
	}
	v := reflect.ValueOf(perm)
	for i := 0; i < v.NumField(); i++ {
		cond := v.Field(i).Interface().(string)
		name := v.Type().Field(i).Name
		if len(cond) == 0 && name != `Read` && name != `Filter` {
			return logErrorfShort(eEmptyCond, name, consts.EmptyObject)
		}
		if err = script.VMCompileEval(sc.VM, cond, uint32(sc.TxSmart.EcosystemID)); err != nil {
			return logError(err, consts.EvalError, "compile evaluating permissions")
		}
	}

	if isEdit {
		if err = sc.AccessTable(name, `update`); err != nil {
			if err = sc.AccessRights(`changing_tables`, false); err != nil {
				return err
			}
		}
		return nil
	}
	if err := columnConditions(sc, columns); err != nil {
		return err
	}
	if err := sc.AccessRights("new_table", false); err != nil {
		return err
	}

	return nil
}

// ValidateCondition checks if the condition can be compiled
func ValidateCondition(sc *SmartContract, condition string, state int64) error {
	if len(condition) == 0 {
		return logErrorShort(errConditionEmpty, consts.EmptyObject)
	}
	return script.VMCompileEval(sc.VM, condition, uint32(state))
}

// ColumnCondition is contract func
func ColumnCondition(sc *SmartContract, tableName, name, coltype, permissions string) error {
	name = converter.EscapeSQL(strings.ToLower(name))
	tableName = converter.EscapeSQL(strings.ToLower(tableName))
	if err := validateAccess(sc, "ColumnCondition"); err != nil {
		return err
	}

	isExist := accessContracts(sc, nEditColumn)
	tEx := &sqldb.Table{}
	prefix := converter.Int64ToStr(sc.TxSmart.EcosystemID)
	tEx.SetTablePrefix(prefix)

	exists, err := tEx.IsExistsByPermissionsAndTableName(sc.DbTransaction, name, tableName)
	if err != nil {
		return logErrorDB(err, "querying that table is exists by permissions and table name")
	}
	if isExist {
		if !exists {
			return logErrorfShort(eColumnNotExist, name, consts.NotFound)
		}
	} else if exists {
		return logErrorfShort(eColumnExist, name, consts.Found)
	}
	_, err = qb.GetColumns(name)
	if err != nil {
		return err
	}
	if len(permissions) == 0 {
		return logErrorShort(errPermEmpty, consts.EmptyObject)
	}
	perm, err := getPermColumns(permissions)
	if err = script.VMCompileEval(sc.VM, perm.Update, uint32(sc.TxSmart.EcosystemID)); err != nil {
		return err
	}
	if len(perm.Read) > 0 {
		if err = script.VMCompileEval(sc.VM, perm.Read, uint32(sc.TxSmart.EcosystemID)); err != nil {
			return err
		}
	}
	tblName := qb.GetTableName(sc.TxSmart.EcosystemID, tableName)
	if isExist {
		return nil
	}
	count, err := sc.DbTransaction.GetColumnCount(tblName)
	if err != nil {
		return logErrorDB(err, "counting table columns")
	}
	if count >= int64(syspar.GetMaxColumns()) {
		return logErrorfShort(eManyColumns, syspar.GetMaxColumns(), consts.ParameterExceeded)
	}
	if len(typeToPSQL[coltype]) == 0 {
		return logErrorValue(errIncorrectType, consts.InvalidObject, "Unknown column type", coltype)
	}
	return sc.AccessTable(tblName, "new_column")
}

// AllowChangeCondition check access to change condition throught supper contract
func AllowChangeCondition(sc *SmartContract, tblname string) error {
	_, name := converter.ParseName(tblname)
	if param, ok := tableParamConditions[name]; ok {
		return sc.AccessRights(param, false)
	}

	return nil
}

// RowConditions checks conditions for table row by id
func RowConditions(sc *SmartContract, tblname string, id int64, conditionOnly bool) error {
	condition, err := sc.DbTransaction.GetRowConditionsByTableNameAndID(
		qb.GetTableName(sc.TxSmart.EcosystemID, tblname), id)
	if err != nil {
		return logErrorDB(err, "executing row condition query")
	}

	if len(condition) == 0 {
		return logErrorValue(fmt.Errorf(eItemNotFound, id), consts.NotFound,
			"record not found", tblname)
	}

	for _, v := range sc.TxContract.StackCont {
		if v == condition {
			return errRecursion
		}
	}

	if err := Eval(sc, condition); err != nil {
		if err == errAccessDenied && conditionOnly {
			return AllowChangeCondition(sc, tblname)
		}

		return err
	}

	return nil
}

func checkColumnName(name string) error {
	if len(name) == 0 {
		return errEmptyColumn
	} else if name[0] >= '0' && name[0] <= '9' {
		return errWrongColumn
	}
	if !converter.IsLatin(name) {
		return fmt.Errorf(eLatin, name)
	}
	return nil
}

func checkTableNameRule(name string) error {
	if len(name) == 0 {
		return errTableEmptyName
	}
	if (name[0] >= '0' && name[0] <= '9') || name[0] == '@' {
		return errTableName
	}
	if !converter.IsLatin(name) {
		return fmt.Errorf(eLatin, name)
	}
	return nil
}

// CreateColumn is creating column
func CreateColumn(sc *SmartContract, tableName, name, colType, permissions string) (err error) {
	var (
		sqlColType string
		permout    []byte
	)
	if err = validateAccess(sc, "CreateColumn"); err != nil {
		return
	}
	name = converter.EscapeSQL(strings.ToLower(name))
	if err = checkColumnName(name); err != nil {
		return
	}

	tblname := qb.GetTableName(sc.TxSmart.EcosystemID, tableName)

	sqlColType, err = columnType(colType)

	if err != nil {
		return
	}

	err = sc.DbTransaction.AlterTableAddColumn(tblname, name, sqlColType)
	if err != nil {
		return logErrorDB(err, "adding column to the table")
	}

	type cols struct {
		ID      int64
		Columns string
	}
	temp := &cols{}
	err = sqldb.GetDB(sc.DbTransaction).Table(`1_tables`).Where("name = ? and ecosystem = ?",
		strings.ToLower(tableName), sc.TxSmart.EcosystemID).Select("id,columns").Find(temp).Error

	if err != nil {
		return
	}
	var perm map[string]string
	if err = unmarshalJSON([]byte(temp.Columns), &perm, `columns from the table`); err != nil {
		return err
	}
	perm[name] = permissions
	permout, err = marshalJSON(perm, `permissions to json`)
	if err != nil {
		return
	}
	_, _, err = sc.update([]string{`columns`}, []any{string(permout)},
		`1_tables`, `id`, temp.ID)
	if err != nil {
		return err
	}
	if !sc.CLB {
		if err := syspar.SysTableColType(sc.DbTransaction); err != nil {
			return err
		}
		return SysRollback(sc, SysRollData{Type: "NewColumn", TableName: tblname, Data: name})
	}
	return
}

// PermColumn is contract func
func PermColumn(sc *SmartContract, tableName, name, permissions string) error {
	if err := validateAccess(sc, "PermColumn"); err != nil {
		return err
	}
	name = converter.EscapeSQL(strings.ToLower(name))
	tableName = converter.EscapeSQL(strings.ToLower(tableName))
	tables := `1_tables`
	type cols struct {
		Columns string
	}
	temp := &cols{}
	err := sqldb.GetDB(sc.DbTransaction).Table(tables).Where("name = ?", tableName).Select("columns").Find(temp).Error
	if err != nil {
		return logErrorDB(err, "querying columns by table name")
	}
	var perm map[string]string
	if err = unmarshalJSON([]byte(temp.Columns), &perm, `columns from json`); err != nil {
		return err
	}
	perm[name] = permissions
	permout, err := marshalJSON(perm, `column permissions to json`)
	if err != nil {
		return err
	}
	_, _, err = sc.update([]string{`columns`}, []any{string(permout)},
		tables, `name`, tableName)
	return err
}

// HMac returns HMAC hash as raw or hex string
func HMac(key, data string, raw_output bool) (ret string, err error) {
	hash, err := crypto.GetHMAC(key, data)
	if err != nil {
		return ``, logError(err, consts.CryptoError, "getting HMAC")
	}
	if raw_output {
		return string(hash), nil
	}
	return hex.EncodeToString(hash), nil
}

// GetMapKeys returns the array of keys of the map
func GetMapKeys(in *types.Map) []any {
	keys := make([]any, 0, in.Size())
	for _, k := range in.Keys() {
		keys = append(keys, k)
	}
	return keys
}

// SortedKeys returns the sorted array of keys of the map
func SortedKeys(m *types.Map) []any {
	i, sorted := 0, make([]string, m.Size())
	for _, k := range m.Keys() {
		sorted[i] = k
		i++
	}
	sort.Strings(sorted)

	ret := make([]any, len(sorted))
	for k, v := range sorted {
		ret[k] = v
	}
	return ret
}

func httpRequest(req *http.Request, headers map[string]any) (string, error) {
	for key, v := range headers {
		req.Header.Set(key, fmt.Sprint(v))
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return ``, logError(err, consts.NetworkError, "http request")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return ``, logError(err, consts.IOError, "reading http answer")
	}
	if resp.StatusCode != http.StatusOK {
		return ``, logError(fmt.Errorf(`%d %s`, resp.StatusCode, strings.TrimSpace(string(data))),
			consts.NetworkError, "http status code")
	}
	return string(data), nil
}

// HTTPRequest sends http request
func HTTPRequest(requrl, method string, head *types.Map, params *types.Map) (string, error) {

	var ioform io.Reader

	headers := make(map[string]any)
	for _, key := range head.Keys() {
		v, _ := head.Get(key)
		headers[key] = v
	}
	form := &url.Values{}
	for _, key := range params.Keys() {
		v, _ := params.Get(key)
		form.Set(key, fmt.Sprint(v))
	}
	if len(*form) > 0 {
		ioform = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, requrl, ioform)
	if err != nil {
		return ``, logError(err, consts.NetworkError, "new http request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return httpRequest(req, headers)
}

// HTTPPostJSON sends post http request with json
func HTTPPostJSON(requrl string, head *types.Map, json_str string) (string, error) {
	req, err := http.NewRequest("POST", requrl, bytes.NewBuffer([]byte(json_str)))
	if err != nil {
		return ``, logError(err, consts.NetworkError, "new http request")
	}
	headers := make(map[string]any)
	for _, key := range head.Keys() {
		v, _ := head.Get(key)
		headers[key] = v
	}
	return httpRequest(req, headers)
}

// RandomInt returns a random value between min and max
func RandomInt(sc *SmartContract, min int64, max int64) (int64, error) {
	if min < 0 || max < 0 || min >= max {
		return 0, logError(fmt.Errorf(eWrongRandom, min, max), consts.InvalidObject, "getting random")
	}
	return min + sc.Rand.Int63n(max-min), nil
}

func RandomFloat(sc *SmartContract, min float64, max float64) (float64, error) {
	if min < 0 || max < 0 || min >= max {
		return 0, logError(fmt.Errorf(eWrongRandom, min, max), consts.InvalidObject, "getting random")
	}
	return min + sc.Rand.Float64()*(max-min), nil
}

func RandomDecimal(sc *SmartContract, min decimal.Decimal, max decimal.Decimal) (decimal.Decimal, error) {
	if min.LessThan(decimal.Zero) || max.LessThan(decimal.Zero) || min.GreaterThanOrEqual(max) {
		return decimal.Zero, logError(fmt.Errorf(eWrongRandom, min, max), consts.InvalidObject, "getting random")
	}
	return decimal.NewFromFloat(sc.Rand.Float64()).Mul(max.Sub(min)).Add(min).Floor(), nil
}

func ValidateCron(cronSpec string) (err error) {
	_, err = scheduler.Parse(cronSpec)
	return
}

func UpdateCron(sc *SmartContract, id int64) error {
	cronTask := &sqldb.Cron{}
	cronTask.SetTablePrefix(converter.Int64ToStr(sc.TxSmart.EcosystemID))

	ok, err := cronTask.Get(id)
	if err != nil {
		return logErrorDB(err, "get cron record")
	}

	if !ok {
		return nil
	}

	err = scheduler.UpdateTask(&scheduler.Task{
		ID:       cronTask.UID(),
		CronSpec: cronTask.Cron,
		Handler: &contract.ContractHandler{
			Contract: cronTask.Contract,
		},
	})
	return err
}

func UpdateNodesBan(smartContract *SmartContract, timestamp int64) error {
	if conf.Config.IsSupportingCLB() {
		return ErrNotImplementedOnCLB
	}

	now := time.Unix(timestamp, 0)

	badBlocks := &sqldb.BadBlocks{}
	banRequests, err := badBlocks.GetNeedToBanNodes(now, syspar.GetIncorrectBlocksPerDay())
	if err != nil {
		logError(err, consts.DBError, "get nodes need to be banned")
		return err
	}

	honorNodes := syspar.GetNodes()
	var updHonorNodes bool
	for i, honorNode := range honorNodes {
		// Removing ban in case ban time has already passed
		if honorNode.UnbanTime.Unix() > 0 && now.After(honorNode.UnbanTime) {
			honorNode.UnbanTime = time.Unix(0, 0)
			updHonorNodes = true
		}
		nodeKeyID := crypto.Address(honorNode.PublicKey)

		// Setting ban time if we have ban requests for the current node from 51% of all nodes.
		// Ban request is mean that node have added more or equal N(system parameter) of bad blocks
		for _, banReq := range banRequests {
			if banReq.ProducerNodeId == nodeKeyID && banReq.Count >= int64((len(honorNodes)/2)+1) {
				honorNode.UnbanTime = now.Add(syspar.GetNodeBanTime())

				blocks, err := badBlocks.GetNodeBlocks(nodeKeyID, now)
				if err != nil {
					return logErrorDB(err, "getting node bad blocks for removing")
				}

				for _, b := range blocks {
					if _, err := DBUpdate(smartContract, "@1bad_blocks", b.ID,
						types.LoadMap(map[string]any{"deleted": "1"})); err != nil {
						return logErrorValue(err, consts.DBError, "deleting bad block",
							converter.Int64ToStr(b.ID))
					}
				}

				banMessage := fmt.Sprintf(
					"%d/%d nodes voted for ban with %d or more blocks each",
					banReq.Count,
					len(honorNodes),
					syspar.GetIncorrectBlocksPerDay(),
				)

				_, _, err = DBInsert(
					smartContract,
					"@1node_ban_logs",
					types.LoadMap(map[string]any{
						"node_id":   nodeKeyID,
						"banned_at": now.Format(time.RFC3339),
						"ban_time":  int64(syspar.GetNodeBanTime() / time.Millisecond), // in ms
						"reason":    banMessage,
					}))

				if err != nil {
					return logErrorValue(err, consts.DBError, "inserting log to node_ban_log",
						converter.Int64ToStr(banReq.ProducerNodeId))
				}

				_, _, err = DBInsert(
					smartContract,
					"@1notifications",
					types.LoadMap(map[string]any{
						"recipient->member_id": nodeKeyID,
						"notification->type":   sqldb.NotificationTypeSingle,
						"notification->header": nodeBanNotificationHeader,
						"notification->body":   banMessage,
					}))

				if err != nil {
					return logErrorValue(err, consts.DBError, "inserting log to node_ban_log",
						converter.Int64ToStr(banReq.ProducerNodeId))
				}

				updHonorNodes = true
			}
		}

		honorNodes[i] = honorNode
	}

	if updHonorNodes {
		data, err := marshalJSON(honorNodes, `honor nodes`)
		if err != nil {
			return err
		}

		_, err = UpdatePlatformParam(smartContract, syspar.HonorNodes, string(data), "")
		if err != nil {
			return logErrorDB(err, "updating honor nodes")
		}
	}

	return nil
}

func GetBlock(blockID int64) (*types.Map, error) {
	block := sqldb.BlockChain{}
	ok, err := block.Get(blockID)
	if err != nil {
		return nil, logErrorDB(err, "getting block")
	}
	if !ok {
		return nil, nil
	}

	return types.LoadMap(map[string]any{
		"id":     block.ID,
		"time":   block.Time,
		"key_id": block.KeyID,
	}), nil
}

// DecodeBase64 decodes base64 string
func DecodeBase64(input string) (out string, err error) {
	var bin []byte
	bin, err = base64.StdEncoding.DecodeString(input)
	if err == nil {
		out = string(bin)
	}
	return
}

// EncodeBase64 encodes string in base64
func EncodeBase64(input string) (out string) {
	return base64.StdEncoding.EncodeToString([]byte(input))
}

// Hash returns hash sum of data
func Hash(data any) (string, error) {
	var b []byte

	switch v := data.(type) {
	case []uint8:
		b = v
	case string:
		b = []byte(v)
	default:
		return "", logErrorf(eUnsupportedType, v, consts.ConversionError, "converting to bytes")
	}

	hash := crypto.Hash(b)

	return hex.EncodeToString(hash[:]), nil
}

// GetColumnType returns the type of the column
func GetColumnType(sc *SmartContract, tableName, columnName string) (string, error) {
	return sc.DbTransaction.GetColumnType(qb.GetTableName(sc.TxSmart.EcosystemID, tableName), columnName)
}

// GetType returns the name of the type of the value
func GetType(val any) string {
	if val == nil {
		return `nil`
	}
	return reflect.TypeOf(val).String()
}

// StringToBytes converts string to bytes
func StringToBytes(src string) []byte {
	return []byte(src)
}

// BytesToString converts bytes to string
func BytesToString(src []byte) string {
	if bytes.HasPrefix(src, BOM) && utf8.Valid(src[len(BOM):]) {
		return string(src[len(BOM):])
	}
	return string(src)
}

// CreateCLB allow create new CLB throught clbmanager
func CreateCLB(sc *SmartContract, name, dbUser, dbPassword string, port int64) error {
	return clbmanager.Manager.CreateCLB(name, dbUser, dbPassword, int(port))
}

// DeleteCLB delete clb
func DeleteCLB(sc *SmartContract, name string) error {
	return clbmanager.Manager.DeleteCLB(name)
}

// StartCLB run CLB process
func StartCLB(sc *SmartContract, name string) error {
	return clbmanager.Manager.StartCLB(name)
}

// StopCLBProcess stops CLB process
func StopCLBProcess(sc *SmartContract, name string) error {
	return clbmanager.Manager.StopCLB(name)
}

// GetCLBList returns list CLB process with statuses
func GetCLBList(sc *SmartContract) map[string]string {
	list, _ := clbmanager.Manager.ListProcessWithPorts()
	return list
}

func GetHistoryRaw(dbTx *sqldb.DbTransaction, ecosystem int64, tableName string,
	id, idRollback int64) ([]any, error) {
	table := qb.GetTableName(ecosystem, tableName)
	rows, err := sqldb.GetDB(dbTx).Table(table).Where("id=?", id).Rows()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("get current values")
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, errNotFound
	}
	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("get columns")
		return nil, err
	}
	values := make([][]byte, len(columns))
	scanArgs := make([]any, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}
	err = rows.Scan(scanArgs...)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("scan values")
		return nil, err
	}
	var value string
	curVal := types.NewMap()
	for i, col := range values {
		if col == nil {
			value = "NULL"
		} else {
			value = string(col)
		}
		curVal.Set(columns[i], value)
	}
	var rollbackList []any
	rollbackTx := &sqldb.RollbackTx{}
	txs, err := rollbackTx.GetRollbackTxsByTableIDAndTableName(converter.Int64ToStr(id),
		table, historyLimit)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("rollback history")
		return nil, err
	}
	for _, tx := range *txs {
		if len(rollbackList) > 0 {
			prev := rollbackList[len(rollbackList)-1].(*types.Map)
			prev.Set(`block_id`, converter.Int64ToStr(tx.BlockID))
			prev.Set(`id`, converter.Int64ToStr(tx.ID))
			block := sqldb.BlockChain{}
			if ok, err := block.Get(tx.BlockID); ok {
				prev.Set(`block_time`, time.Unix(block.Time, 0).Format(`2006-01-02 15:04:05`))
			} else if err != nil {
				log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting block time")
				return nil, err
			}
			if idRollback == tx.ID {
				return rollbackList[len(rollbackList)-1:], nil
			}
		}
		if tx.Data == "" {
			continue
		}
		rollback := types.NewMap()
		for _, k := range curVal.Keys() {
			v, _ := curVal.Get(k)
			rollback.Set(k, v)
		}
		var updValues map[string]any
		if err := json.Unmarshal([]byte(tx.Data), &updValues); err != nil {
			log.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling rollbackTx.Data from JSON")
			return nil, err
		}
		updMap := types.LoadMap(updValues)
		for _, k := range updMap.Keys() {
			v, _ := updMap.Get(k)
			rollback.Set(k, v)
		}
		rollbackList = append(rollbackList, rollback)
		curVal = rollback
	}
	if len(*txs) > 0 && len((*txs)[len(*txs)-1].Data) > 0 {
		prev := rollbackList[len(rollbackList)-1].(*types.Map)
		prev.Set(`block_id`, `1`)
		prev.Set(`id`, ``)
		prev.Set(`block_time`, ``)
	}
	if idRollback > 0 {
		return []any{}, nil
	}
	return rollbackList, nil
}

const (
	// AddressLength is the expected length of the address
	AddressLength = 20
)

type Address [AddressLength]byte

func (a Address) Hex() string {
	return string(a.checksumHex())
}

func (a *Address) checksumHex() []byte {
	buf := a.hex()

	// compute checksum
	sha := sha3.NewLegacyKeccak256()
	sha.Write(buf[2:])
	hash := sha.Sum(nil)
	for i := 2; i < len(buf); i++ {
		hashByte := hash[(i-2)/2]
		if i%2 == 0 {
			hashByte = hashByte >> 4
		} else {
			hashByte &= 0xf
		}
		if buf[i] > '9' && hashByte > 7 {
			buf[i] -= 32
		}
	}
	return buf[:]
}

func (a Address) hex() []byte {
	var buf [len(a)*2 + 2]byte
	copy(buf[:2], "0x")
	hex.Encode(buf[2:], a[:])
	return buf[:]
}

func EthereumAddress(pub []byte) string {
	var addr Address
	keccak256 := &hashalgo.Keccak256{}
	copy(addr[:], keccak256.GetHash(pub[:])[12:])
	return addr.Hex()
}

func GetLogTxCount(sc *SmartContract, ecosystemID int64) (int64, error) {
	return sqldb.GetLogTxCount(sc.DbTransaction, ecosystemID)
}

func GetHistory(sc *SmartContract, tableName string, id int64) ([]any, error) {
	return GetHistoryRaw(sc.DbTransaction, sc.TxSmart.EcosystemID, tableName, id, 0)
}

func GetHistoryRow(sc *SmartContract, tableName string, id, idRollback int64) (*types.Map,
	error) {
	list, err := GetHistoryRaw(sc.DbTransaction, sc.TxSmart.EcosystemID, tableName, id, idRollback)
	if err != nil {
		return nil, err
	}
	result := types.NewMap()
	if len(list) > 0 {
		for _, key := range list[0].(*types.Map).Keys() {
			val, _ := list[0].(*types.Map).Get(key)
			result.Set(key, val)
		}
	}
	return result, nil
}

func UpdateNotifications(sc *SmartContract, ecosystemID int64, accounts ...any) {
	accountList := make([]string, 0, len(accounts))
	for i, id := range accounts {
		switch v := id.(type) {
		case string:
			accountList = append(accountList, v)
		case []any:
			if i == 0 {
				UpdateNotifications(sc, ecosystemID, v...)
				return
			}
		}
	}
	sc.Notifications.AddAccounts(ecosystemID, accountList...)
}

func UpdateRolesNotifications(sc *SmartContract, ecosystemID int64, roles ...any) {
	rolesList := make([]int64, 0, len(roles))
	for i, roleID := range roles {
		switch v := roleID.(type) {
		case int64:
			rolesList = append(rolesList, v)
		case string:
			rolesList = append(rolesList, converter.StrToInt64(v))
		case []any:
			if i == 0 {
				UpdateRolesNotifications(sc, ecosystemID, v...)
				return
			}
		}
	}
	sc.Notifications.AddRoles(ecosystemID, rolesList...)
}
func DelColumn(sc *SmartContract, tableName, name string) (err error) {
	var (
		count   int64
		permout []byte
	)
	name = converter.EscapeSQL(strings.ToLower(name))
	tblname := qb.GetTableName(sc.TxSmart.EcosystemID, strings.ToLower(tableName))

	t := sqldb.Table{}
	prefix := converter.Int64ToStr(sc.TxSmart.EcosystemID)
	t.SetTablePrefix(prefix)
	TableName := tblname
	if strings.HasPrefix(TableName, prefix) {
		TableName = TableName[len(prefix)+1:]
	}
	if err = sc.AccessTable(tblname, "update"); err != nil {
		return
	}
	found, err := t.Get(sc.DbTransaction, TableName)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting table info")
		return err
	}
	if !found {
		log.WithFields(log.Fields{"type": consts.NotFound, "error": err}).Error("not found table info")
		return fmt.Errorf(eTableNotFound, tblname)
	}
	count, err = sc.DbTransaction.GetRecordsCountTx(tblname, ``)
	if err != nil {
		return
	}
	if count > 0 {
		return fmt.Errorf(eTableNotEmpty, tblname)
	}
	colType, err := sc.DbTransaction.GetColumnType(tblname, name)
	if err != nil {
		return err
	}
	if len(colType) == 0 {
		return fmt.Errorf(eColumnNotExist, name)
	}
	var perm map[string]string
	if err = unmarshalJSON([]byte(t.Columns), &perm, `columns from the table`); err != nil {
		return err
	}
	if _, ok := perm[name]; !ok {
		return fmt.Errorf(eColumnNotDeleted, name)
	}
	delete(perm, name)
	permout, err = marshalJSON(perm, `permissions to json`)
	if err != nil {
		return
	}
	if err = sc.DbTransaction.AlterTableDropColumn(tblname, name); err != nil {
		return
	}
	_, _, err = sc.update([]string{`columns`}, []any{string(permout)},
		`1_tables`, `id`, t.ID)
	if err != nil {
		return err
	}
	if !sc.CLB {
		if err = syspar.SysTableColType(sc.DbTransaction); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating sys table col type")
			return err
		}
		data := map[string]string{"name": name, "type": colType}
		out, err := marshalJSON(data, `marshalling column info`)
		if err != nil {
			return err
		}
		return SysRollback(sc, SysRollData{Type: "DeleteColumn", TableName: tblname,
			Data: string(out)})
	}

	return
}

func DelTable(sc *SmartContract, tableName string) (err error) {
	var (
		count int64
	)
	tblname := qb.GetTableName(sc.TxSmart.EcosystemID, strings.ToLower(tableName))

	t := sqldb.Table{}
	prefix := converter.Int64ToStr(sc.TxSmart.EcosystemID)
	t.SetTablePrefix(prefix)
	TableName := tblname
	if strings.HasPrefix(TableName, prefix) {
		TableName = TableName[len(prefix)+1:]
	}
	if err = sc.AccessTable(tblname, "update"); err != nil {
		return
	}
	found, err := t.Get(sc.DbTransaction, TableName)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting table info")
		return err
	}
	if !found {
		log.WithFields(log.Fields{"type": consts.NotFound, "error": err}).Error("not found table info")
		return fmt.Errorf(eTableNotFound, tblname)
	}

	count, err = sc.DbTransaction.GetRecordsCountTx(tblname, ``)
	if err != nil {
		return
	}
	if count > 0 {
		return fmt.Errorf(eTableNotEmpty, tblname)
	}
	if err = t.Delete(sc.DbTransaction); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting table")
		return err
	}

	if err = sc.DbTransaction.DropTable(tblname); err != nil {
		return
	}
	if !sc.CLB {
		var (
			out []byte
		)
		cols, err := sc.DbTransaction.GetAllColumnTypes(tblname)
		if err != nil {
			return err
		}
		tinfo := TableInfo{Table: &t, Columns: make(map[string]string)}
		for _, item := range cols {
			if item["column_name"] == `id` {
				continue
			}
			tinfo.Columns[item["column_name"]] = sqldb.DataTypeToColumnType(item["data_type"])
		}
		out, err = marshalJSON(tinfo, `marshalling table info`)
		if err != nil {
			return err
		}
		if err = syspar.SysTableColType(sc.DbTransaction); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating sys table col type")
			return err
		}
		return SysRollback(sc, SysRollData{Type: "DeleteTable", TableName: tblname, Data: string(out)})
	}
	return
}

func FormatMoney(sc *SmartContract, exp string, digit int64) (string, error) {
	var cents int64
	if digit != 0 {
		cents = digit
	} else {
		sp := &sqldb.Ecosystem{}
		_, err := sp.Get(sc.DbTransaction, sc.TxSmart.EcosystemID)
		if err != nil {
			return `0`, logErrorDB(err, "getting money_digit param")
		}
		cents = sp.Digits
	}
	return converter.FormatMoney(exp, int32(cents))
}

func PubToHex(in any) (ret string) {
	switch v := in.(type) {
	case string:
		ret = crypto.PubToHex([]byte(v))
	case []byte:
		ret = crypto.PubToHex(v)
	}
	return
}

func SendExternalTransaction(sc *SmartContract, uid, url, externalContract string,
	params *types.Map, resultContract string) (err error) {
	var (
		out, insertQuery string
	)
	if _, err := syspar.GetThisNodePosition(); err != nil {
		return nil
	}

	out, err = JSONEncode(params)
	if err != nil {
		return err
	}
	logger := sc.GetLogger()
	sqlBuilder := &qb.SQLQueryBuilder{
		Entry: logger,
		Table: `external_blockchain`,
		Fields: []string{`value`, `uid`, `url`, `external_contract`,
			`result_contract`, `tx_time`},
		FieldValues: []any{out, uid, url, externalContract,
			resultContract, sc.Timestamp},
		KeyTableChkr: sqldb.KeyTableChecker{},
	}
	insertQuery, err = sqlBuilder.GetSQLInsertQuery(sqldb.NextIDGetter{Tx: sc.DbTransaction})
	if err != nil {
		return
	}
	return sqldb.GetDB(sc.DbTransaction).Exec(insertQuery).Error
}

func IsHonorNodeKey(id int64) bool {
	if syspar.IsCandidateNodeMode() {
		return true
	}
	for _, item := range syspar.GetNodes() {
		if crypto.Address(item.PublicKey) == id {
			return true
		}
	}
	return false
}
