/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"fmt"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type smartParams struct {
	Params  map[string]string
	Results map[string]string
}

type smartContract struct {
	Name   string
	Value  string
	Params []smartParams
}

func TestUpperName(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	rnd := crypto.RandSeq(4)
	form := url.Values{"Name": {"testTable" + rnd}, "ApplicationId": {"1"}, "Columns": {`[{"name":"num","type":"text",   "conditions":"true"},
	{"name":"text", "type":"text","conditions":"true"}]`},
		"Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	err := postTx(`NewTable`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	form = url.Values{`Value`: {`contract AddRow` + rnd + ` {
		data {
		}
		conditions {
		}
		action {
		   DBInsert("testTable` + rnd + `", {num: "fgdgf", text: "124234"}) 
		}
	}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	if err := postTx(`NewContract`, &form); err != nil {
		t.Error(err)
		return
	}
	if err := postTx(`AddRow`+rnd, &url.Values{}); err != nil {
		t.Error(err)
		return
	}
}

func TestSmartFields(t *testing.T) {

	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	var cntResult getContractResult
	err := sendGet(`contract/MainCondition`, nil, &cntResult)
	if err != nil {
		t.Error(err)
		return
	}
	if len(cntResult.Fields) != 0 {
		t.Error(`MainCondition fields must be empty`)
		return
	}
	if cntResult.Name != `@1MainCondition` {
		t.Errorf(`MainCondition name is wrong: %s`, cntResult.Name)
		return
	}
	if err := postTx(`MainCondition`, &url.Values{}); err != nil {
		t.Error(err)
		return
	}
}

func TestMoneyTransfer(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}

	form := url.Values{`Amount`: {`53330000`}, `Recipient`: {`0005-2070-2000-0006-0200`}}
	if err := postTx(`MoneyTransfer`, &form); err != nil {
		t.Error(err)
		return
	}
	form = url.Values{`Amount`: {`2440000`}, `Recipient`: {`1109-7770-3360-6764-7059`}, `Comment`: {`Test`}}
	if err := postTx(`MoneyTransfer`, &form); err != nil {
		t.Error(err)
		return
	}
	form = url.Values{`Amount`: {`53330000`}, `Recipient`: {`0005207000`}}
	if err := postTx(`MoneyTransfer`, &form); cutErr(err) != `{"type":"error","error":"Recipient 0005207000 is invalid"}` {
		t.Error(err)
		return
	}
	size := 1000000
	big := make([]byte, size)
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < size; i++ {
		big[i] = '0' + byte(rand.Intn(10))
	}
	form = url.Values{`Amount`: {string(big)}, `Recipient`: {`0005-2070-2000-0006-0200`}}
	if err := postTx(`MoneyTransfer`, &form); err.Error() != `400 {"error": "E_LIMITFORSIGN", "msg": "Length of forsign is too big (1000106)" , "params": ["1000106"]}` {
		t.Error(err)
		return
	}
}

func TestRoleAccess(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	name := randName(`page`)
	menu := `government`
	value := `P(test,test paragraph)`

	form := url.Values{"Name": {name}, "Value": {value}, "Menu": {menu}, "ApplicationId": {`1`},
		"Conditions": {`RoleAccess(10,1)`}}
	assert.NoError(t, postTx(`NewPage`, &form))

	var ret listResult
	assert.NoError(t, sendGet(`list/pages`, nil, &ret))
	id := strconv.FormatInt(ret.Count, 10)
	form = url.Values{"Id": {id}, "Value": {"Div(){Ooops}"}, "Conditions": {`RoleAccess(65)`}}
	assert.NoError(t, postTx(`EditPage`, &form))
	form = url.Values{"Id": {id}, "Value": {"Div(){Update}"}}
	assert.EqualError(t, postTx(`EditPage`, &form), `{"type":"panic","error":"Access denied"}`)
}

