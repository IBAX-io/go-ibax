/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/stretchr/testify/assert"
)

type tplItem struct {
	input string
	want  string
}

type tplList []tplItem

func TestAPI(t *testing.T) {
	var (
		ret               contentResult
		retHash, retHash2 hashResult
		err               error
		msg               string
	)

	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	name := randName(`page`)
	value := `Div(,#ecosystem_id#)
	Div(,#key_id#)
	Div(,#role_id#)
	Div(,#isMobile#)`
	form := url.Values{"Name": {name}, "Value": {value}, "ApplicationId": {`1`},
		"Menu": {`default_menu`}, "Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`NewPage`, &form))

	assert.NoError(t, sendPost(`content/hash/`+name, &url.Values{}, &retHash))
	if len(retHash.Hash) != 64 {
		t.Error(`wrong hash ` + retHash.Hash)
		return
	}
	form = url.Values{"Name": {name}, "Value": {`contract ` + name + ` {
		action {
			$result = $key_id
		}}`}, "ApplicationId": {`1`}, "Conditions": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx("NewContract", &form))
	_, msg, err = postTxResult(name, &url.Values{})
	assert.NoError(t, err)

	gAddress = ``
	gPrivate = ``
	gPublic = ``
	gAuth = ``
	assert.NoError(t, sendPost(`content/hash/`+name, &url.Values{`ecosystem`: {`1`}, `keyID`: {msg}, `roleID`: {`0`}},
		&retHash2))
	if retHash.Hash != retHash2.Hash {
		t.Error(`Wrong hash`)
		return
	}
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	err = sendPost(`content/page/default_page`, &url.Values{}, &ret)
	if err != nil {
		t.Error(err)
		return
	}

	for _, item := range forTest {
		err := sendPost(`content`, &url.Values{`template`: {item.input}}, &ret)
		if err != nil {
			t.Error(err)
			return
		}
		if RawToString(ret.Tree) != item.want {
			t.Error(fmt.Errorf("wrong tree \r\n%s != \r\n%s", RawToString(ret.Tree), item.want))
			return
		}
	}
	err = sendPost(`content/page/mypage`, &url.Values{}, &ret)
	if err != nil && err.Error() != `404 {"error":"E_NOTFOUND","msg":"Page not found"}` {
		t.Error(err)
		return
	}
	err = sendPost(`content/menu/default_menu`, &url.Values{}, &ret)
	if err != nil {
		t.Error(err)
		return
	}
}

var forTest = tplList{
	{`DBFind(contracts, src).Columns("id").Where({"app_id": 1, ,"id": {"$gt": 2}})`,
		`[{"tag":"text","text":"unexpected comma"}]`},
	{`DBFind(contracts, src).Columns("id").Where({"app_id": 1, "id": {"$gt": 2},})`,
		`[{"tag":"text","text":"unexpected comma"}]`},
	{`SetVar(w_filter, ` + "`" + `"id": {"$lt": "2"}` + "`" + `)SetVar(w, {#w_filter# #w_search#})
		  DBFind("contracts", src).Columns("id,name").Where(#w#)Table(src)`,
		`[{"tag":"dbfind","attr":{"columns":["id","name"],"data":[["1","MainCondition"]],"name":"contracts","source":"src","types":["text","text"],"where":"{"id": {"$lt": "2"} }"}},{"tag":"table","attr":{"source":"src"}}]`},
	{`DBFind("contracts", src).Columns("id,name").Where({"id": {"$lt": "2"} })`,
		`[{"tag":"dbfind","attr":{"columns":["id","name"],"data":[["1","MainCondition"]],"name":"contracts","source":"src","types":["text","text"],"where":"{"id": {"$lt": "2"} }"}}]`},
	{`SetVar(where, {"$or": ["name": #poa#,"valueN": #poa#]})
		DBFind(contracts, src).Columns("id").Where(#where#)`, `[{"tag":"text","text":"pq: column "valuen" does not exist in query select "id" from "1_contracts" where ("name" = '' or "valuen" = '') order by id [[]]"}]`},
	{`DBFind("@1roles_participants", src).Where({"ecosystem": #ecosystem_id#, "role->id": {"$in": []}, "member->member_id": #key_id#, "deleted": 0})`, `[{"tag":"dbfind","attr":{"columns":["id","role","member","appointed","date_created","date_deleted","deleted","ecosystem"],"data":[],"name":"@1roles_participants","source":"src","types":[],"where":"{"ecosystem": 1, "role-\u003eid": {"$in": []}, "member-\u003emember_id": 2665397054248150876, "deleted": 0}"}}]`},
	{`DBFind("@1roles_participants").Where({"ecosystem": #ecosystem_id#, "role->id": {"$in": []}, "member->member_id": #key_id#, "deleted": 0}).Vars(v)`, `[{"tag":"dbfind","attr":{"columns":["id","role","member","appointed","date_created","date_deleted","deleted","ecosystem"],"data":[],"name":"@1roles_participants","types":[],"where":"{"ecosystem": 1, "role-\u003eid": {"$in": []}, "member-\u003emember_id": 2665397054248150876, "deleted": 0}"}}]`},
	{`DBFind(@1pages).Where({{id:{$neq:5}}, {id:2}, id:{$neq:6}, $or:[id:6, {id:1}, {id:2}, id:3]}).Columns("id,name").Order(id)`, `[{"tag":"dbfind","attr":{"columns":["id","name"],"data":[["1","developer_index"],["3","notifications"]],"name":"@1pages","order":"id","types":["text","text"],"where":"{{id:{$neq:5}}, {id:2}, id:{$neq:6}, $or:[id:6, {id:1}, {id:2}, id:3]}"}}]`},
	{`DBFind(@1pages).Where({id:[{$neq:5},{$neq:4}, 2], name:{$neq: Edit}}).Columns("id,name")`,
		`[{"tag":"dbfind","attr":{"columns":["id","name"],"data":[["2","developer_index"]],"name":"@1pages","types":["text","text"],"where":"{id:[{$neq:5},{$neq:4}, 2], name:{$neq: Edit}}"}}]`},
	{`DBFind(@1pages).Where({id:3, name: {$neq:EditPage}, $or:[id:1, {id:5}, id:{$neq:2}, id:4]}).Columns("id,name")`, `[{"tag":"dbfind","attr":{"columns":["id","name"],"data":[["3","notifications"]],"name":"@1pages","types":["text","text"],"where":"{id:3, name: {$neq:EditPage}, $or:[id:1, {id:5}, id:{$neq:2}, id:4]}"}}]`},
	{`DBFind(keys).Where("id='#key_id#'").Columns("amount").Vars(amount)`, `[{"tag":"text","text":"Where has wrong format"}]`},
	{`SetVar(val, 123456789)Money(#val#)`, `[{"tag":"text","text":"0.000000000123456789"}]`},
	{`SetVar(coltype, GetColumnType(members, member_name))Div(){#coltype#GetColumnType(none,none)GetColumnType()}`, `[{"tag":"div","children":[{"tag":"text","text":"varchar"}]}]`},
	{`DBFind(parameters, src_par).Columns("id").Order([id]).Where({id:[{$gte:1}, {$lte:3}]}).Count(count)Span(#count#)`,
		`[{"tag":"dbfind","attr":{"columns":["id"],"count":"3","data":[["1"],["2"],["3"]],"name":"parameters","order":"[id]","source":"src_par","types":["text"],"where":"{id:[{$gte:1}, {$lte:3}]}"}},{"tag":"span","children":[{"tag":"text","text":"3"}]}]`},
	{`SetVar(coltype, GetColumnType(members, member_name))Div(){#coltype#GetColumnType(none,none)GetColumnType()}`, `[{"tag":"div","children":[{"tag":"text","text":"varchar"}]}]`},
	{`SetVar(where).(lim,3)DBFind(contracts, src).Columns(id).Order([{id:1}, {name:-1}]).Limit(#lim#).Custom(a){SetVar(where, #where# #id#)}
	Div(){Table(src, "=x")}Div(){Table(src)}Div(){#where#}`,
		`[{"tag":"dbfind","attr":{"columns":["id","a"],"data":[["1","null"],["2","null"],["3","null"]],"limit":"3","name":"contracts","order":"[{id:1}, {name:-1}]","source":"src","types":["text","tags"]}},{"tag":"div","children":[{"tag":"table","attr":{"columns":[{"Name":"x","Title":""}],"source":"src"}}]},{"tag":"div","children":[{"tag":"table","attr":{"source":"src"}}]},{"tag":"div","children":[{"tag":"text","text":" 1 2 3"}]}]`},
	{`If(#isMobile#){Span(Mobile)}.Else{Span(Desktop)}`,
		`[{"tag":"span","children":[{"tag":"text","text":"Desktop"}]}]`},
	{`SetVar(off, 10)DBFind(contracts, src_contracts).Columns("id").Order(id).Limit(2).Offset(#off#).Custom(){}`,
		`[{"tag":"dbfind","attr":{"columns":["id"],"data":[["11"],["12"]],"limit":"2","name":"contracts","offset":"10","order":"id","source":"src_contracts","types":["text"]}}]`},
	{`DBFind(contracts, src_pos).Columns(id).Where({id:[{$gte:1}, {$lte:3}]}).Order(id)
		ForList(src_pos, Index: index){
			Div(list-group-item) {
				DBFind(parameters, src_hol).Columns(id).Where({id: #id#}).Vars("ret")
				SetVar(qq, #ret_id#)
				Div(Body: #index# ForList=#id# DBFind=#ret_id# SetVar=#qq#)  
			}
		}`, `[{"tag":"dbfind","attr":{"columns":["id"],"data":[["1"],["2"],["3"]],"name":"contracts","order":"id","source":"src_pos","types":["text"],"where":"{id:[{$gte:1}, {$lte:3}]}"}},{"tag":"forlist","attr":{"index":"index","source":"src_pos"},"children":[{"tag":"div","attr":{"class":"list-group-item"},"children":[{"tag":"dbfind","attr":{"columns":["id"],"data":[["1"]],"name":"parameters","source":"src_hol","types":["text"],"where":"{id: 1}"}},{"tag":"div","children":[{"tag":"text","text":"1 ForList=1 DBFind=1 SetVar=1"}]}]},{"tag":"div","attr":{"class":"list-group-item"},"children":[{"tag":"dbfind","attr":{"columns":["id"],"data":[["2"]],"name":"parameters","source":"src_hol","types":["text"],"where":"{id: 2}"}},{"tag":"div","children":[{"tag":"text","text":"2 ForList=2 DBFind=2 SetVar=2"}]}]},{"tag":"div","attr":{"class":"list-group-item"},"children":[{"tag":"dbfind","attr":{"columns":["id"],"data":[["3"]],"name":"parameters","source":"src_hol","types":["text"],"where":"{id: 3}"}},{"tag":"div","children":[{"tag":"text","text":"3 ForList=3 DBFind=3 SetVar=3"}]}]}]}]`},
	{`Data(Source: mysrc, Columns: "startdate,enddate", Data:
		2017-12-10 10:11,2017-12-12 12:13
		2017-12-17 16:17,2017-12-15 14:15
	).Custom(custom_id){
		SetVar(Name: vStartDate, Value: DateTime(DateTime: #startdate#, Format: "YYYY-MM-DD HH:MI"))
		SetVar(Name: vEndDate, Value: DateTime(DateTime: #enddate#, Format: "YYYY-MM-DD HH:MI"))
		SetVar(Name: vCmpDate, Value: CmpTime(#vStartDate#,#vEndDate#)) 
		P(Body: #vStartDate# #vEndDate# #vCmpDate#)
	}.Custom(custom_name){
		P(Body: #vStartDate# #vEndDate# #vCmpDate#)
	}`,
		`[{"tag":"data","attr":{"columns":["startdate","enddate","custom_id","custom_name"],"data":[["2017-12-10 10:11","2017-12-12 12:13","[{"tag":"p","children":[{"tag":"text","text":"2017-12-10 10:11 2017-12-12 12:13 -1"}]}]","[{"tag":"p","children":[{"tag":"text","text":"2017-12-10 10:11 2017-12-12 12:13 -1"}]}]"],["2017-12-17 16:17","2017-12-15 14:15","[{"tag":"p","children":[{"tag":"text","text":"2017-12-17 16:17 2017-12-15 14:15 1"}]}]","[{"tag":"p","children":[{"tag":"text","text":"2017-12-17 16:17 2017-12-15 14:15 1"}]}]"]],"source":"mysrc","types":["text","text","tags","tags"]}}]`},
	{`Strong(SysParam(taxes_size))`,
		`[{"tag":"strong","children":[{"tag":"text","text":"3"}]}]`},
	{`SetVar(Name: vDateNow, Value: Now("YYYY-MM-DD HH:MI")) 
		SetVar(Name: simple, Value: TestFunc(my value)) 
		SetVar(Name: vStartDate, Value: DateTime(DateTime: #vDateNow#, Format: "YYYY-MM-DD HH:MI"))
		SetVar(Name: vCmpStartDate, Value: CmpTime(#vStartDate#,#vDateNow#))
		Span(#vCmpStartDate# #simple#)`,
		`[{"tag":"span","children":[{"tag":"text","text":"-1 TestFunc(my value)"}]}]`},
	{`Input(Type: text, Value: Now(MMYY))`,
		`[{"tag":"input","attr":{"type":"text","value":"Now(MMYY)"}}]`},
	{`Button(Body: LangRes(savex), Class: btn btn-primary, Contract: EditProfile, 
		Page:members_list,).Alert(Text: $want_save_changesx$, 
		ConfirmButton: $yesx$, CancelButton: $nox$, Icon: question)`,
		`[{"tag":"button","attr":{"alert":{"cancelbutton":"$nox$","confirmbutton":"$yesx$","icon":"question","text":"$want_save_changesx$"},"class":"btn btn-primary","contract":"EditProfile","page":"members_list"},"children":[{"tag":"text","text":"savex"}]}]`},
	{`Button(Body: button).Popup(Width: 100)`,
		`[{"tag":"button","attr":{"popup":{"width":"100"}},"children":[{"tag":"text","text":"button"}]}]`},
	{`Button(Body: button).Popup(Width: 100, Header: header)`,
		`[{"tag":"button","attr":{"popup":{"header":"header","width":"100"}},"children":[{"tag":"text","text":"button"}]}]`},
	{`Button(Body: button).Popup(Header: header)`,
		`[{"tag":"button","children":[{"tag":"text","text":"button"}]}]`},
	{`Simple Strong(bold text)`,
		`[{"tag":"text","text":"Simple "},{"tag":"strong","children":[{"tag":"text","text":"bold text"}]}]`},
	{`EcosysParam(gender, Source: mygender)`,
		`[{"tag":"data","attr":{"columns":["id","name"],"data":[["1",""]],"source":"mygender","types":["text","text"]}}]`},
	{`EcosysParam(new_table)`,
		`[{"tag":"text","text":"ContractConditions("MainCondition")"}]`},
	{`SetVar(varZero, 0) If(#varZero#>0) { the varZero should be hidden }
		SetVar(varNotZero, 1) If(#varNotZero#>0) { the varNotZero should be visible }
		If(#varUndefined#>0) { the varUndefined should be hidden }`,
		`[{"tag":"text","text":"the varNotZero should be visible"}]`},
	{`DateTime(1257894000)`,
		`[{"tag":"text","text":"` + time.Unix(1257894000, 0).Format("2006-01-02 15:04:05") + `"}]`},
	{`CmpTime(1257894000, 1257895000)CmpTime(1257895000, 1257894000)CmpTime(1257894000, 1257894000)`,
		`[{"tag":"text","text":"-110"}]`},
	{`P(Guest = #guest_key#)`, `[{"tag":"p","children":[{"tag":"text","text":"Guest = 4544233900443112470"}]}]`},
}

func TestMoney(t *testing.T) {
	var ret contentResult
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	size := 10000000
	money := make([]byte, size)
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < size; i++ {
		money[i] = '0' + byte(rand.Intn(10))
	}
	err := sendPost(`content`, &url.Values{`template`: {`Money(` + string(money) + `)`}}, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	if RawToString(ret.Tree) != `[{"tag":"text","text":"invalid money value"}]` {
		t.Errorf(`wrong value %s`, RawToString(ret.Tree))
	}
}

func TestMobile(t *testing.T) {
	var ret contentResult
	gMobile = true
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	err := sendPost(`content`, &url.Values{`template`: {`If(#isMobile#){Span(Mobile)}.Else{Span(Desktop)}`}}, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	if RawToString(ret.Tree) != `[{"tag":"span","children":[{"tag":"text","text":"Mobile"}]}]` {
		t.Error(fmt.Errorf(`wrong mobile tree %s`, RawToString(ret.Tree)))
		return
	}
}

func TestCutoff(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	name := randName(`tbl`)
	form := url.Values{
		"Name": {name},
		"Columns": {`[
			{"name":"name","type":"varchar", "index": "1", "conditions":"true"},
			{"name":"long_text", "type":"text", "index":"0", "conditions":"true"},
			{"name":"short_text", "type":"varchar", "index":"0", "conditions":"true"}
			]`},
		"Permissions":   {`{"insert": "true", "update" : "true", "new_column": "true"}`},
		"ApplicationId": {"1"},
	}
	assert.NoError(t, postTx(`NewTable`, &form))
	form = url.Values{
		"Name": {name},
		"Value": {`
			contract ` + name + ` {
				data {
					LongText string
					ShortText string
				}
				action {
					DBInsert("` + name + `", {name: "test", long_text: $LongText, short_text: $ShortText})
				}
			}
		`},
		"Conditions":    {`true`},
		"ApplicationId": {"1"},
	}
	assert.NoError(t, postTx(`NewContract`, &form))

	shortText := crypto.RandSeq(30)
	longText := crypto.RandSeq(100)

	assert.NoError(t, postTx(name, &url.Values{
		"ShortText": {shortText},
		"LongText":  {longText},
	}))

	var ret contentResult
	template := `DBFind(Name: ` + name + `, Source: mysrc).Cutoff("short_text,long_text")`
	start := time.Now()
	assert.NoError(t, sendPost(`content`, &url.Values{`template`: {template}}, &ret))
	duration := time.Since(start)
	if int(duration.Seconds()) > 0 {
		t.Errorf(`Too much time for template parsing`)
		return
	}
	assert.NoError(t, postTx(name, &url.Values{
		"ShortText": {shortText},
		"LongText":  {longText},
	}))

	template = `DBFind("` + name + `", mysrc).Columns("id,name,short_text,long_text").Cutoff("short_text,long_text").WhereId(2).Vars(prefix)`
	assert.NoError(t, sendPost(`content`, &url.Values{`template`: {template}}, &ret))

	linkLongText := fmt.Sprintf("/data/1_%s/2/long_text/%x", name, md5.Sum([]byte(longText)))

	want := `[{"tag":"dbfind","attr":{"columns":["id","name","short_text","long_text"],"cutoff":"short_text,long_text","data":[["2","test","{"link":"","title":"` + shortText + `"}","{"link":"` + linkLongText + `","title":"` + longText[:32] + `"}"]],"name":"` + name + `","source":"mysrc","types":["text","text","long_text","long_text"],"whereid":"2"}}]`
	if RawToString(ret.Tree) != want {
		t.Errorf("Wrong image tree %s != %s", RawToString(ret.Tree), want)
	}

	resp, err := http.Get(apiAddress + consts.ApiPath + linkLongText)
	if err != nil {
		t.Error(err)
		return
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	assert.Equal(t, "attachment", resp.Header.Get("Content-Disposition"))
	assert.Equal(t, longText, string(data))
}

var imageData = `iVBORw0KGgoAAAANSUhEUgAAADIAAAAyCAIAAACRXR/mAAAACXBIWXMAAAsTAAALEwEAmpwYAAAARklEQVRYw+3OMQ0AIBAEwQOzaCLBBQZfAd0XFLMCNjOyb1o7q2Ey82VYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYrwqjmwKzLUjCbwAAAABJRU5ErkJggg==`

func TestBinary(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	data, err := base64.StdEncoding.DecodeString(imageData)
	assert.NoError(t, err)

	file := types.NewFile()
	file.Set("Body", data)

	params := contractParams{
		"ApplicationId": "1",
		"Name":          "file",
		"Data":          file,
	}

	_, id, err := postTxResult("UploadFile", &params)
	assert.NoError(t, err)

	hash := crypto.Hash(data)
	assert.NoError(t, err)
	hashImage := fmt.Sprintf("%x", hash)
	hashFindedImage := fmt.Sprintf("%x", md5.Sum(data))

	cases := []struct {
		source string
		result string
	}{
		{
			`Image(Src: Binary(Name: file, AppID: 1, Account: #account_id#))`,
			`\[{"tag":"image","attr":{"src":"/data/1_binaries/\d+/data/` + hashImage + `"}}\]`,
		},
		{
			`Image(Src: Binary().ById(` + id + `)`,
			`\[{"tag":"image","attr":{"src":"/data/1_binaries/\d+/data/` + hashImage + `"}}\]`,
		},
		{
			`SetVar(eco, 1)Image(Src: Binary().ById(` + id + `).Ecosystem(#eco#)`,
			`\[{"tag":"image","attr":{"src":"/data/1_binaries/\d+/data/` + hashImage + `"}}\]`,
		},
		{
			`SetVar(name, file)SetVar(app_id, 1)SetVar(member_id, #key_id#)Image(Src: Binary(Name: #name#, AppID: #app_id#, MemberID: #member_id#))`,
			`\[{"tag":"image","attr":{"src":"/data/1_binaries/\d+/data/` + hashImage + `"}}\]`,
		},
		{
			`SetVar(id, "` + id + `")Image(Src: Binary().ById(#id#)`,
			`\[{"tag":"image","attr":{"src":"/data/1_binaries/\d+/data/` + hashImage + `"}}\]`,
		},
		{
			`DBFind(Name: binaries, Src: mysrc).Where({app_id: 1, account: #account_id#, name: "file"}).Custom(img){Image(Src: #data#)}Table(mysrc, "Image=img")`,
			`\[{"tag":"dbfind","attr":{"columns":\["id","app_id","account","name","data","hash","mime_type","img"\],"data":\[\["\d+","1","\d+","file","{\\"link\\":\\"/data/1_binaries/\d+/data/` + hashFindedImage + `\\",\\"title\\":\\"` + hashFindedImage + `\\"}","` + hashFindedImage + `","application/octet-stream","\[{\\"tag\\":\\"image\\",\\"attr\\":{\\"src\\":\\"/data/1_binaries/\d+/data/` + hashFindedImage + `\\"}}\]"\]\],"name":"binaries","source":"Src: mysrc","types":\["text","text","text","text","blob","text","text","tags"\],"where":"app_id=1 AND member_id = \d+ AND name = 'file'"}},{"tag":"table","attr":{"columns":\[{"Name":"img","Title":"Image"}\],"source":"mysrc"}}\]`,
		},
		{
			`DBFind(Name: binaries, Src: mysrc).Where({app_id: 1, account: #account_id#, name: "file"}).Vars(prefix)Image(Src: "#prefix_data#")`,
			`\[{"tag":"dbfind","attr":{"columns":\["id","app_id","account","name","data","hash","mime_type"\],"data":\[\["\d+","1","\d+","file","{\\"link\\":\\"/data/1_binaries/\d+/data/` + hashFindedImage + `\\",\\"title\\":\\"` + hashFindedImage + `\\"}","` + hashFindedImage + `","application/octet-stream"\]\],"name":"binaries","source":"Src: mysrc","types":\["text","text","text","text","blob","text","text"\],"where":"app_id=1 AND member_id = \d+ AND name = 'file'"}},{"tag":"image","attr":{"src":"{\\"link\\":\\"/data/1_binaries/\d+/data/` + hashFindedImage + `\\",\\"title\\":\\"` + hashFindedImage + `\\"}"}}\]`,
		},
	}

	for _, v := range cases {
		var ret contentResult
		err := sendPost(`content`, &url.Values{`template`: {v.source}}, &ret)
		assert.NoError(t, err)
		fmt.Println(v.result)
		assert.Regexp(t, v.result, string(ret.Tree))
	}
}

func TestStringToBinary(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	contract := randName("binary")
	content := randName("content")
	filename := randName("file")
	mimeType := "text/plain"

	form := url.Values{
		"Value": {`
			contract ` + contract + ` {
				data {
					Content string
				}
				conditions {}
				action {
					UploadBinary("Name,ApplicationId,Data,DataMimeType", "` + filename + `", 1, StringToBytes($Content), "text/plain")
					$result = $account_id
				}
			}
		`}, "ApplicationId": {`1`}, "Conditions": {"true"},
	}
	assert.NoError(t, postTx("NewContract", &form))

	form = url.Values{"Content": {content}}
	_, account, err := postTxResult(contract, &form)
	assert.NoError(t, err)

	form = url.Values{
		"template": {`SetVar(link, Binary(Name: ` + filename + `, AppID: 1, Account: "` + account + `"))#link#`},
	}

	var ret struct {
		Tree []struct {
			Link string `json:"text"`
		} `json:"tree"`
	}
	assert.NoError(t, sendPost(`content`, &form, &ret))

	resp, err := http.Get(apiAddress + consts.ApiPath + ret.Tree[0].Link)
	if err != nil {
		t.Error(err)
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)

	assert.NoError(t, err)
	assert.Equal(t, content, string(data))
	assert.Equal(t, mimeType, resp.Header.Get("Content-Type"))
	assert.Equal(t, `attachment; filename="`+filename+`"`, resp.Header.Get("Content-Disposition"))
}
