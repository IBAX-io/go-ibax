/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/language"
	"github.com/IBAX-io/go-ibax/packages/publisher"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	qb "github.com/IBAX-io/go-ibax/packages/storage/sqldb/queryBuilder"
	"github.com/IBAX-io/go-ibax/packages/template"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type commonApi struct {
	mode Mode
}

func NewCommonApi(m Mode) *commonApi {
	return &commonApi{
		mode: m,
	}
}

type contractField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Optional bool   `json:"optional"`
}

type GetContractResult struct {
	ID         uint32          `json:"id"`
	StateID    uint32          `json:"state"`
	TableID    string          `json:"tableid"`
	WalletID   string          `json:"walletid"`
	TokenID    string          `json:"tokenid"`
	Address    string          `json:"address"`
	Fields     []contractField `json:"fields"`
	Name       string          `json:"name"`
	AppId      uint32          `json:"app_id"`
	Ecosystem  uint32          `json:"ecosystem"`
	Conditions string          `json:"conditions"`
}

func getContract(r *http.Request, name string) *smart.Contract {
	vm := script.GetVM()
	if vm == nil {
		return nil
	}
	client := getClient(r)
	contract := smart.VMGetContract(vm, name, uint32(client.EcosystemID))
	if contract == nil {
		return nil
	}
	return contract
}

func getContractInfo(contract *smart.Contract) *script.ContractInfo {
	return contract.Info()
}

func (c *commonApi) GetContractInfo(ctx RequestContext, auth Auth, contractName string) (*GetContractResult, *Error) {
	r := ctx.HTTPRequest()
	logger := getLogger(r)

	contract := getContract(r, contractName)
	if contract == nil {
		logger.WithFields(log.Fields{"type": consts.ContractError, "contract_name": contractName}).Debug("contract name")
		return nil, DefaultError(fmt.Sprintf("There is not %s contract", contractName))
	}

	var result GetContractResult
	info := getContractInfo(contract)
	con := &sqldb.Contract{}
	exits, err := con.Get(info.Owner.TableID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "contract_id": info.Owner.TableID}).Error("get contract")
		return nil, DefaultError(fmt.Sprintf("get contract %d failed:%s", info.Owner.TableID, err.Error()))
	}
	if !exits {
		logger.WithFields(log.Fields{"type": consts.ContractError, "contract id": info.Owner.TableID}).Debug("get contract")
		return nil, DefaultError(fmt.Sprintf("There is not %d contract", info.Owner.TableID))
	}
	fields := make([]contractField, 0)
	result = GetContractResult{
		ID:         uint32(info.Owner.TableID + consts.ShiftContractID),
		TableID:    converter.Int64ToStr(info.Owner.TableID),
		Name:       info.Name,
		StateID:    info.Owner.StateID,
		WalletID:   converter.Int64ToStr(info.Owner.WalletID),
		TokenID:    converter.Int64ToStr(info.Owner.TokenID),
		Address:    converter.AddressToString(info.Owner.WalletID),
		Ecosystem:  uint32(con.EcosystemID),
		AppId:      uint32(con.AppID),
		Conditions: con.Conditions,
	}

	if info.Tx != nil {
		for _, fitem := range *info.Tx {
			fields = append(fields, contractField{
				Name:     fitem.Name,
				Type:     script.OriginalToString(fitem.Original),
				Optional: fitem.ContainsTag(script.TagOptional),
			})
		}
	}
	result.Fields = fields
	return &result, nil
}

type ListResult struct {
	Count int64               `json:"count"`
	List  []map[string]string `json:"list"`
}

func (c *commonApi) GetContracts(ctx RequestContext, auth Auth, offset, limit *int) (*ListResult, *Error) {
	r := ctx.HTTPRequest()

	form := &paginatorForm{}
	if offset != nil {
		form.Offset = *offset
	}
	if limit != nil {
		form.Limit = *limit
	}

	if err := parameterValidator(r, form); err != nil {
		return nil, InvalidParamsError(err.Error())
	}

	client := getClient(r)
	logger := getLogger(r)

	contract := &sqldb.Contract{}
	contract.EcosystemID = client.EcosystemID

	count, err := contract.CountByEcosystem()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting table records count")
		return nil, DefaultError(err.Error())
	}

	contracts, err := contract.GetListByEcosystem(form.Offset, form.Limit)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting all")
		return nil, DefaultError(err.Error())
	}

	list := make([]map[string]string, len(contracts))
	for i, c := range contracts {
		list[i] = c.ToMap()
		list[i]["address"] = converter.AddressToString(c.WalletID)
	}

	if len(list) == 0 {
		list = nil
	}

	return &ListResult{
		Count: count,
		List:  list,
	}, nil
}

type roleInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type notifyInfo struct {
	RoleID string `json:"role_id"`
	Count  int64  `json:"count"`
}

type KeyInfoResult struct {
	Account    string              `json:"account"`
	Ecosystems []*keyEcosystemInfo `json:"ecosystems"`
}

type keyEcosystemInfo struct {
	Ecosystem     string       `json:"ecosystem"`
	Name          string       `json:"name"`
	Digits        int64        `json:"digits"`
	Roles         []roleInfo   `json:"roles,omitempty"`
	Notifications []notifyInfo `json:"notifications,omitempty"`
}

func getNotifications(ecosystemID int64, key *sqldb.Key) ([]notifyInfo, error) {
	notif, err := sqldb.GetNotificationsCount(ecosystemID, []string{key.AccountID})
	if err != nil {
		return nil, err
	}

	list := make([]notifyInfo, 0)
	for _, n := range notif {
		if n.RecipientID != key.ID {
			continue
		}

		list = append(list, notifyInfo{
			RoleID: converter.Int64ToStr(n.RoleID),
			Count:  n.Count,
		})
	}
	return list, nil
}

func (c *commonApi) GetKeyInfo(ctx RequestContext, accountAddress string) (*KeyInfoResult, *Error) {
	r := ctx.HTTPRequest()
	logger := getLogger(r)

	keysList := make([]*keyEcosystemInfo, 0)
	keyID := converter.StringToAddress(accountAddress)
	if keyID == 0 {
		return nil, InvalidParamsError(fmt.Sprintf("account address %s is not valid", accountAddress))
	}

	ids, names, err := c.mode.EcosystemGetter.GetEcosystemLookup()
	if err != nil {
		return nil, DefaultError(err.Error())
	}

	var (
		account = converter.AddressToString(keyID)
		found   bool
	)

	for i, ecosystemID := range ids {
		key := &sqldb.Key{}
		key.SetTablePrefix(ecosystemID)
		found, err = key.Get(nil, keyID)
		if err != nil {
			return nil, DefaultError(err.Error())
		}
		if !found {
			continue
		}
		eco := sqldb.Ecosystem{}
		_, err = eco.Get(nil, ecosystemID)
		if err != nil {
			return nil, DefaultError(err.Error())
		}
		keyRes := &keyEcosystemInfo{
			Ecosystem: converter.Int64ToStr(ecosystemID),
			Name:      names[i],
			Digits:    eco.Digits,
		}
		ra := &sqldb.RolesParticipants{}
		roles, err := ra.SetTablePrefix(ecosystemID).GetActiveMemberRoles(key.AccountID)
		if err != nil {
			return nil, DefaultError(err.Error())
		}
		for _, r := range roles {
			var role roleInfo
			if err := json.Unmarshal([]byte(r.Role), &role); err != nil {
				logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling role")
				return nil, DefaultError(err.Error())
			}
			keyRes.Roles = append(keyRes.Roles, role)
		}
		keyRes.Notifications, err = getNotifications(ecosystemID, key)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting notifications")
			return nil, DefaultError(err.Error())
		}

		keysList = append(keysList, keyRes)
	}

	// in test mode, registration is open in the first ecosystem
	if len(keysList) == 0 {
		notify := make([]notifyInfo, 0)
		notify = append(notify, notifyInfo{})
		keysList = append(keysList, &keyEcosystemInfo{
			Ecosystem:     converter.Int64ToStr(ids[0]),
			Name:          names[0],
			Notifications: notify,
		})
	}

	return &KeyInfoResult{
		Account:    account,
		Ecosystems: keysList,
	}, nil
}

type ListForm struct {
	Name string `json:"name"` //table name
	paginatorForm
	rowForm
}

func (f *ListForm) Validate(r *http.Request) error {
	if f == nil || f.Name == "" {
		return errors.New(paramsEmpty)
	}
	if err := f.paginatorForm.Validate(r); err != nil {
		return err
	}
	return f.rowForm.Validate(r)
}

type rowForm struct {
	Columns string `json:"columns"`
}