func TestDBFind(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	name := randName(`tbl`)
	form := url.Values{"Name": {name}, "ApplicationId": {"1"}, "Columns": {`[{"name":"txt","type":"varchar", 
		"conditions":"true"},
	  {"name":"Name", "type":"varchar","index": "0", "conditions":"{\"read\":\"true\",\"update\":\"true\"}"}]`},
		"Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	assert.NoError(t, postTx(`NewTable`, &form))
	form = url.Values{`Value`: {`contract sub` + name + ` {
		action {
			DBInsert("` + name + `", {txt:"ok", name: "thisis"})
			DBInsert("` + name + `", {txt:"test", name: "test"})
			$result = DBFind("` + name + `").Columns("name").Where({txt:"test"}).One("name")
		}
	}`}, `Conditions`: {`true`}, "ApplicationId": {"1"}}
	assert.NoError(t, postTx(`NewContract`, &form))
	_, ret, err := postTxResult(`sub`+name, &url.Values{})
	assert.Equal(t, `heading`, ret)
	var retPage contentResult
	value := `DBFind(` + name + `, src).Columns(name).Where({txt:test})`
	form = url.Values{"Name": {name}, "Value": {value}, "ApplicationId": {`1`},
		"Menu": {`default_menu`}, "Conditions": {"ContractConditions(`MainCondition`)"}}
	assert.NoError(t, postTx(`NewPage`, &form))
	assert.NoError(t, sendPost(`content/page/`+name, &url.Values{}, &retPage))
	if err != nil {
		t.Error(err)
		return
	}
	if RawToString(retPage.Tree) != `[{"tag":"dbfind","attr":{"columns":["name","id"],"data":[["test","2"]],"name":"`+name+`","source":"src","types":["text","text"],"where":"{txt:test}"}}]` {
		t.Error(fmt.Errorf(`wrong tree %s`, RawToString(retPage.Tree)))
		return
	}
}
func TestPage(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	name := randName(`page`)
	menuname := randName(`menu`)
	menu := `government`
	value := `P(test,test paragraph)`

	form := url.Values{"Name": {name}, "Value": {`Param Value`},
		"Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`NewParameter`, &form))

	err := postTx(`NewParameter`, &form)
	assert.Equal(t, fmt.Sprintf(`{"type":"warning","error":"Parameter %s already exists"}`, name), cutErr(err))

	form = url.Values{"Name": {menuname}, "Value": {`first
			second
			third`}, "Title": {`My Menu`},
		"Conditions": {`true`}}
	assert.NoError(t, postTx(`NewMenu`, &form))

	err = postTx(`NewMenu`, &form)
	assert.Equal(t, fmt.Sprintf(`{"type":"warning","error":"Menu %s already exists"}`, menuname), cutErr(err))

	form = url.Values{"Id": {`7123`}, "Value": {`New Param Value`},
		"Conditions": {`ContractConditions("MainCondition")`}}
	err = postTx(`EditParameter`, &form)
	assert.Equal(t, `{"type":"panic","error":"Item 7123 has not been found"}`, cutErr(err))

	form = url.Values{"Id": {`16`}, "Value": {`Changed Param Value`},
		"Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`EditParameter`, &form))

	name = randName(`page`)
	form = url.Values{"Name": {name}, "Value": {value}, "ApplicationId": {`1`},
		"Menu": {menu}, "Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`NewPage`, &form))

	err = postTx(`NewPage`, &form)
	assert.Equal(t, fmt.Sprintf(`{"type":"warning","error":"Page %s already exists"}`, name), cutErr(err))
	err = postTx(`NewPage`, &form)
	if cutErr(err) != fmt.Sprintf(`{"type":"warning","error":"Page %s already exists"}`, name) {
		t.Error(err)
		return
	}
	form = url.Values{"Name": {`app` + name}, "Value": {value}, "ValidateCount": {"2"},
		"ValidateMode": {"1"}, "ApplicationId": {`1`},
		"Menu": {menu}, "Conditions": {`ContractConditions("MainCondition")`}}
	err = postTx(`NewPage`, &form)
	if err != nil {
		t.Error(err)
		return
	}

	var ret listResult
	err = sendGet(`list/pages`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	id := strconv.FormatInt(ret.Count, 10)
	form = url.Values{"Id": {id}, "ValidateCount": {"2"}, "ValidateMode": {"1"}}
	err = postTx(`EditPage`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	var row rowResult
	err = sendGet(`row/pages/`+id, nil, &row)
	if err != nil {
		t.Error(err)
		return
	}

	if row.Value["validate_mode"] != `1` {
		t.Errorf(`wrong validate value %s`, row.Value["validate_mode"])
		return
	}

	form = url.Values{"Id": {id}, "Value": {value}, "ValidateCount": {"1"},
		"ValidateMode": {"0"}}
	err = postTx(`EditPage`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	err = sendGet(`row/pages/`+id, nil, &row)
	if err != nil {
		t.Error(err)
		return
	}
	if row.Value["validate_mode"] != `0` {
		t.Errorf(`wrong validate value %s`, row.Value["validate_mode"])
		return
	}

	form = url.Values{"Id": {id}, "Value": {value}, "ValidateCount": {"1"},
		"ValidateMode": {"0"}}
	err = postTx(`EditPage`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	err = sendGet(`row/pages/`+id, nil, &row)
	if err != nil {
		t.Error(err)
		return
	}
	if row.Value["validate_mode"] != `0` {
		t.Errorf(`wrong validate value %s`, row.Value["validate_mode"])
		return
	}

	form = url.Values{"Name": {name}, "Value": {value}, "ApplicationId": {`1`},
		"Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`NewSnippet`, &form))

	err = postTx(`NewSnippet`, &form)
	assert.EqualError(t, err, fmt.Sprintf(`{"type":"warning","error":"Block %s already exists"}`, name))

	form = url.Values{"Id": {`1`}, "Name": {name}, "Value": {value},
		"Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`EditSnippet`, &form))

	form = url.Values{"Id": {`1`}, "Value": {value + `Span(Test)`},
		"Menu": {menu}, "Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`EditPage`, &form))

	form = url.Values{"Id": {`1112`}, "Value": {value + `Span(Test)`},
		"Menu": {menu}, "Conditions": {`ContractConditions("MainCondition")`}}
	err = postTx(`EditPage`, &form)
	assert.Equal(t, `{"type":"panic","error":"Item 1112 has not been found"}`, cutErr(err))

	form = url.Values{"Id": {`1`}, "Value": {`Span(Append)`}}
	assert.NoError(t, postTx(`AppendPage`, &form))
}

func TestNewTableOnly(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	name := "MMy_s_test_table"
	form := url.Values{"Name": {name}, "ApplicationId": {"1"}, "Columns": {`[{"name":"MyName","type":"varchar", 
		"conditions":"true"},
	  {"name":"Name", "type":"varchar","index": "0", "conditions":"{\"read\":\"true\",\"update\":\"true\"}"}]`},
		"Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	require.NoError(t, postTx(`NewTable`, &form))

	var ret tableResult
	require.NoError(t, sendGet(`table/`+name, nil, &ret))
	fmt.Printf("%+v\n", ret)
}

func TestUpperTable(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	name := randName(`Tab_`)
	form := url.Values{"Name": {name}, "ApplicationId": {"1"}, "Columns": {`[{"name":"MyName","type":"varchar", 
		"conditions":"true"},
	  {"name":"Name", "type":"varchar","index": "0", "conditions":"{\"read\":\"true\",\"update\":\"true\"}"}]`},
		"Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	assert.NoError(t, postTx(`NewTable`, &form))

	form = url.Values{"TableName": {name}, "Name": {`newCol`},
		"Type": {"varchar"}, "Index": {"0"}, "UpdatePerm": {"true"}, "ReadPerm": {"true"}}
	assert.NoError(t, postTx(`NewColumn`, &form))
	form = url.Values{"TableName": {name}, "Name": {`newCol`},
		"UpdatePerm": {"true"}, "ReadPerm": {"true"}}
	assert.NoError(t, postTx(`EditColumn`, &form))
}

func TestNewTable(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	name := randName(`tbl`)
	form := url.Values{"Name": {`1_` + name}, "ApplicationId": {"1"}, "Columns": {`[{"name":"MyName","type":"varchar", 
		"conditions":"true"},
	  {"name":"Name", "type":"varchar","index": "0", "conditions":"{\"read\":\"true\",\"update\":\"true\"}"}]`},
		"Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	assert.NoError(t, postTx(`NewTable`, &form))

	form = url.Values{"TableName": {`1_` + name}, "Name": {`newCol`},
		"Type": {"varchar"}, "Index": {"0"}, "Permissions": {"true"}}
	assert.NoError(t, postTx(`NewColumn`, &form))

	form = url.Values{`Value`: {`contract sub` + name + ` {
		action {
			DBInsert("1_` + name + `", {"name": "ok"})
			DBUpdate("1_` + name + `", 1, {"name": "test value"} )
			$result = DBFind("1_` + name + `").Columns("name").WhereId(1).One("name")
		}
	}`}, `Conditions`: {`true`}, "ApplicationId": {"1"}}
	assert.NoError(t, postTx(`NewContract`, &form))

	_, msg, err := postTxResult(`sub`+name, &url.Values{})
	assert.NoError(t, err)
	assert.Equal(t, msg, "test value")

	form = url.Values{"Name": {name}, "ApplicationId": {"1"}, "Columns": {`[{"name":"MyName","type":"varchar", "index": "1", 
	  "conditions":"true"},
	{"name":"Amount", "type":"number","index": "0", "conditions":"true"},
	{"name":"Doc", "type":"json","index": "0", "conditions":"true"},	
	{"name":"Active", "type":"character","index": "0", "conditions":"true"}]`},
		"Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	assert.NoError(t, postTx(`NewTable`, &form))

	assert.EqualError(t, postTx(`NewTable`, &form), fmt.Sprintf(`{"type":"panic","error":"table %s exists"}`, name))

	form = url.Values{"Name": {name},
		"Permissions": {`{"insert": "ContractConditions(\"MainCondition\")",
				"update" : "true", "new_column": "ContractConditions(\"MainCondition\")"}`}}
	assert.NoError(t, postTx(`EditTable`, &form))

	form = url.Values{"TableName": {name}, "Name": {`newDoc`},
		"Type": {"json"}, "Index": {"0"}, "Permissions": {"true"}}
	assert.NoError(t, postTx(`NewColumn`, &form))

	form = url.Values{"TableName": {name}, "Name": {`newCol`},
		"Type": {"varchar"}, "Index": {"0"}, "Permissions": {"true"}}
	assert.NoError(t, postTx(`NewColumn`, &form))

	err = postTx(`NewColumn`, &form)
	if err.Error() != `{"type":"panic","error":"column newcol exists"}` {
		t.Error(err)
		return
	}
	form = url.Values{"TableName": {name}, "Name": {`newCol`},
		"Permissions": {"ContractConditions(\"MainCondition\")"}}
	assert.NoError(t, postTx(`EditColumn`, &form))

	upname := strings.ToUpper(name)
	form = url.Values{"TableName": {upname}, "Name": {`UPCol`},
		"Type": {"varchar"}, "Index": {"0"}, "Permissions": {"true"}}
	assert.NoError(t, postTx(`NewColumn`, &form))

	form = url.Values{"TableName": {upname}, "Name": {`upCOL`},
		"Permissions": {"ContractConditions(\"MainCondition\")"}}
	assert.NoError(t, postTx(`EditColumn`, &form))

	form = url.Values{"Name": {upname},
		"Permissions": {`{"insert": "ContractConditions(\"MainCondition\")", 
			"update" : "true", "new_column": "ContractConditions(\"MainCondition\")"}`}}
	assert.NoError(t, postTx(`EditTable`, &form))

	var ret tablesResult
	assert.NoError(t, sendGet(`tables`, nil, &ret))
}

type invalidPar struct {
	Name  string
	Value string
}

func TestUpdatePlatformParam(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	form := url.Values{"Name": {`max_columns`}, "Value": {`49`}}
	assert.NoError(t, postTx(`UpdatePlatformParam`, &form))

	var sysList paramsResult
	assert.NoError(t, sendGet(`systemparams?names=max_columns`, nil, &sysList))
	assert.Len(t, sysList.List, 1)
	assert.Equal(t, "49", sysList.List[0].Value)

	name := randName(`test`)
	form = url.Values{"Name": {name}, "Value": {`contract ` + name + ` {
		action { 
			var costlen int
			costlen = SysParamInt("price_exec_len") + 1
			UpdatePlatformParam("Name,Value","max_columns","51")
			DBUpdatePlatformParam("price_exec_len", Str(costlen), "true" )
			if SysParamInt("price_exec_len") != costlen {
				error "Incorrect updated value"
			}
			DBUpdatePlatformParam("max_indexes", "4", "false" )
		}
		}`}, "ApplicationId": {"1"},
		"Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx("NewContract", &form))

	err := postTx(name, &form)
	if err != nil {
		assert.EqualError(t, err, `{"type":"panic","error":"Access denied"}`)
	}

	assert.NoError(t, sendGet(`systemparams?names=max_columns,max_indexes`, nil, &sysList))
	if len(sysList.List) != 2 || !((sysList.List[0].Value == `51` && sysList.List[1].Value == `4`) ||
		(sysList.List[0].Value == `4` && sysList.List[1].Value == `51`)) {
		t.Error(`Wrong max_column or max_indexes value`)
		return
	}
	err = postTx(name, &form)
	if err == nil || err.Error() != `{"type":"panic","error":"Access denied"}` {
		t.Error(`incorrect access to system parameter`)
		return
	}

	notvalid := []invalidPar{
		{`gap_between_blocks`, `100000`},
		{`rollback_blocks`, `-1`},
		{`price_create_page`, `-20`},
		{`max_block_size`, `0`},
		{`max_fuel_tx`, `20string`},
		{`fuel_rate`, `string`},
		{`fuel_rate`, `[test]`},
		{`fuel_rate`, `[["name", "100"]]`},
		{`taxes_wallet`, `[["1", "0"]]`},
		{`taxes_wallet`, `[{"1", "50"}]`},
		{`honor_nodes`, `[["", "http://127.0.0.1", "100", "c1a9e7b2fb8cea2a272e183c3e27e2d59a3ebe613f51873a46885c9201160bd263ef43b583b631edd1284ab42483712fd2ccc40864fe9368115ceeee47a7"]]`},
		{`honor_nodes`, `[["127.0.0.1", "", "100", "c1a9e7b2fb8cea2a272e183c3e27e2d59a3ebe613f51873a46885c9201160bd263ef43b583b631edd1284ab42483712fd2ccc40864fe9368115ceeee47a7c7d0"]]`},
		{`honor_nodes`, `[["127.0.0.1", "http://127.0.0.1", "0", "c1a9e7b2fb8cea2a272e183c3e27e2d59a3ebe613f51873a46885c9201160bd263ef43b583b631edd1284ab42483712fd2ccc40864fe9368115ceeee47a7c7d0"]]`},
		{"honor_nodes", "[]"},
	}
	for _, item := range notvalid {
		assert.Error(t, postTx(`UpdatePlatformParam`, &url.Values{`Name`: {item.Name}, `Value`: {item.Value}}))
		assert.NoError(t, sendGet(`systemparams?names=`+item.Name, nil, &sysList))
		assert.Len(t, sysList.List, 1, `have got wrong parameter `+item.Name)

		if len(sysList.List[0].Value) == 0 {
			continue
		}

		err = postTx(`UpdatePlatformParam`, &url.Values{`Name`: {item.Name}, `Value`: {sysList.List[0].Value}})
		assert.NoError(t, err, item.Name, sysList.List[0].Value, sysList.List[0])
	}
}

func TestUpdateHonorNodesWithEmptyArray(t *testing.T) {
	require.NoErrorf(t, keyLogin(1), "on login")

	byteNodes := `[{"tcp_address":"127.0.0.1:7078", "api_address":"https://127.0.0.1:7079", "key_id":"-3122230976936134914", "public_key":"d512e7bbaaa8889e2e471d730bbae663bd291a345153ff34d1d9896e36832408eb9f238deca8d410aeb282ff8547ba3f056c5b2a64e2d0b03928e6dd1336e918"},
	{"tcp_address":"127.0.0.1:7080", "api_address":"https://127.0.0.1:7081", "key_id":"-3928816940965469512", "public_key":"9fdf51cd74e3a03fbe776a7122e2f28e3d560467d96a624296656a3a2120653e6347572a50693077cc8b8309ea1ea4a33cb84b9e62874a2d762aca85fad84bf7"}]`
	form := &url.Values{
		"Name":  {"honor_nodes"},
		"Value": {string(byteNodes)},
	}

	require.NoError(t, postTx(`UpdatePlatformParam`, form))
}

/*
func TestHelper_InsertNodeKey(t *testing.T) {
	require.NoErrorf(t, keyLogin(1), "on login")

	form := url.Values{
		`Value`: {`contract InsertNodeKey {
			data {
				KeyID string
				PubKey string
			}
			conditions {}
			action {
				DBInsert("keys", {id: $KeyID, pub: $PubKey,amount: "100000000000000000000"})
			}
		}`},
		`ApplicationId`: {`1`},
		`Conditions`:    {`true`},
	}
	if err := postTx(`NewContract`, &form); err != nil {
		t.Error(err)
		return
	}

	form = url.Values{
		`KeyID`:  {"-3928816940965469512"},
		`PubKey`: {"704dfabedb65099a8f05f9e20a2e2f04da2e2b4fc9fd8a5a487278bd1212a020a3b469c4756e6f3fc4f7162373e8da576085fb840a8c666d58085e631be501d6"},
	}

	if err := postTx(`InsertNodeKey`, &form); err != nil {
		t.Error(err)
		return
	}
}
*/
func TestValidateConditions(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}

	baseForm := url.Values{"Id": {"1"}, "Value": {"Test"}, "Conditions": {"incorrectConditions"}}
	contracts := map[string]url.Values{
		"EditContract":  baseForm,
		"EditParameter": baseForm,
		"EditMenu":      baseForm,
		"EditPage":      {"Id": {"1"}, "Value": {"Test"}, "Conditions": {"incorrectConditions"}, "Menu": {"1"}},
	}
	expectedErr := `{"type":"panic","error":"unknown identifier incorrectConditions"}`

	for contract, form := range contracts {
		err := postTx(contract, &form)
		if err.Error() != expectedErr {
			t.Errorf("contract %s expected '%s' got '%s'", contract, expectedErr, err)
			return
		}
	}
}

func TestPartitialEdit(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	name := randName(`part`)
	form := url.Values{"Name": {name}, "Value": {"Span(Original text)"},
		"Menu": {"original_menu"}, "ApplicationId": {"1"}, "Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`NewPage`, &form))

	var retList listResult
	assert.NoError(t, sendGet(`list/pages`, nil, &retList))

	idItem := strconv.FormatInt(retList.Count, 10)
	value := `Span(Temp)`
	menu := `temp_menu`
	assert.NoError(t, postTx(`EditPage`, &url.Values{
		"Id":    {idItem},
		"Value": {value},
		"Menu":  {menu},
	}))

	var ret rowResult
	assert.NoError(t, sendGet(`row/pages/`+idItem, nil, &ret))
	assert.Equal(t, value, ret.Value["value"])
	assert.Equal(t, menu, ret.Value["menu"])

	value = `Span(Updated)`
	menu = `default_menu`
	conditions := `true`
	assert.NoError(t, postTx(`EditPage`, &url.Values{"Id": {idItem}, "Value": {value}}))
	assert.NoError(t, postTx(`EditPage`, &url.Values{"Id": {idItem}, "Menu": {menu}}))
	assert.NoError(t, postTx(`EditPage`, &url.Values{"Id": {idItem}, "Conditions": {conditions}}))
	assert.NoError(t, sendGet(`row/pages/`+idItem, nil, &ret))
	assert.Equal(t, value, ret.Value["value"])
	assert.Equal(t, menu, ret.Value["menu"])

	form = url.Values{"Name": {name}, "Value": {`MenuItem(One)`}, "Title": {`My Menu`},
		"ApplicationId": {"1"}, "Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`NewMenu`, &form))
	assert.NoError(t, sendGet(`list/menu`, nil, &retList))
	idItem = strconv.FormatInt(retList.Count, 10)
	value = `MenuItem(Two)`
	assert.NoError(t, postTx(`EditMenu`, &url.Values{"Id": {idItem}, "Value": {value}}))
	assert.NoError(t, postTx(`EditMenu`, &url.Values{"Id": {idItem}, "Conditions": {conditions}}))
	assert.NoError(t, sendGet(`row/menu/`+idItem, nil, &ret))
	assert.Equal(t, value, ret.Value["value"])
	assert.Equal(t, conditions, ret.Value["conditions"])

	form = url.Values{"Name": {name}, "Value": {`Span(Snippet)`},
		"ApplicationId": {"1"}, "Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`NewSnippet`, &form))
	assert.NoError(t, sendGet(`list/snippets`, nil, &retList))
	idItem = strconv.FormatInt(retList.Count, 10)
	value = `Span(Updated block)`
	assert.NoError(t, postTx(`EditSnippet`, &url.Values{"Id": {idItem}, "Value": {value}}))
	assert.NoError(t, postTx(`EditSnippet`, &url.Values{"Id": {idItem}, "Conditions": {conditions}}))
	assert.NoError(t, sendGet(`row/snippets/`+idItem, nil, &ret))
	assert.Equal(t, value, ret.Value["value"])
	assert.Equal(t, conditions, ret.Value["conditions"])
}

func TestContractEdit(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	name := randName(`part`)
	form := url.Values{"Value": {`contract ` + name + ` {
		    action {
				$result = "before"
			}
		}`}, "ApplicationId": {"1"},
		"Conditions": {`ContractConditions("MainCondition")`}}
	err := postTx(`NewContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	var retList listResult
	err = sendGet(`list/contracts`, nil, &retList)
	if err != nil {
		t.Error(err)
		return
	}
	idItem := strconv.FormatInt(retList.Count, 10)
	value := `contract ` + name + ` {
		action {
			$result = "after"
		}
	}`
	conditions := `true`
	wallet := "1231234123412341230"
	err = postTx(`EditContract`, &url.Values{"Id": {idItem}, "Value": {value}})
	if err != nil {
		t.Error(err)
		return
	}
	err = postTx(`EditContract`, &url.Values{"Id": {idItem}, "Conditions": {conditions},
		"WalletId": {wallet}})
	if err != nil {
		t.Error(err)
		return
	}
	var ret rowResult
	err = sendGet(`row/contracts/`+idItem, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	if ret.Value["value"] != value || ret.Value["conditions"] != conditions ||
		ret.Value["wallet_id"] != wallet {
		t.Errorf(`wrong parameters of contract`)
		return
	}
	_, msg, err := postTxResult(name, &url.Values{})
	if err != nil {
		t.Error(err)
		return
	}
	if msg != "after" {
		t.Errorf(`the wrong result of the contract %s`, msg)
	}
}

func TestDelayedContracts(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}

	form := url.Values{
		"Contract":   {"UnknownContract"},
		"EveryBlock": {"10"},
		"Limit":      {"2"},
		"Conditions": {"true"},
	}
	err := postTx("NewDelayedContract", &form)
	assert.EqualError(t, err, `{"type":"error","error":"Unknown contract @1UnknownContract"}`)

	form.Set("Contract", "MainCondition")
	err = postTx("NewDelayedContract", &form)
	assert.NoError(t, err)

	form.Set("BlockID", "1")
	err = postTx("NewDelayedContract", &form)
	assert.EqualError(t, err, `{"type":"error","error":"The blockID must be greater than the current blockID"}`)

	form = url.Values{
		"Id":         {"1"},
		"Contract":   {"MainCondition"},
		"EveryBlock": {"10"},
		"Conditions": {"true"},
		"Deleted":    {"1"},
	}
	err = postTx("EditDelayedContract", &form)
	assert.NoError(t, err)
}

func TestJSON(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	contract := randName("JSONEncode")
	assert.NoError(t, postTx("NewContract", &url.Values{
		"Value": {`contract ` + contract + ` {
			action {
				var a array, m map
				m["k1"] = 1
				m["k2"] = 2
				a[0] = m
				a[1] = m

				info JSONEncode(a)
			}
		}`}, "ApplicationId": {"1"},
		"Conditions": {"true"},
	}))
	assert.EqualError(t, postTx(contract, &url.Values{}), `{"type":"info","error":"[{\"k1\":1,\"k2\":2},{\"k1\":1,\"k2\":2}]"}`)

	contract = randName("JSONDecode")
	assert.NoError(t, postTx("NewContract", &url.Values{
		"Value": {`contract ` + contract + ` {
			data {
				Input string
			}
			action {
				info Sprintf("%v", JSONDecode($Input))
			}
		}`}, "ApplicationId": {"1"},
		"Conditions": {"true"},
	}))

	cases := []struct {
		source string
		result string
	}{
		{`"test"`, `{"type":"info","error":"test"}`},
		{`["test"]`, `{"type":"info","error":"[test]"}`},
		{`{"test":1}`, `{"type":"info","error":"map[test:1]"}`},
		{`[{"test":1}]`, `{"type":"info","error":"[map[test:1]]"}`},
		{`{"test":1`, `{"type":"panic","error":"unexpected end of JSON input"}`},
	}

	for _, v := range cases {
		assert.EqualError(t, postTx(contract, &url.Values{"Input": {v.source}}), v.result)
	}
}

func TestBytesToString(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	contract := randName("BytesToString")
	assert.NoError(t, postTx("NewContract", &url.Values{
		"Value": {`contract ` + contract + ` {
			data {
				Data bytes
			}
			action {
				$result = BytesToString($Data)
			}
		}`},
		"Conditions":    {"true"},
		"ApplicationId": {"1"},
	}))

	content := crypto.RandSeq(100)
	_, res, err := postTxResult(contract, &contractParams{
		"Data": []byte(content),
	})
	assert.NoError(t, err)
	assert.Equal(t, content, res)
}

func TestMoneyDigits(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	contract := randName("MoneyDigits")
	assert.NoError(t, postTx("NewContract", &url.Values{
		"Value": {`contract ` + contract + ` {
			data {
				Value money
			}
			action {
				$result = $Value
			}
		}`},
		"ApplicationId": {"1"},
		"Conditions":    {"true"},
	}))

	_, result, err := postTxResult(contract, &url.Values{
		"Value": {"1"},
	})
	assert.NoError(t, err)

	d := decimal.New(1, int32(consts.MoneyDigits))
	assert.Equal(t, d.StringFixed(0), result)
}

func TestMemoryLimit(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	contract := randName("Contract")
	assert.NoError(t, postTx("NewContract", &url.Values{
		"Value": {`contract ` + contract + ` {
			data {
				Count int "optional"
			}
			action {
				var a array
				while (true) {
					$Count = $Count + 1
					a[Len(a)] = JSONEncode(a)
				}
			}
		}`},
		"ApplicationId": {"1"},
		"Conditions":    {"true"},
	}))

	assert.EqualError(t, postTx(contract, &url.Values{}), `{"type":"panic","error":"Memory limit exceeded"}`)
}

func TestStack(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	parent := randName("Parent")
	child := randName("Child")

	assert.NoError(t, postTx("NewContract", &url.Values{
		"Value": {`contract ` + child + ` {
			action {
				$result = $stack
			}
		}`},
		"ApplicationId": {"1"},
		"Conditions":    {"true"},
	}))

	assert.NoError(t, postTx("NewContract", &url.Values{
		"Value": {`contract ` + parent + ` {
			action {
				var arr array
				arr[0] = $stack
				arr[1] = ` + child + `()
				$result = arr
			}
		}`},
		"ApplicationId": {"1"},
		"Conditions":    {"true"},
	}))

	_, res, err := postTxResult(parent, &url.Values{})
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("[[@1%s] [@1%[1]s @1%s]]", parent, child), res)
}

func TestPageHistory(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	name := randName(`page`)
	value := `P(test,test paragraph)`

	form := url.Values{"Name": {name}, "Value": {value}, "ApplicationId": {`1`},
		"Menu": {"default_menu"}, "Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`NewPage`, &form))

	var ret listResult
	assert.NoError(t, sendGet(`list/pages`, nil, &ret))
	id := strconv.FormatInt(ret.Count, 10)
	assert.NoError(t, postTx(`EditPage`, &url.Values{"Id": {id}, "Value": {"Div(style){ok}"}}))
	assert.NoError(t, postTx(`EditPage`, &url.Values{"Id": {id}, "Conditions": {"true"}}))

	form = url.Values{"Name": {randName(`menu`)}, "Value": {`MenuItem(First)MenuItem(Second)`},
		"ApplicationId": {`1`}, "Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`NewMenu`, &form))

	assert.NoError(t, sendGet(`list/menu`, nil, &ret))
	idmenu := strconv.FormatInt(ret.Count, 10)
	assert.NoError(t, postTx(`EditMenu`, &url.Values{"Id": {idmenu}, "Conditions": {"true"}}))
	assert.NoError(t, postTx(`EditMenu`, &url.Values{"Id": {idmenu}, "Value": {"MenuItem(Third)"}}))
	assert.NoError(t, postTx(`EditMenu`, &url.Values{"Id": {idmenu},
		"Value": {"MenuItem(Third)"}, "Conditions": {"false"}}))

	form = url.Values{"Value": {`contract C` + name + `{ action {}}`},
		"ApplicationId": {`1`}, "Conditions": {`ContractConditions("MainCondition")`}}
	_, idCont, err := postTxResult(`NewContract`, &form)
	assert.NoError(t, err)
	assert.NoError(t, postTx(`EditContract`, &url.Values{"Id": {idCont},
		"Value": {`contract C` + name + `{ action {Println("OK")}}`}, "Conditions": {"true"}}))

	form = url.Values{`Value`: {`contract Get` + name + ` {
		data {
			IdPage int
			IdMenu int
			IdCont int
		}
		action {
			var ret array
			ret = GetHistory("pages", $IdPage)
			$result = Str(Len(ret))
			ret = GetHistory("menu", $IdMenu)
			$result = $result + Str(Len(ret))
			ret = GetHistory("contracts", $IdCont)
			$result = $result + Str(Len(ret))
		}
	}`}, "ApplicationId": {`1`}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	form = url.Values{`Value`: {`contract GetRow` + name + ` {
		data {
			IdPage int
		}
		action {
			var ret array
			var row got map
			ret = GetHistory("pages", $IdPage)
			row = ret[1]
			got = GetHistoryRow("pages", $IdPage, Int(row["id"]))
			if got["block_id"] != row["block_id"] {
				error "GetPageHistory"
			}
		}
	}`}, "ApplicationId": {`1`}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	_, msg, err := postTxResult(`Get`+name, &url.Values{"IdPage": {id}, "IdMenu": {idmenu},
		"IdCont": {idCont}})
	assert.NoError(t, err)
	assert.Equal(t, `231`, msg)

	form = url.Values{"Name": {name + `1`}, "Value": {value}, "ApplicationId": {`1`},
		"Menu": {"default_menu"}, "Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`NewPage`, &form))

	assert.NoError(t, postTx(`Get`+name, &url.Values{"IdPage": {converter.Int64ToStr(
		converter.StrToInt64(id) + 1)}, "IdMenu": {idmenu}, "IdCont": {idCont}}))

	assert.EqualError(t, postTx(`Get`+name, &url.Values{"IdPage": {`1000000`}, "IdMenu": {idmenu},
		"IdCont": {idCont}}), `{"type":"panic","error":"Record has not been found"}`)

	assert.NoError(t, postTx(`GetRow`+name, &url.Values{"IdPage": {id}}))

	var retTemp contentResult
	assert.NoError(t, sendPost(`content`, &url.Values{`template`: {fmt.Sprintf(`GetHistory(MySrc, "pages", %s)`,
		id)}}, &retTemp))

	if len(RawToString(retTemp.Tree)) < 400 {
		t.Error(fmt.Errorf(`wrong tree %s`, RawToString(retTemp.Tree)))
	}
}
