/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRead(t *testing.T) {
	var (
		retCont contentResult
	)

	assert.NoError(t, keyLogin(1))

	name := randName(`tbl`)
	form := url.Values{"Name": {name}, "ApplicationId": {`1`},
		"Columns": {`[{"name":"my","type":"varchar", "index": "1", 
	  "conditions":"true"},
	{"name":"amount", "type":"number","index": "0", "conditions":"{\"update\":\"true\", \"read\":\"true\"}"},
	{"name":"active", "type":"character","index": "0", "conditions":"{\"update\":\"true\", \"read\":\"false\"}"}]`},
		"Permissions": {`{"insert": "true", "update" : "true", "read": "true", "new_column": "true"}`}}
	assert.NoError(t, postTx(`NewTable`, &form))

	contList := []string{`contract %s {
		action {
			DBInsert("%[1]s", {my: "Alex",amount: 100})
			DBInsert("%[1]s", {my: "Alex 2",amount: 13300})
			DBInsert("%[1]s", {my: "Mike",amount: 0})
			DBInsert("%[1]s", {my: "Mike 2",amount: 25500})
			DBInsert("%[1]s", {my: "John Mike", amount: 0})
			DBInsert("%[1]s", {my: "Serena Martin",amount:777})
		}
	}`,
		`contract Get%s {
		action {
			var row array
			row = DBFind("%[1]s").Where({id:[{$gte: 2},{"$lte":5}]})
		}
	}`,
		`contract GetOK%s {
		action {
			var row array
			row = DBFind("%[1]s").Columns("my,amount").Where({id:[{$gte: 2},{"$lte":5}]})
		}
	}`,
		`contract GetData%s {
		action {
			var row array
			row = DBFind("%[1]s").Columns("active").Where({id:[{$gte: 2},{"$lte":5}]})
		}
	}`,
		`func ReadFilter%s bool {
				var i int
				var row map
				while i < Len($data) {
					row = $data[i]
					if i == 1 || i == 3 {
						row["my"] = "No name"
						$data[i] = row
					}
					i = i+ 1
				}
				return true
			}`,
	}
	for _, contract := range contList {
		form = url.Values{"Value": {fmt.Sprintf(contract, name)}, "ApplicationId": {`1`},
			"Conditions": {`true`}}
		assert.NoError(t, postTx(`NewContract`, &form))
	}
	assert.NoError(t, postTx(name, &url.Values{}))

	assert.EqualError(t, postTx(`GetData`+name, &url.Values{}), `{"type":"panic","error":"Access denied"}`)
	assert.NoError(t, sendPost(`content`, &url.Values{`template`: {
		`DBFind(` + name + `, src).Limit(2)`}}, &retCont))

	if strings.Contains(RawToString(retCont.Tree), `active`) {
		t.Errorf(`wrong tree %s`, RawToString(retCont.Tree))
		return
	}

	assert.NoError(t, postTx(`GetOK`+name, &url.Values{}))

	assert.NoError(t, postTx(`EditColumn`, &url.Values{`TableName`: {name}, `Name`: {`active`},
		"UpdatePerm": {"true"}, "ReadPerm": {"true" /*"ContractConditions(\"MainCondition\")"*/},
	}))
	var ret listResult
	assert.NoError(t, sendGet(`list/`+name, nil, &ret))

	assert.NoError(t, postTx(`Get`+name, &url.Values{}))

	assert.NoError(t, sendPost(`content`, &url.Values{`template`: {
		`DBFind(` + name + `, src).Limit(2)`}}, &retCont))
	if !strings.Contains(RawToString(retCont.Tree), `Alex 2`) {
		t.Errorf(`wrong tree %s`, RawToString(retCont.Tree))
		return
	}

	form = url.Values{"Name": {name}, "InsertPerm": {`ContractConditions("MainCondition")`},
		"UpdatePerm": {"true"}, "ReadPerm": {`false`}, "NewColumnPerm": {`true`}}
	assert.NoError(t, postTx(`EditTable`, &form))
	assert.EqualError(t, postTx(`GetOK`+name, &url.Values{}), `{"type":"panic","error":"Access denied"}`)

	assert.EqualError(t, sendGet(`list/`+name, nil, &ret), `400 {"error":"E_SERVER","msg":"Access denied"}`)

	form = url.Values{"Name": {name}, "InsertPerm": {`ContractConditions("MainCondition")`},
		"UpdatePerm": {"true"}, "FilterPerm": {`ReadFilter` + name + `()`},
		"NewColumnPerm": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`EditTable`, &form))

	var tableInfo tableResult
	assert.NoError(t, sendGet(`table/`+name, nil, &tableInfo))
	assert.Equal(t, `ReadFilter`+name+`()`, tableInfo.Filter)

	assert.NoError(t, sendPost(`content`, &url.Values{`template`: {
		`DBFind(` + name + `, src).Limit(2)`}}, &retCont))
	if !strings.Contains(RawToString(retCont.Tree), `No name`) {
		t.Errorf(`wrong tree %s`, RawToString(retCont.Tree))
		return
	}
}