func (f *rowForm) Validate(r *http.Request) error {
	if len(f.Columns) > 0 {
		f.Columns = converter.EscapeName(f.Columns)
	}
	return nil
}

func checkAccess(tableName, columns string, client *UserClient) (table string, cols string, err error) {
	sc := smart.SmartContract{
		CLB: conf.Config.IsSupportingCLB(),
		VM:  script.GetVM(),
		TxSmart: &types.SmartTransaction{
			Header: &types.Header{
				EcosystemID: client.EcosystemID,
				KeyID:       client.KeyID,
				NetworkID:   conf.Config.LocalConf.NetworkID,
			},
		},
	}
	table, _, cols, err = sc.CheckAccess(tableName, columns, client.EcosystemID)
	return
}

func (c *commonApi) GetList(ctx RequestContext, auth Auth, form *ListWhereForm) (*ListResult, *Error) {
	r := ctx.HTTPRequest()
	if form == nil {
		return nil, InvalidParamsError(paramsEmpty)
	}

	if err := parameterValidator(r, form); err != nil {
		return nil, InvalidParamsError(err.Error())
	}

	client := getClient(r)
	logger := getLogger(r)

	var (
		err          error
		table, where string
	)
	table, form.Columns, err = checkAccess(form.Name, form.Columns, client)
	if err != nil {
		return nil, DefaultError(err.Error())
	}
	var q *gorm.DB
	q = sqldb.GetTableListQuery(form.Name, client.EcosystemID)

	if len(form.Columns) > 0 {
		q = q.Select("id," + smart.PrepareColumns([]string{form.Columns}))
	}

	if form.Where != nil {
		var inWhere any
		switch form.Where.(type) {
		case string:
			if len(form.Where.(string)) > 0 {
				inWhere, _, err = template.ParseObject([]rune(form.Where.(string)))
				if err != nil {
					return nil, DefaultError("where parse object failed")
				}
			} else {
				inWhere = ""
			}
		}

		switch v := inWhere.(type) {
		case string:
			if len(v) == 0 {
				where = `true`
			} else {
				return nil, DefaultError("Where has wrong format")
			}
		case map[string]any:
			where, err = qb.GetWhere(types.LoadMap(v))
			if err != nil {
				return nil, DefaultError(err.Error())
			}
		case *types.Map:
			where, err = qb.GetWhere(v)
			if err != nil {
				return nil, DefaultError(err.Error())
			}
		default:
			return nil, DefaultError("Where has wrong format")
		}
		q = q.Where(where)
	}

	result := new(ListResult)
	err = q.Count(&result.Count).Error

	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).
			Errorf("selecting rows from table %s select %s where %s", table, smart.PrepareColumns([]string{form.Columns}), where)
		return nil, DefaultError(fmt.Sprintf("Table %s has not been found", table))
	}

	if len(form.Order) > 0 {
		rows, err := q.Order(form.Order).Offset(form.Offset).Limit(form.Limit).Rows()
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Error("Getting rows from table")
			return nil, DefaultError(err.Error())
		}
		result.List, err = sqldb.GetResult(rows)
		if err != nil {
			return nil, DefaultError(err.Error())
		}
	} else {
		rows, err := q.Order("id ASC").Offset(form.Offset).Limit(form.Limit).Rows()
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Error("Getting rows from table")
			return nil, DefaultError(err.Error())
		}
		result.List, err = sqldb.GetResult(rows)
		if err != nil {
			return nil, DefaultError(err.Error())
		}
	}

	return result, nil
}

type HonorNodeJSON struct {
	TCPAddress string `json:"tcp_address"`
	APIAddress string `json:"api_address"`
	PublicKey  string `json:"public_key"`
	UnbanTime  string `json:"unban_time"`
	Stopped    bool   `json:"stopped"`
}

type NetworkResult struct {
	NetworkID     string          `json:"network_id"`
	CentrifugoURL string          `json:"centrifugo_url"`
	Test          bool            `json:"test"`
	Private       bool            `json:"private"`
	HonorNodes    []HonorNodeJSON `json:"honor_nodes"`
}

func getNodesJSON() []HonorNodeJSON {
	nodes := make([]HonorNodeJSON, 0)
	for _, node := range syspar.GetNodes() {
		nodes = append(nodes, HonorNodeJSON{
			TCPAddress: node.TCPAddress,
			APIAddress: node.APIAddress,
			PublicKey:  crypto.PubToHex(node.PublicKey),
			UnbanTime:  strconv.FormatInt(node.UnbanTime.Unix(), 10),
		})
	}
	return nodes
}

