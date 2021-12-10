/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTables(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	var ret tablesResult
	err := sendGet(`tables`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	if int64(ret.Count) < 7 {
		t.Error(fmt.Errorf(`The number of tables %d < 7`, ret.Count))
		return
	}
}

func TestTable(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	var ret tableResult
	err := sendGet(`table/keys`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	if len(ret.Columns) == 0 {
		t.Errorf(`Wrong result columns`)
		return
	}
	err = sendGet(`table/contracts`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
}

func TestTableName(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	form := url.Values{"Name": {`test`}, "Columns": {`[{"name":"MyName","type":"varchar", "index": "0", 
		"conditions":{"update":"true", "read":"true"}}]`}, "ApplicationId": {"1"},
		"Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	assert.EqualError(t, postTx(`NewTable`, &form),
		`{"type":"panic","error":"Name test must only contain latin, digit and '_', '-' characters"}`)

	form = url.Values{"Name": {`latin`}, "Columns": {`[{"name":"test","type":"varchar", "index": "0", 
		"conditions":{"update":"true", "read":"true"}}]`}, "ApplicationId": {"1"},
		"Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	assert.EqualError(t, postTx(`NewTable`, &form),
		`{"type":"panic","error":"Name latin must only contain latin, digit and '_', '-' characters"}`)

	name := randName(`tbl`)
	form = url.Values{"Name": {`tbl-` + name}, "Columns": {`[{"name":"MyName","type":"varchar", "index": "0", 
	  "conditions":{"update":"true", "read":"true"}}]`}, "ApplicationId": {"100"},
		"Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	err := postTx(`NewTable`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	form = url.Values{"Name": {name}, "Value": {`contract ` + name + ` {
		action { 
			DBInsert("tbl-` + name + `", {"MyName": "test"})
			DBUpdate("tbl-` + name + `", 1, {"MyName": "New test"})
		}}`}, "ApplicationId": {`100`}, "Conditions": {`ContractConditions("MainCondition")`}}
	err = postTx("NewContract", &form)
	if err != nil {
		t.Error(err)
		return
	}
	err = postTx(name, &url.Values{})
	if err != nil {
		t.Error(err)
		return
	}
	var ret tableResult
	err = sendGet(`table/tbl-`+name, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	if len(ret.Columns) == 0 || ret.AppID != `100` {
		t.Errorf(`wrong table columns or app_id`)
		return
	}
	var retList listResult
	err = sendGet(`list/tbl-`+name, nil, &retList)
	if err != nil {
		t.Error(err)
		return
	}
	if retList.Count != 1 {
		t.Errorf(`wrong table count`)
		return
	}
	forTest := tplList{
		{`DBFind(tbl-` + name + `,my).Columns("id,myname").WhereId(1)`,
			`[{"tag":"dbfind","attr":{"columns":["id","myname"],"data":[["1","New test"]],"name":"tbl-` + name + `","source":"my","types":["text","text"],"whereid":"1"}}]`},
	}
	var retCont contentResult
	for _, item := range forTest {
		err := sendPost(`content`, &url.Values{`template`: {item.input}}, &retCont)
		if err != nil {
			t.Error(err)
			return
		}
		if RawToString(retCont.Tree) != item.want {
			t.Error(fmt.Errorf(`wrong tree %s != %s`, RawToString(retCont.Tree), item.want))
			return
		}
	}
}

func TestJSONTable(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	name := randName(`json`)
	form := url.Values{"Name": {name}, "Columns": {`[{"name":"MyName","type":"varchar", "index": "0", 
		"conditions":"true"}, {"name":"Doc", "type":"json","index": "0", "conditions":"true"}]`},
		"ApplicationId": {`1`}, "Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	assert.NoError(t, postTx(`NewTable`, &form))

	checkGet := func(want string) {
		_, msg, err := postTxResult(name+`Get`, &url.Values{"Id": {`2`}})
		assert.NoError(t, err)
		assert.Equal(t, want, msg)
	}

	form = url.Values{"Name": {name}, "Value": {`contract ` + name + ` {
		action { 
			var ret1, ret2 int
			ret1 = DBInsert("` + name + `", {MyName: "test",Doc: "{\"type\": \"0\"}"})
			var mydoc map
			mydoc["type"] = "document"
			mydoc["ind"] = 2
			mydoc["check"] = "99"
			mydoc["doc"] = "Some text."
			ret2 = DBInsert("` + name + `", {MyName: "test2",Doc: mydoc})
			DBInsert("` + name + `", {MyName: "test3",Doc: "{\"title\": {\"name\":\"Test att\",\"text\":\"low\"}}"})
			DBInsert("` + name + `", {MyName: "test4",doc: "{\"languages\": {\"arr_id\":{\"1\":\"0\",\"2\":\"0\",\"3\":\"0\"}}}"})
			DBInsert("` + name + `", {MyName: "test5",Doc: "{\"app_id\": \"33\"}"})
		}}`}, "ApplicationId": {`1`},
		"Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx("NewContract", &form))

	form = url.Values{"Name": {name}, "Value": {`contract ` + name + `Get {
			data {
				Id int
			}
			action {
				var ret map
				var list array
				var one out tmp where empty string
				ret = DBFind("` + name + `").Columns("Myname,doc,Doc->Ind").WhereId($Id).Row()
				out = ret["doc.ind"]
				out = out + DBFind("` + name + `").Columns("myname,doc->Type").WhereId($Id).One("Doc->type")
				list = DBFind("` + name + `").Columns(["Myname", "doc", "Doc->Ind"]).Where({"Doc->ind": "101"})
				out = out + Str(Len(list))
				tmp = DBFind("` + name + `").Columns("doc->title->name").WhereId(3).One("doc->title->name")
				where = DBFind("` + name + `").Columns("doc->title->name").Where({"doc->title->text":"low"}).One("doc->title->name")
				one = DBFind("` + name + `").Where({"doc->title->text":"low"}).One("doc->title->text")
				empty = DBFind("` + name + `").WhereId(4).One("doc->languages->arr_id->2")
				$result = out + Str(DBFind("` + name + `").WhereId($Id).One("doc->check")) + tmp + where +one + empty
			}
		}`}, "ApplicationId": {`1`},
		"Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx("NewContract", &form))

	form = url.Values{"Name": {name}, "Value": {`contract ` + name + `Upd {
		action {
			DBUpdate("` + name + `", 1, {"Doc": "{\"type\": \"doc\", \"ind\": \"3\", \"check\": \"33\"}"})
			var mydoc map
			mydoc["type"] = "doc"
			mydoc["doc"] = "Some test text."
			DBUpdate("` + name + `", 2, {"myname": "test3", "Doc": mydoc})
		}}`}, "ApplicationId": {`1`},
		"Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx("NewContract", &form))

	form = url.Values{"Name": {name}, "Value": {`contract ` + name + `UpdOne {
			data {
				Type int
			}
			action {
				DBUpdate("` + name + `", 1, {"myname": "New name", "Doc->Ind": $Type,
				    "Doc->type": "new\"doc\" val"})
				DBUpdate("` + name + `", 2, {"myname": "New name","Doc->Ind": $Type,
				   "Doc->type": "new\"doc\""})
				DBUpdate("` + name + `", 3, {"doc->flag": "Flag","doc->sub": 100})
				DBUpdate("` + name + `", 3, {"doc->temp":"Temp"})
		  }}
		`}, "ApplicationId": {`1`},
		"Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx("NewContract", &form))
	assert.NoError(t, postTx(name, &url.Values{}))

	checkGet(`2document099Test attTest attlow0`)

	assert.NoError(t, postTx(name+`Upd`, &url.Values{}))
	checkGet(`doc0Test attTest attlow0`)

	assert.NoError(t, postTx(name+`UpdOne`, &url.Values{"Type": {"101"}}))
	checkGet(`101new"doc"2Test attTest attlow0`)

	form = url.Values{"Name": {`res` + name}, "Value": {`contract res` + name + ` {
		data {
			Id int
		}
		action { 
			$result = DBFind("contracts").WhereId($Id).Row()
		}}`}, "ApplicationId": {`1`},
		"Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx("NewContract", &form))

	form = url.Values{"Name": {`run` + name}, "Value": {`contract run` + name + ` {
		action { 
			$temp = res` + name + `("Id",10)
			$result = $temp["id"]
		}}`}, "ApplicationId": {`1`},
		"Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx("NewContract", &form))

	_, msg, err := postTxResult(`run`+name, &url.Values{})
	assert.NoError(t, err)
	assert.Equal(t, "10", msg)

	forTest := tplList{
		{`DBFind(` + name + `).Columns("id,doc->app_id").WhereId(5).Vars(buffer)Span(#buffer_doc_app_id#)`,
			`[{"tag":"dbfind","attr":{"columns":["id","doc.app_id"],"data":[["5","33"]],"name":"` + name + `","types":["text","text"],"whereid":"5"}},{"tag":"span","children":[{"tag":"text","text":"33"}]}]`},
		{`DBFind(` + name + `,my).Columns("id").Where({"doc->title->text":"low"})`,
			`[{"tag":"dbfind","attr":{"columns":["id"],"data":[["3"]],"name":"` + name + `","source":"my","types":["text"],"where":"{"doc-\u003etitle-\u003etext":"low"}"}}]`},
		{`DBFind(` + name + `,my).Columns("id,doc->title->name").WhereId(3).Vars(prefix)Div(){#prefix_id# = #prefix_doc_title_name#}`,
			`[{"tag":"dbfind","attr":{"columns":["id","doc.title.name"],"data":[["3","Test att"]],"name":"` + name + `","source":"my","types":["text","text"],"whereid":"3"}},{"tag":"div","children":[{"tag":"text","text":"3 = Test att"}]}]`},
		{`DBFind(` + name + `,my).Columns("id,doc->languages->arr_id").WhereId(4).Custom(aa){Span(#doc.languages.arr_id#)}`,
			`[{"tag":"dbfind","attr":{"columns":["id","doc.languages.arr_id","aa"],"data":[["4","{"1": "0", "2": "0", "3": "0"}","[{"tag":"span","children":[{"tag":"text","text":"{\\"1\\": \\"0\\", \\"2\\": \\"0\\", \\"3\\": \\"0\\"}"}]}]"]],"name":"` + name + `","source":"my","types":["text","text","tags"],"whereid":"4"}}]`},
		{`DBFind(` + name + `,my).Columns("id,doc->title->name").WhereId(3)`,
			`[{"tag":"dbfind","attr":{"columns":["id","doc.title.name"],"data":[["3","Test att"]],"name":"` + name + `","source":"my","types":["text","text"],"whereid":"3"}}]`},
		{`DBFind(` + name + `,my).Columns("doc").WhereId(3)`,
			`[{"tag":"dbfind","attr":{"columns":["doc","id"],"data":[["{"sub": "100", "flag": "Flag", "temp": "Temp", "title": {"name": "Test att", "text": "low"}}","3"]],"name":"` + name + `","source":"my","types":["text","text"],"whereid":"3"}}]`},
		{`DBFind(` + name + `,my).Columns("id,doc,doc->type").Where({doc->ind:101, doc->check:33})`,
			`[{"tag":"dbfind","attr":{"columns":["id","doc","doc.type"],"data":[["1","{"ind": "101", "type": "new\\"doc\\" val", "check": "33"}","new"doc" val"]],"name":"` + name + `","source":"my","types":["text","text","text"],"where":"{doc-\u003eind:101, doc-\u003echeck:33}"}}]`},
		{`DBFind(` + name + `,my).Columns("id,doc,doc->type").WhereId(2).Vars(my)
			Span(#my_id##my_doc_type#)`,
			`[{"tag":"dbfind","attr":{"columns":["id","doc","doc.type"],"data":[["2","{"doc": "Some test text.", "ind": "101", "type": "new\\"doc\\""}","new"doc""]],"name":"` + name + `","source":"my","types":["text","text","text"],"whereid":"2"}},{"tag":"span","children":[{"tag":"text","text":"2new"doc""}]}]`},
		{`DBFind(` + name + `,my).Columns("id,doc->type").WhereId(2)`,
			`[{"tag":"dbfind","attr":{"columns":["id","doc.type"],"data":[["2","new"doc""]],"name":"` + name + `","source":"my","types":["text","text"],"whereid":"2"}}]`},
		{`DBFind(` + name + `,my).Columns("doc->type").Order(id).Custom(mytype, OK:#doc.type#)`,
			`[{"tag":"dbfind","attr":{"columns":["doc.type","id","mytype"],"data":[["new"doc" val","1","[{"tag":"text","text":"OK:new"doc" val"}]"],["new"doc"","2","[{"tag":"text","text":"OK:new"doc""}]"],["","3","[{"tag":"text","text":"OK:NULL"}]"],["","4","[{"tag":"text","text":"OK:NULL"}]"],["","5","[{"tag":"text","text":"OK:NULL"}]"]],"name":"` + name + `","order":"id","source":"my","types":["text","text","tags"]}}]`},
	}
	var ret contentResult
	for i, item := range forTest {
		if i > 100 {
			break
		}
		assert.NoError(t, sendPost(`content`, &url.Values{`template`: {item.input}}, &ret))
		assert.Equal(t, item.want, RawToString(ret.Tree))
	}
}

func TestTableDesc(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	name := randName(`tbl`)
	form := url.Values{"Name": {name}, "Columns": {`[{"name":"desc","type":"varchar", "index": "0", 
	  "conditions":{"update":"true", "read":"true"}}]`}, "ApplicationId": {"1"},
		"Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	assert.NoError(t, postTx(`NewTable`, &form))

	form = url.Values{"Name": {name}, "Value": {`contract ` + name + ` {
		action { 
			DBInsert("` + name + `", {"desc": "test"})
			DBUpdate("` + name + `", 1, {"desc": "new test"})
			$result = DBFind("` + name + `").Columns("desc").WhereId(1).One("desc")
		   var vals map
		   vals = DBRow("pages").Columns("NAME, menu").Where({id:1})
		   $result = $result + vals["name"]
		}}`}, "ApplicationId": {"1"},
		"Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx("NewContract", &form))

	_, msg, err := postTxResult(name, &url.Values{})
	assert.NoError(t, err)
	if msg != `new testdefault_page` {
		t.Errorf(`wrong msg %s`, msg)
	}

	form = url.Values{
		"template": {`DBFind("` + name + `", src1)`},
	}
	var ret contentResult
	assert.NoError(t, sendPost(`content`, &form, &ret))

	if RawToString(ret.Tree) != `[{"tag":"dbfind","attr":{"columns":["id","desc"],"data":[["1","new test"]],"name":"`+name+`","source":"src1","types":["text","text"]}}]` {
		t.Error(fmt.Errorf(`wrong tree %s`, RawToString(ret.Tree)))
		return
	}
}