const defaultSectionsLimit = 100

type SectionsForm struct {
	paginatorForm
	Lang string `schema:"lang"`
}

func (f *SectionsForm) Validate(r *http.Request) error {
	if f == nil {
		return errors.New(paramsEmpty)
	}
	if err := f.paginatorForm.Validate(r); err != nil {
		return err
	}

	if len(f.Lang) == 0 {
		f.Lang = r.Header.Get("Accept-Language")
	}

	return nil
}

func (c *commonApi) GetSections(ctx RequestContext, auth Auth, params *SectionsForm) (*ListResult, *Error) {
	r := ctx.HTTPRequest()
	form := &SectionsForm{}
	if params != nil {
		form = params
	}
	if err := parameterValidator(r, form); err != nil {
		return nil, InvalidParamsError(err.Error())
	}

	client := getClient(r)
	logger := getLogger(r)

	table := "1_sections"
	q := sqldb.GetDB(nil).Table(table).Where("ecosystem = ? AND status > 0", client.EcosystemID).Order("id ASC")

	result := new(ListResult)
	err := q.Count(&result.Count).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Error("Getting table records count")
		return nil, DefaultError(fmt.Sprintf("Table %s has not been found", table))
	}

	rows, err := q.Offset(form.Offset).Limit(form.Limit).Rows()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Error("Getting rows from table")
		return nil, DefaultError(err.Error())
	}

	result.List, err = sqldb.GetResult(rows)
	if err != nil {
		return nil, DefaultError(err.Error())
	}

	var sections []map[string]string
	for _, item := range result.List {
		var roles []int64
		if err := json.Unmarshal([]byte(item["roles_access"]), &roles); err != nil {
			return nil, DefaultError(err.Error())
		}
		if len(roles) > 0 {
			var added bool
			for _, v := range roles {
				if v == client.RoleID {
					added = true
					break
				}
			}
			if !added {
				continue
			}
		}

		if item["status"] == consts.StatusMainPage {
			roles := &sqldb.Role{}
			roles.SetTablePrefix(1)
			role, err := roles.Get(nil, client.RoleID)

			if err != nil {
				logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Debug("Getting role by id")
				return nil, DefaultError(err.Error())
			}
			if role == true && roles.DefaultPage != "" {
				item["default_page"] = roles.DefaultPage
			}
		}

		item["title"] = language.LangMacro(item["title"], int(client.EcosystemID), form.Lang)
		sections = append(sections, item)
	}
	result.List = sections
	return result, nil
}

type RowResult struct {
	Value map[string]string `json:"value"`
}

// GetRow
// whereColumn: find whereColumn = id or Find id
// columns: select colunms
// example: "params":["@1history",6660819716178795186,"sender_id","created_at,ecosystem"]
func (c *commonApi) GetRow(ctx RequestContext, auth Auth, tableName string, id int64, columns *string, whereColumn *string) (*RowResult, *Error) {
	r := ctx.HTTPRequest()
	form := &rowForm{}
	if columns != nil {
		form.Columns = *columns
		if err := parameterValidator(r, form); err != nil {
			return nil, InvalidParamsError(err.Error())
		}
	}
	idStr := strconv.FormatInt(id, 10)
	if tableName == "" || idStr == "" {
		return nil, InvalidParamsError("tableName or id invalid")
	}

	client := getClient(r)
	logger := getLogger(r)

	q := sqldb.GetDB(nil).Limit(1)

	var (
		err   error
		table string
	)
	table, form.Columns, err = checkAccess(tableName, form.Columns, client)
	if err != nil {
		return nil, DefaultError(err.Error())
	}
	col := `id`
	if whereColumn != nil && len(*whereColumn) > 0 {
		col = converter.Sanitize(*whereColumn, `-`)
	}
	if converter.FirstEcosystemTables[tableName] {
		q = q.Table(table).Where(col+" = ? and ecosystem = ?", idStr, client.EcosystemID)
	} else {
		q = q.Table(table).Where(col+" = ?", idStr)
	}

	if len(form.Columns) > 0 {
		q = q.Select(form.Columns)
	}

	rows, err := q.Rows()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Error("Getting rows from table")
		return nil, DefaultError("DB query is wrong")
	}

	result, err := sqldb.GetResult(rows)
	if err != nil {
		return nil, DefaultError(err.Error())
	}

	if len(result) == 0 {
		return nil, NotFoundError()
	}

	return &RowResult{
		Value: result[0],
	}, nil
}

type PartModel interface {
	SetTablePrefix(prefix string)
	Get(name string) (bool, error)
}

func getPageRowMux(ctx RequestContext, name string) (PartModel, *Error) {
	return getInterfaceRow(ctx, name, &sqldb.Page{})
}

func getMenuRowMux(ctx RequestContext, name string) (PartModel, *Error) {
	return getInterfaceRow(ctx, name, &sqldb.Menu{})
}

func getSnippetRowMux(ctx RequestContext, name string) (PartModel, *Error) {
	return getInterfaceRow(ctx, name, &sqldb.Snippet{})
}

func getInterfaceRow(ctx RequestContext, name string, c PartModel) (PartModel, *Error) {
	r := ctx.HTTPRequest()
	logger := getLogger(r)
	client := getClient(r)

	c.SetTablePrefix(client.Prefix())
	if ok, err := c.Get(name); err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting one row")
		return nil, DefaultError("DB query is wrong")
	} else if !ok {
		return nil, NotFoundError()
	}
	return c, nil
}

func (c *commonApi) GetPageRow(ctx RequestContext, auth Auth, name string) (PartModel, *Error) {
	if name == "" {
		return nil, InvalidParamsError(paramsEmpty)
	}
	return getPageRowMux(ctx, name)
}

func (c *commonApi) GetMenuRow(ctx RequestContext, auth Auth, name string) (PartModel, *Error) {
	if name == "" {
		return nil, InvalidParamsError(paramsEmpty)
	}
	return getMenuRowMux(ctx, name)
}

func (c *commonApi) GetSnippetRow(ctx RequestContext, auth Auth, name string) (PartModel, *Error) {
	if name == "" {
		return nil, InvalidParamsError(paramsEmpty)
	}
	return getSnippetRowMux(ctx, name)
}

type columnInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Perm string `json:"perm"`
}

type TableResult struct {
	Name       string       `json:"name"`
	Insert     string       `json:"insert"`
	NewColumn  string       `json:"new_column"`
	Update     string       `json:"update"`
	Read       string       `json:"read,omitempty"`
	Filter     string       `json:"filter,omitempty"`
	Conditions string       `json:"conditions"`
	AppID      string       `json:"app_id"`
	Columns    []columnInfo `json:"columns"`
}

func (c *commonApi) GetTable(ctx RequestContext, auth Auth, name string) (*TableResult, *Error) {
	if name == "" {
		return nil, InvalidParamsError(paramsEmpty)
	}
	r := ctx.HTTPRequest()
	logger := getLogger(r)
	client := getClient(r)
	prefix := client.Prefix()

	table := &sqldb.Table{}
	table.SetTablePrefix(prefix)

	_, err := table.Get(nil, strings.ToLower(name))
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting table")
		return nil, DefaultError(err.Error())
	}

	if len(table.Name) == 0 {
		return nil, DefaultError(fmt.Sprintf("Table %s has not been found", name))
	}

	var columnsMap map[string]string
	err = json.Unmarshal([]byte(table.Columns), &columnsMap)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("Unmarshalling table columns to json")
		return nil, DefaultError(err.Error())
	}

	columns := make([]columnInfo, 0)
	for key, value := range columnsMap {
		colType, err := sqldb.NewDbTransaction(nil).GetColumnType(prefix+`_`+name, key)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting column type from db")
			return nil, DefaultError(err.Error())
		}
		columns = append(columns, columnInfo{
			Name: key,
			Perm: value,
			Type: colType,
		})
	}
	return &TableResult{
		Name:       table.Name,
		Insert:     table.Permissions.Insert,
		NewColumn:  table.Permissions.NewColumn,
		Update:     table.Permissions.Update,
		Read:       table.Permissions.Read,
		Filter:     table.Permissions.Filter,
		Conditions: table.Conditions,
		AppID:      converter.Int64ToStr(table.AppID),
		Columns:    columns,
	}, nil
}

type tableInfo struct {
	Name  string `json:"name"`
	Count string `json:"count"`
}

type TableCountResult struct {
	Count int64       `json:"count"`
	List  []tableInfo `json:"list"`
}

func (c *commonApi) GetTableCount(ctx RequestContext, auth Auth, offset, limit *int) (*TableCountResult, *Error) {
	r := ctx.HTTPRequest()

	form := &paginatorForm{}
	if offset != nil {
		form.Offset = *offset
	}
	if limit != nil {
		form.Limit = *limit
	}
	if err := parameterValidator(r, form); err != nil {
		return nil, InvalidParamsError(err.Error())
	}

	client := getClient(r)
	logger := getLogger(r)
	prefix := client.Prefix()

	table := &sqldb.Table{}
	table.SetTablePrefix(prefix)

	count, err := table.Count()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("selecting records count from tables")
		return nil, DefaultError(err.Error())
	}

	rows, err := sqldb.GetDB(nil).Table(table.TableName()).Where("ecosystem = ?", client.EcosystemID).Offset(form.Offset).Limit(form.Limit).Rows()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Error("Getting rows from table")
		return nil, DefaultError(err.Error())
	}

	list, err := sqldb.GetResult(rows)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("selecting names from tables")
		return nil, DefaultError(err.Error())
	}

	result := &TableCountResult{
		Count: count,
		List:  make([]tableInfo, len(list)),
	}
	for i, item := range list {
		err = sqldb.GetTableQuery(item["name"], client.EcosystemID).Count(&count).Error
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("selecting count from table")
			return nil, DefaultError(err.Error())
		}

		result.List[i].Name = item["name"]
		result.List[i].Count = converter.Int64ToStr(count)
	}
	return result, nil
}

func (c *commonApi) GetVersion() string {
	return consts.Version() + " " + node.NodePauseType().String()
}

func replaceHttpSchemeToWs(centrifugoURL string) string {
	if strings.HasPrefix(centrifugoURL, "http:") {
		return strings.Replace(centrifugoURL, "http:", "ws:", -1)
	} else if strings.HasPrefix(centrifugoURL, "https:") {
		return strings.Replace(centrifugoURL, "https:", "wss:", -1)
	}
	return centrifugoURL
}

func centrifugoAddressHandler(r *http.Request) (string, error) {
	logger := getLogger(r)

	if _, err := publisher.GetStats(); err != nil {
		logger.WithFields(log.Fields{"type": consts.CentrifugoError, "error": err}).Warn("on getting centrifugo stats")
		return "", err
	}

	return replaceHttpSchemeToWs(conf.Config.Centrifugo.URL), nil
}

func (c *commonApi) GetConfig(ctx RequestContext, option string) (map[string]any, *Error) {
	r := ctx.HTTPRequest()
	logger := getLogger(r)
	if option == "" {
		logger.WithFields(log.Fields{"type": consts.EmptyObject, "error": "option not specified"}).Error("on getting option in config handler")
		return nil, NotFoundError()
	}

	rets := make(map[string]any)
	var err error
	switch option {
	case "centrifugo":
		rets[option], err = centrifugoAddressHandler(r)
		if err != nil {
			return nil, DefaultError(err.Error())
		}
		return rets, nil
	}

	return nil, NotFoundError()
}

func parseEcosystem(in string) (string, string) {
	ecosystem, name := converter.ParseName(in)
	if ecosystem == 0 {
		return ``, name
	}
	return converter.Int64ToStr(ecosystem), name
}

func pageValue(r *http.Request, name string) (*sqldb.Page, string, error) {
	logger := getLogger(r)
	client := getClient(r)

	var ecosystem string
	page := &sqldb.Page{}
	if strings.HasPrefix(name, `@`) {
		ecosystem, name = parseEcosystem(name)
		if len(name) == 0 {
			logger.WithFields(log.Fields{
				"type":  consts.NotFound,
				"value": name,
			}).Debug("page not found")
			return nil, ``, errors.New(consts.NotFound)
		}
	} else {
		ecosystem = client.Prefix()
	}
	page.SetTablePrefix(ecosystem)
	found, err := page.Get(name)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting page")
		return nil, ``, err
	}
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound}).Debug("page not found")
		return nil, ``, errors.New(consts.NotFound)
	}
	return page, ecosystem, nil
}

func (c *commonApi) GetPageValidatorsCount(ctx RequestContext, name string) (*map[string]int64, *Error) {
	r := ctx.HTTPRequest()
	if name == "" {
		return nil, NotFoundError()
	}
	page, _, err := pageValue(r, name)
	if err != nil {
		return nil, DefaultError(err.Error())
	}

	res := map[string]int64{"validate_count": page.ValidateCount}
	return &res, nil
}

func initVars(r *http.Request, vals *map[string]string) *map[string]string {
	client := getClient(r)

	vars := make(map[string]string)
	if vals != nil {
		for k, v := range *vals {
			vars[k] = v
		}
	}
	vars["_full"] = "0"
	vars["current_time"] = fmt.Sprintf("%d", time.Now().Unix())
	vars["guest_key"] = consts.GuestKey
	vars["guest_account"] = consts.GuestAddress
	vars["black_hole_key"] = strconv.FormatInt(converter.HoleAddrMap[converter.BlackHoleAddr].K, 10)
	vars["black_hole_account"] = converter.HoleAddrMap[converter.BlackHoleAddr].S
	vars["white_hole_key"] = strconv.FormatInt(converter.HoleAddrMap[converter.WhiteHoleAddr].K, 10)
	vars["white_hole_account"] = converter.HoleAddrMap[converter.WhiteHoleAddr].S
	if client.KeyID != 0 {
		vars["ecosystem_id"] = converter.Int64ToStr(client.EcosystemID)
		vars["key_id"] = converter.Int64ToStr(client.KeyID)
		vars["account_id"] = client.AccountID
		vars["role_id"] = converter.Int64ToStr(client.RoleID)
		vars["ecosystem_name"] = client.EcosystemName
	} else {
		vars["ecosystem_id"] = vars["ecosystem"]
		delete(vars, "ecosystem")
		if len(vars["keyID"]) > 0 {
			vars["key_id"] = vars["keyID"]
			vars["account_id"] = converter.AddressToString(converter.StrToInt64(vars["keyID"]))
		} else {
			vars["key_id"] = "0"
			vars["account_id"] = ""
		}
		if len(vars["roleID"]) > 0 {
			vars["role_id"] = vars["roleID"]
		} else {
			vars["role_id"] = "0"
		}
		if len(vars["ecosystem_id"]) != 0 {
			ecosystems := sqldb.Ecosystem{}
			if found, _ := ecosystems.Get(nil, converter.StrToInt64(vars["ecosystem_id"])); found {
				vars["ecosystem_name"] = ecosystems.Name
			}
		}
	}
	if _, ok := vars["lang"]; !ok {
		vars["lang"] = r.Header.Get("Accept-Language")
	}

	return &vars
}

type ContentResult struct {
	Menu       string          `json:"menu,omitempty"`
	MenuTree   json.RawMessage `json:"menutree,omitempty"`
	Title      string          `json:"title,omitempty"`
	Tree       json.RawMessage `json:"tree"`
	NodesCount int64           `json:"nodesCount,omitempty"`
}

const strOne = `1`

func (c *commonApi) GetSource(ctx RequestContext, auth Auth, name string, vals *map[string]string) (*ContentResult, *Error) {
	r := ctx.HTTPRequest()
	if name == "" {
		return nil, NotFoundError()
	}
	page, _, err := pageValue(r, name)
	if err != nil {
		return nil, DefaultError(err.Error())
	}
	var timeout bool
	vars := initVars(r, vals)
	(*vars)["_full"] = strOne
	ret := template.Template2JSON(page.Value, &timeout, vars)

	return &ContentResult{Tree: ret}, nil
}

func getPage(r *http.Request, name string, vals *map[string]string) (result *ContentResult, err error) {
	page, _, err := pageValue(r, name)
	if err != nil {
		return nil, err
	}

	logger := getLogger(r)

	client := getClient(r)
	menu := &sqldb.Menu{}
	menu.SetTablePrefix(client.Prefix())
	_, err = menu.Get(page.Menu)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting page menu")
		return nil, errors.New("Server error")
	}
	var wg sync.WaitGroup
	var timeout bool
	wg.Add(2)
	success := make(chan bool, 1)
	go func() {
		defer wg.Done()

		vars := initVars(r, vals)
		(*vars)["app_id"] = converter.Int64ToStr(page.AppID)

		ret := template.Template2JSON(page.Value, &timeout, vars)
		if timeout {
			return
		}
		retmenu := template.Template2JSON(menu.Value, &timeout, vars)
		if timeout {
			return
		}
		result = &ContentResult{
			Tree:       ret,
			Menu:       page.Menu,
			MenuTree:   retmenu,
			NodesCount: page.ValidateCount,
		}
		success <- true
	}()
	go func() {
		defer wg.Done()
		if conf.Config.LocalConf.MaxPageGenerationTime == 0 {
			return
		}
		select {
		case <-time.After(time.Duration(conf.Config.LocalConf.MaxPageGenerationTime) * time.Millisecond):
			timeout = true
		case <-success:
		}
	}()
	wg.Wait()
	close(success)

	if timeout {
		logger.WithFields(log.Fields{"type": consts.InvalidObject}).Error(page.Name + " is a heavy page")
		return nil, errors.New("this page is heavy")
	}

	return result, nil
}

func (c *commonApi) GetPage(ctx RequestContext, auth Auth, name string, vals *map[string]string) (*ContentResult, *Error) {
	r := ctx.HTTPRequest()
	result, err := getPage(r, name, vals)
	if err != nil {
		return nil, DefaultError(err.Error())
	}
	return result, nil
}

type hashResult struct {
	Hash string `json:"hash"`
}

func (c *commonApi) GetPageHash(ctx RequestContext, name string, ecosystem *int64, vals *map[string]string) (*hashResult, *Error) {
	r := ctx.HTTPRequest()
	logger := getLogger(r)

	if ecosystem != nil && !strings.HasPrefix(name, "@") && *ecosystem != 0 {
		name = "@" + strconv.FormatInt(*ecosystem, 10) + name
	}
	result, err := getPage(r, name, vals)
	if err != nil {
		return nil, DefaultError(err.Error())
	}

	out, err := json.Marshal(result)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.JSONMarshallError, "error": err}).Error("getting string for hash")
		return nil, DefaultError(err.Error())
	}

	return &hashResult{Hash: hex.EncodeToString(crypto.Hash(out))}, nil
}

func (c *commonApi) GetMenu(ctx RequestContext, auth Auth, name string, vals *map[string]string) (*ContentResult, *Error) {
	r := ctx.HTTPRequest()
	client := getClient(r)
	logger := getLogger(r)

	var ecosystem string
	menu := &sqldb.Menu{}
	if strings.HasPrefix(name, `@`) {
		ecosystem, name = parseEcosystem(name)
		if len(name) == 0 {
			logger.WithFields(log.Fields{
				"type":  consts.NotFound,
				"value": name,
			}).Debug("page not found")
			return nil, NotFoundError()
		}
	} else {
		ecosystem = client.Prefix()
	}

	menu.SetTablePrefix(ecosystem)
	found, err := menu.Get(name)

	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting menu")
		return nil, DefaultError(err.Error())
	}
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound}).Debug("menu not found")
		return nil, NotFoundError()
	}
	var timeout bool
	ret := template.Template2JSON(menu.Value, &timeout, initVars(r, vals))

	return &ContentResult{Tree: ret, Title: menu.Title}, nil
}

type jsonContentForm struct {
	Template string `json:"template"`
	Source   bool   `json:"source"`
}

func (f *jsonContentForm) Validate(r *http.Request) error {
	if f == nil || len(f.Template) == 0 {
		return errors.New(paramsEmpty)
	}
	return nil
}

func (c *commonApi) GetContent(ctx RequestContext, form *jsonContentForm, vals *map[string]string) (*ContentResult, *Error) {
	r := ctx.HTTPRequest()
	if err := parameterValidator(r, form); err != nil {
		return nil, InvalidParamsError(err.Error())
	}

	var timeout bool
	vars := initVars(r, vals)

	if form.Source {
		(*vars)["_full"] = strOne
	}

	ret := template.Template2JSON(form.Template, &timeout, vars)

	return &ContentResult{Tree: ret}, nil
}

func (c *commonApi) GetTxCount(ctx RequestContext) (*int64, *Error) {
	r := ctx.HTTPRequest()
	count, err := sqldb.GetTxCount()
	if err != nil {
		logger := getLogger(r)
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on getting tx count")
		return nil, InternalError(err.Error())
	}

	return &count, nil
}

func (c *commonApi) GetEcosystemCount(ctx RequestContext) (*int64, *Error) {
	r := ctx.HTTPRequest()
	total, err := sqldb.GetAllSystemCount()
	if err != nil {
		logger := getLogger(r)
		logger.WithError(err).Error("on getting ecosystem count")
		return nil, InternalError(err.Error())
	}

	return &total, nil
}
