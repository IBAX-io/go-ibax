/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	crypto.InitAsymAlgo("ECC_Secp256k1")
	crypto.InitHashAlgo("KECCAK256")
}
func TestBin(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	form := url.Values{`nowait`: {`nowait`}}
	assert.NoError(t, postTxMultipart(`@1NewContract`, &form))
	//_, _, err := postTxResult("rndPtlziReAis01638415503", &url.Values{})
	//assert.NoError(t, err)
}

func TestTransferSelf(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	form := url.Values{`nowait`: {`nowait`}}
	assert.NoError(t, postTransferSelfTxMultipart(&form))

}

func TestUTXO(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	form := url.Values{`nowait`: {`nowait`}}
	assert.NoError(t, postUTXOTxMultipart(&form))

}

func TestMath(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	users := []string{
		`04d750da3e19c5f7721a1dafd8663f2739dba23d81e01f0667730d217472a3bc9f93c6fbaade9c6b6387ece296c478d25559cb87ca3e58aa7ce627dd47ec902aea`,
		`0471f24f9dee904277a081df37377c4dbd7cc14671fec2f0c1a580a5d972c2f98d528ea7f460de8900ce70d6de08a118060f1a261b272722a460d87de93014f960`,
		`04df303174dfbc79b1e3baa196103c284b6ca97c21848511af18a401fdd1b0fb29bfd8724f43b44062ff5f67a969409ab037ae674a1010602b1a7d25245b49738c`,
		`044d92cd4f7f6bd3cb0c9600782d32797ebbc8a277a308b128f33453ed84f470e2bb88ceb5de87cd8288b8ade9e437c90de3610ddcfcfec41d6e70d302690bc3ec`,
		`04cbe75c40a2ef1256483c5bb910745171258b341f981598a01876fc3711c243f8fab181d72ea7abb838ad3f27e8a03b54032e8ab34656d9503539a38b05151cfa`,
		`04f220d6620b8a0ede8ac23e2724e467888138d654721fefb1a910bb717aaff279fed5560462ed7af1ac1208852822e73336033ce6289694a0d7dc974754cf61ae`,
	}
	for _, u := range users {
		form := url.Values{"NewPubkey": {u}}
		assert.NoError(t, postTx(`@1NewUser`, &form))
	}
	//form := url.Values{`Value`: {`[{"api_address":"http://127.0.0.1:7079","public_key":"0498b18e551493a269b6f419d7784d26c8e3555638e80897c69997ef9f211e21d5d0b8adeeaab0e0e750e720ddf3048ec55d613ba5dee3fdfd4e7c17d346731e9b","tcp_address":"127.0.0.1:7078"},{"tcp_address":"127.0.0.1:2078","api_address":"http://127.0.0.1:2079","public_key":"04d750da3e19c5f7721a1dafd8663f2739dba23d81e01f0667730d217472a3bc9f93c6fbaade9c6b6387ece296c478d25559cb87ca3e58aa7ce627dd47ec902aea"},{"tcp_address":"127.0.0.1:3078","api_address":"http://127.0.0.1:3079","public_key":"04df303174dfbc79b1e3baa196103c284b6ca97c21848511af18a401fdd1b0fb29bfd8724f43b44062ff5f67a969409ab037ae674a1010602b1a7d25245b49738c"},{"tcp_address":"127.0.0.1:4078","api_address":"http://127.0.0.1:4079","public_key":"04cbe75c40a2ef1256483c5bb910745171258b341f981598a01876fc3711c243f8fab181d72ea7abb838ad3f27e8a03b54032e8ab34656d9503539a38b05151cfa"}]`}, "Name": {"honor_nodes"}, `Conditions`: {`true`}}
	form := url.Values{`Value`: {`[{"api_address":"http://127.0.0.1:7079","public_key":"0498b18e551493a269b6f419d7784d26c8e3555638e80897c69997ef9f211e21d5d0b8adeeaab0e0e750e720ddf3048ec55d613ba5dee3fdfd4e7c17d346731e9b","tcp_address":"127.0.0.1:7078"},{"tcp_address":"127.0.0.1:2078","api_address":"http://127.0.0.1:2079","public_key":"04d750da3e19c5f7721a1dafd8663f2739dba23d81e01f0667730d217472a3bc9f93c6fbaade9c6b6387ece296c478d25559cb87ca3e58aa7ce627dd47ec902aea"}]`}, "Name": {"honor_nodes"}, `Conditions`: {`true`}}
	//form := url.Values{`Value`: {`[{"api_address":"http://127.0.0.1:7079","public_key":"0498b18e551493a269b6f419d7784d26c8e3555638e80897c69997ef9f211e21d5d0b8adeeaab0e0e750e720ddf3048ec55d613ba5dee3fdfd4e7c17d346731e9b","tcp_address":"127.0.0.1:7078"},{"tcp_address":"127.0.0.1:2078","api_address":"http://127.0.0.1:2079","public_key":"04d750da3e19c5f7721a1dafd8663f2739dba23d81e01f0667730d217472a3bc9f93c6fbaade9c6b6387ece296c478d25559cb87ca3e58aa7ce627dd47ec902aea"},{"tcp_address":"127.0.0.1:3078","api_address":"http://127.0.0.1:3079","public_key":"04df303174dfbc79b1e3baa196103c284b6ca97c21848511af18a401fdd1b0fb29bfd8724f43b44062ff5f67a969409ab037ae674a1010602b1a7d25245b49738c"}]`}, "Name": {"honor_nodes"}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`@1UpdatePlatformParam`, &form))
}

func TestArray(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	rnd := `@1rnd0Q3a3mIzpg01627469951`
	form := url.Values{`Info`: {crypto.RandSeq(14)}}
	assert.NoError(t, postTx(rnd, &form))
}

func TestCheckCondition(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	rnd := `cnt` + crypto.RandSeq(4)
	form := url.Values{`Value`: {`contract ` + rnd + ` {
    action {
			$result = Sprintf("%v %v %v %v %v", CheckCondition("1"), CheckCondition("0"), 
					CheckCondition("ContractConditions(\"MainCondition\")"), CheckCondition("true"), 
					CheckCondition("false"))
    }
}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	_, msg, err := postTxResult(rnd, &url.Values{})
	assert.NoError(t, err)
	if msg != `true false true true false` {
		t.Error(fmt.Errorf(`wrong msg %s`, msg))
	}
}

func TestDBFindContract(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	rnd := `db` + crypto.RandSeq(4)
	form := url.Values{`Value`: {`contract ` + rnd + `1 {
		data {
		}
		action { 
			var fm array
			fm = DBFind("@1contracts").Where({"ecosystem": $ecosystem_id, 
			   "app_id": 1, ,"id": {"$gt": 2}})
			$result = Len(fm)
		}}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	assert.EqualError(t, postTx(`NewContract`, &form),
		`{"type":"panic","error":"unexpected lexeme; expecting string key [CreateContract @1NewContract:32]"}`)
	form = url.Values{`Value`: {`contract ` + rnd + `2 {
				data {
				}
				action {
					var fm array
					fm = DBFind("@1contracts").Where({"ecosystem": $ecosystem_id,
					   "app_id": 1,"id": {"$gt": 2},})
				}}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	assert.EqualError(t, postTx(`NewContract`, &form),
		`{"type":"panic","error":"unexpected lexeme; expecting string key [CreateContract @1NewContract:32]"}`)
	form = url.Values{`Value`: {`contract ` + rnd + `3 {
			data {
			}
			action { 
				var fm array
				fm = [1, 2, 3,]
			}}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	assert.EqualError(t, postTx(`NewContract`, &form),
		`{"type":"panic","error":"unexpected lexeme; expecting string, int value or variable [CreateContract @1NewContract:32]"}`)

	form = url.Values{`Value`: {`contract ` + rnd + ` {
	   		    data {
	   			}
	   			action {
	   				var ret i j k m array
	   				var inr inc map
	   				var rids array
	                   rids = JSONDecode("[]")//role["roles_access"])
	   				inr = DBFind("@1roles_participants").Where({"ecosystem": $ecosystem_id, "role->id": {"$in": rids}, "member->member_id": $key_id, "deleted": 0}).Row()
	   				inc = DBFind("contracts").Where({"ecosystem": $ecosystem_id, "id": {"$in": rids}}).Row()
	   				ret = DBFind("contracts").Where({value: {"$ibegin": "CONTRACT"}}).Limit(100)
	   				i = DBFind("contracts").Where({value: {$ilike: "rEmove"}}).Limit(100)
	   				j = DBFind("contracts").Where({id: {$lt: 10}})
	   				k = DBFind("contracts").Where({id: {$lt: 11}, $or: [{id: 5}, {id: 7}], $and: [{id: {$neq: 25}}, id: {$neq: 26} ]})
	   				m = DBFind("contracts").Where({id: 10, name: "EditColumn", $or: [id: 10, id: {$neq: 20}]})
	   				$result = Sprintf("%d %d %d %d %d", Len(ret), Len(i), Len(j), Len(k), Len(m))
	   			}}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	_, msg, err := postTxResult(rnd, &url.Values{})
	assert.NoError(t, err)
	if msg != `25 25 9 2 1` {
		t.Error(fmt.Errorf(`wrong msg %s`, msg))
	}
}

func TestErrorContract(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	rnd := `err` + crypto.RandSeq(4)
	form := url.Values{`Value`: {`contract ` + rnd + ` {
		    data {
			}
			action { 
				error("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Fusce tincidunt 
				vestibulum eros. Curabitur fermentum pulvinar nibh, in maximus dolor tempor quis. 
				Donec non nulla id ex lacinia bibendum eu a sapien. Nam eu mi feugiat, gravida 
				erat ac, tincidunt dolor. Curabitur sed erat et felis turpis duis.")
			}}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	_, _, err := postTxResult(rnd, &url.Values{})
	if len(err.Error()) > 250 {
		t.Error(`Too long error`)
	}
	rnd += `1`
	form = url.Values{`Value`: {`contract ` + rnd + ` {
		data {
		}
		action { 
			Throw("This is a problem", "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Fusce tincidunt 
			vestibulum eros. Curabitur fermentum pulvinar nibh, in maximus dolor tempor quis. 
			Donec non nulla id ex lacinia bibendum eu a sapien. Nam eu mi feugiat, gravida 
			erat ac, tincidunt dolor. Curabitur sed erat et felis turpis duis.")
		}}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	_, _, err = postTxResult(rnd, &url.Values{})
	if len(err.Error()) > 250 {
		t.Error(`Too long error`)
	}
}

func TestUpdate_HonorNodes(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}

	err := postTx("UpdatePlatformParam", &url.Values{
		"Name":  {"honor_nodes"},
		"Value": {"[]"},
	})
	if err != nil {
		t.Error(err)
		return
	}
}

func TestCrashContract(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	rnd := `crash` + crypto.RandSeq(4)
	form := url.Values{`Value`: {`contract ` + rnd + ` {
			data {}
		
			conditions {
				$Recipient = Append([], "1")
				$Recipient = Append($Recipient, "7")
			}
		
			action {
				var i int
				var steps map
				var list myarr q b array
				while i < Len($Recipient) {
					steps["recipient_role"] = JSONDecode($Recipient[i])
					list[i] = Append(list, steps)
					myarr = Split(list[i], ",")
					i = i + 1
				}
			}
		}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	assert.EqualError(t, postTx(rnd, &url.Values{}), `{"type":"panic","error":"self assignment"}`)
}

func TestHardContract(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	rnd := `hard` + crypto.RandSeq(4)
	form := url.Values{`Value`: {`contract ` + rnd + ` {
		    data {
			}
			action { 
				var i int
				while i < 500 {
				 DBFind("pages").Where({id:5})
				 DBUpdate("pages", 5, {"value":"P(text)"})
				 DBInsert("pages", {"name": Sprintf("` + rnd + `%d", i),
				      "value":"P(text)", "conditions": "true"})
				 DBFind("pages").Where({id:6})
				 DBFind("pages").Where({id:7})
				 DBUpdate("pages", 6, {"value": "P(text)"})
				 i = i + 1
			   }
			}}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	assert.EqualError(t, postTx(rnd, &url.Values{}), `{"type":"txError","error":"Time limit exceeded"}`)
}

func TestExistContract(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	rnd := `cnt` + crypto.RandSeq(4)
	form := url.Values{"Name": {rnd}, "Value": {`contract ` + rnd + ` {
		data {
			Name string
		}
		action {
		Throw($Name, "Text of the error")
	}}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	assert.EqualError(t, postTx(rnd, &url.Values{"Name": {"1"}}),
		`{"type":"exception","error":"Text of the error","id":"1"}`)
	form = url.Values{"Name": {`EditPage`}, "Value": {`contract EditPage {action {}}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}

	assert.EqualError(t, postTx(`NewContract`, &form), `{"type":"panic","error":"Contract EditPage already exists"}`)
}

func TestDataContract(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	name := `cnt` + crypto.RandSeq(4)
	form := url.Values{"Name": {name}, "Value": {`contract ` + name + `1 {
		data {Name int
			string qwerty}
		action {}
		}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.EqualError(t, postTx(`NewContract`, &form), `{"type":"panic","error":"expecting name of the data field [Ln:3 Col:5]"}`)

	form = url.Values{"Name": {name}, "Value": {`contract ` + name + ` {
		data {MyApp qwerty}
		action {}
		}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.EqualError(t, postTx(`NewContract`, &form), `{"type":"panic","error":"expecting type of the data field [Ln:2 Col:16]"}`)

	form = url.Values{"Name": {name}, "Value": {`contract ` + name + ` {
		data {MyApp int
		    Qwert}
		action {}
		}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.EqualError(t, postTx(`NewContract`, &form), `{"type":"panic","error":"expecting type of the data field [Ln:3 Col:13]"}`)
}

func TestTypesContract(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	name := `cnt` + crypto.RandSeq(4)
	form := url.Values{"Name": {name}, "Value": {`contract ` + name + ` {
		data {
			Float float
			Addr  address
			Arr   array
			Map   map
		}
		action { $result = Sprintf("%v=%v=%v=%v", $Float, $Addr, $Arr, $Map) }
		}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	_, msg, err := postTxResult(name, &url.Values{"Float": {"1.23"}, "Addr": {"-1334343423"},
		"Arr": {`[23,"tt"]`}, "Map": {`{"k" : "v"}`}})
	assert.NoError(t, err)
	if msg != `1.23=-1334343423=[23 tt]=map[k:v]` {
		t.Error(`Wrong msg`, msg)
	}
}

func TestNewContracts(t *testing.T) {

	//wanted := func(name, want string) bool {
	//	var ret getTestResult
	//	return assert.NoError(t, sendPost(`test/`+name, nil, &ret)) && assert.Equal(t, want, ret.Value)
	//}

	assert.NoError(t, keyLogin(1))
	rnd := crypto.RandSeq(4)
	for i, item := range contracts {
		var ret getContractResult
		if i > 100 {
			break
		}
		name := strings.Replace(item.Name, `#rnd#`, rnd, -1)
		err := sendGet(`contract/`+name, nil, &ret)
		if err != nil {
			if strings.Contains(err.Error(), errContract.Errorf(name).Error()) {
				form := url.Values{"Name": {name}, "Value": {strings.Replace(item.Value,
					`#rnd#`, rnd, -1)},
					"ApplicationId": {`1`}, "Conditions": {`true`}}
				if err := postTx(`NewContract`, &form); err != nil {
					assert.EqualError(t, err, item.Params[0].Results[`error`])
					continue
				}
			} else {
				t.Error(err)
				return
			}
		}
		if strings.HasSuffix(name, `testUpd`) {
			continue
		}
		for _, par := range item.Params {
			form := url.Values{}
			for key, value := range par.Params {
				form[key] = []string{value}
			}
			if err := postTx(name, &form); err != nil {
				assert.EqualError(t, err, par.Results[`error`])
				continue
			}
			//for key, value := range par.Results {
			//	if !wanted(key, value) {
			//		return
			//	}
			//}
		}
	}
	var row rowResult
	assert.NoError(t, sendGet(`row/menu/1`, nil, &row))
	assert.NotEqual(t, `update`, row.Value[`value`])
}

var contracts = []smartContract{
	{`Empty`, `contract Empty {
		action {
			var a1 array
			var a2 map
			$a1 = []
			$a2 = {}
			Test("result", "ok")
		}
	}`, []smartParams{
		{nil, map[string]string{`result`: `ok`}},
	}},

	{`FmtMoney`, `contract FmtMoney {
		action {
			Test("result", FormatMoney("123456789", 0))
			$num2 = "5500000"
			$num1 = "12345672372"
			Test("t1", FormatMoney($num1, -1))  //123456723720
			Test("t2", FormatMoney($num1, 0))   //12345672372
			Test("t3", FormatMoney($num1, 1))   //1234567237,2
		}
	}`, []smartParams{
		{nil, map[string]string{`result`: `123456789`,
			`t1`: `123456723720`, `t2`: `12345672372`, `t3`: `1234567237.2`}},
	}},

	{`StrNil`, `contract StrNil {
		action {
			Test("result", Sprintf("empty: %s", Str(nil)))
		}
	}`, []smartParams{
		{nil, map[string]string{`result`: `empty: `}},
	}},
	{`TestJSON`, `contract TestJSON {
		data {}
		conditions { }
		action {
		   var a map
		   a["ok"] = 10
		   a["arr"] = ["first", "<second>"]
		   Test("json", JSONEncode(a))
		   Test("ok", JSONEncodeIndent(a, "\t"))
		}
	}`, []smartParams{
		{nil, map[string]string{`ok`: "{\n\t\"ok\": 10,\n\t\"arr\": [\n\t\t\"first\",\n\t\t\"<second>\"\n\t]\n}",
			`json`: "{\"ok\":10,\"arr\":[\"first\",\"<second>\"]}"}},
	}},
	{`GuestKey`, `contract GuestKey {
		action {
			Test("result", $guest_key)
		}
	}`, []smartParams{
		{nil, map[string]string{`result`: `4544233900443112470`}},
	}},
	{`TestCyr`, `contract TestCyr {
		data {}
		conditions { }
		action {
		   //test
		   var a map
		   a["test"] = "test"
		   Test("ok", a["test"])
		}
	}`, []smartParams{
		{nil, map[string]string{`ok`: `test`}},
	}},
	{`DBFindLike`, `contract DBFindLike {
		action {
			var list array
			list = DBFind("pages").Where({"name":{"$like": "ort_"}})
			Test("size", Len(list))
			list = DBFind("pages").Where({"name":{"$end": "page"}})
			Test("end", Len(list))
		}
	}`, []smartParams{
		{nil, map[string]string{`size`: `4`, `end`: `2`}},
	}},
	{`TestDBFindOK`, `
			contract TestDBFindOK {
			action {
				var ret array
				var vals map
				ret = DBFind("contracts").Columns("id,value").Where({"$and":[{"id":{"$gte": 3}}, {"id":{"$lte":5}}]}).Order("id")
				if Len(ret) {
					Test("0",  "1")
				} else {
					Test("0",  "0")
				}
				ret = DBFind("contracts").Limit(3)
				if Len(ret) == 3 {
					Test("1",  "1")
				} else {
					Test("1",  "0")
				}
				ret = DBFind("contracts").Order("id").Offset(1).Limit(1)
				if Len(ret) != 1 {
					Test("2",  "0")
				} else {
					vals = ret[0]
					Test("2",  vals["id"])
				}
				ret = DBFind("contracts").Columns("id").Order(["id"]).Offset(1).Limit(1)
				if Len(ret) != 1 {
					Test("3",  "0")
				} else {
					vals = ret[0]
					Test("3", vals["id"])
				}
				ret = DBFind("contracts").Columns("id").Where({"$or":[{"id": "1"}]})
				if Len(ret) != 1 {
					Test("4",  "0")
				} else {
					vals = ret[0]
					Test("4", vals["id"])
				}
				ret = DBFind("contracts").Columns("id").Where({"id": 1})
				if Len(ret) != 1 {
					Test("4",  "0")
				} else {
					vals = ret[0]
					Test("4", vals["id"])
				}
				ret = DBFind("contracts").Columns("id,value").Where({"id":[{"$gt":3},{"$lt":8}]}).Order([{"id": 1}, {"name": "-1"}])
				if Len(ret) != 4 {
					Test("5",  "0")
				} else {
					vals = ret[0]
					Test("5", vals["id"])
				}
				ret = DBFind("contracts").WhereId(7)
				if Len(ret) != 1 {
					Test("6",  "0")
				} else {
					vals = ret[0]
					Test("6", vals["id"])
				}
				var one string
				one = DBFind("contracts").WhereId(5).One("id")
				Test("7",  one)
				var row map
				row = DBFind("contracts").WhereId(3).Row()
				Test("8",  row["id"])
				Test("255",  "255")
			}
		}`,
		[]smartParams{
			{nil, map[string]string{`0`: `1`, `1`: `1`, `2`: `2`, `3`: `2`, `4`: `1`, `5`: `4`,
				`6`: `7`, `7`: `5`, `8`: `3`, `255`: `255`}},
		}},
	{`DBFindCol`, `contract DBFindCol {
		action {
			var ret string
			ret = DBFind("keys").Columns(["amount", "id"]).One("amount")
			Test("size", Size(ret)>0)
		}
	}`, []smartParams{
		{nil, map[string]string{`size`: `true`}},
	}},
	{`DBFindColumnNow`, `contract DBFindColumnNow {
		action {
			var list array
			list = DBFind("keys").Columns("now()")
		}
	}`, []smartParams{
		{nil, map[string]string{`error`: `{"type":"panic","error":"pq: current transaction is aborted, commands ignored until end of transaction block"}`}},
	}},
	{`DBFindCURRENT`, `contract DBFindCURRENT {
		action {
			var list array
			list = DBFind("mytable").Where({"date": {"$lt": "CURRENT_DATE"}})
		}
	}`, []smartParams{
		{nil, map[string]string{`error`: `{"type":"panic","error":"It is prohibited to use NOW() or current time functions"}`}},
	}},
	{`RowType`, `contract RowType {
	action {
		var app map
		var result string
		result = GetType(app)
		app = DBFind("applications").Where({"id":"1"}).Row()
		result = result + GetType(app)
		app["app_id"] = 2
		Test("result", Sprintf("%s %s %d", result, app["name"], app["app_id"]))
	}
}`, []smartParams{
		{nil, map[string]string{`result`: `*types.Map*types.Map System 2`}},
	}},
	{`StackType`, `contract StackType {
		action {
			var lenStack int
			lenStack = Len($stack)
			var par string
			par = $stack[0]
			Test("result", Sprintf("len=%d %v %s", lenStack, $stack, par))
		}
	}`, []smartParams{
		{nil, map[string]string{`result`: `len=1 [@1StackType] @1StackType`}},
	}},
	{`DBFindNow`, `contract DBFindNow {
		action {
			var list array
			list = DBFind("mytable").Where({"date": {"$lt": "now ( )"}})
		}
	}`, []smartParams{
		{nil, map[string]string{`error`: `{"type":"panic","error":"It is prohibited to use NOW() or current time functions"}`}},
	}},
	{`BlockTimeCheck`, `contract BlockTimeCheck {
		action {
			if Size(BlockTime()) == Size("2006-01-02 15:04:05") {
				Test("ok", "1")
			} else {
				Test("ok", "0")
			}
		}
	}`, []smartParams{
		{nil, map[string]string{`ok`: `1`}},
	}},
	{`RecCall`, `contract RecCall {
		data {    }
		conditions {    }
		action {
			var par map
			CallContract("RecCall", par)
		}
	}`, []smartParams{
		{nil, map[string]string{`error`: `{"type":"panic","error":"There is loop in @1RecCall contract"}`}},
	}},
	{`Recursion`, `contract Recursion {
		data {    }
		conditions {    }
		action {
			Recursion()
		}
	}`, []smartParams{
		{nil, map[string]string{`error`: `{"type":"panic","error":"The contract can't call itself recursively"}`}},
	}},
	{`MyTable#rnd#`, `contract MyTable#rnd# {
		action {
			NewTable("Name,Columns,ApplicationId,Permissions", "#rnd#1", 
				"[{\"name\":\"MyName\",\"type\":\"varchar\", \"index\": \"0\", \"conditions\":{\"update\":\"true\", \"read\":\"true\"}}]", 100,
				 "{\"insert\": \"true\", \"update\" : \"true\", \"new_column\": \"true\"}")
			var cols array
			cols[0] = "{\"conditions\":\"true\",\"name\":\"column1\",\"type\":\"text\"}"
			cols[1] = "{\"conditions\":\"true\",\"name\":\"column2\",\"type\":\"text\"}"
			NewTable("Name,Columns,ApplicationId,Permissions", "#rnd#2", 
				JSONEncode(cols), 100,
				 "{\"insert\": \"true\", \"update\" : \"true\", \"new_column\": \"true\"}")
			
			Test("ok", "1")
		}
	}`, []smartParams{
		{nil, map[string]string{`ok`: `1`}},
	}},
	{`IntOver`, `contract IntOver {
				action {
					info Int("123456789101112131415161718192021222324252627282930")
				}
			}`, []smartParams{
		{nil, map[string]string{`error`: `{"type":"panic","error":"123456789101112131415161718192021222324252627282930 is not a valid integer : value out of range"}`}},
	}},
	{`Double`, `contract Double {
		data {    }
		conditions {    }
		action {
			$$$$$$$$result = "hello"
		}
	}`, []smartParams{
		{nil, map[string]string{`error`: `{"type":"panic","error":"unknown lexeme $ [Ln:5 Col:6]"}`}},
	}},
	{`Price`, `contract Price {
		action {
			Test("int", Int("")+Int(nil)+2)
			Test("price", 1)
		}
		func price() money {
			return Money(100)
		}
	}`, []smartParams{
		{nil, map[string]string{`price`: `1`, `int`: `2`}},
	}},
	{`CheckFloat`, `contract CheckFloat {
			action {
			var fl float
			fl = -3.67
			Test("float2", Sprintf("%d %s", Int(1.2), Str(fl)))
			Test("float3", Sprintf("%.2f %.2f", 10.7/7, 10/7.0))
			Test("float4", Sprintf("%.2f %.2f %.2f", 10+7.0, 10-3.1, 5*2.5))
			Test("float5", Sprintf("%t %t %t %t %t", 10 <= 7.0, 4.5 <= 5, 3>5.7, 6 == 6.0, 7 != 7.1))
		}}`, []smartParams{
		{nil, map[string]string{`float2`: `1 -3.670000`, `float3`: `1.53 1.43`, `float4`: `17.00 6.90 12.50`, `float5`: `false true false true true`}},
	}},
	{`Crash`, `contract Crash { data {} conditions {} action

			{ $result=DBUpdate("menu", 1, {"value": "updated"}) }
			}`,
		[]smartParams{
			{nil, map[string]string{`error`: `{"type":"panic","error":"Access denied"}`}},
		}},
	{`TestOneInput`, `contract TestOneInput {
			data {
				list array
			}
			action {
				var coltype string
				coltype = GetColumnType("keys", "amount" )
				Test("oneinput",  $list[0]+coltype)
			}
		}`,
		[]smartParams{
			{map[string]string{`list`: `Input value`}, map[string]string{`oneinput`: `Input valuemoney`}},
		}},
	{`DBProblem`, `contract DBProblem {
		action{
			DBFind("members1").Where({"member_name": "name"})
		}
	}`,
		[]smartParams{
			{nil, map[string]string{`error`: `{"type":"panic","error":"pq: current transaction is aborted, commands ignored until end of transaction block"}`}},
		}},
	{`TestMultiForm`, `contract TestMultiForm {
				data {
					list array
				}
				action {
					Test("multiform",  $list[0]+$list[1])
				}
			}`,
		[]smartParams{
			{map[string]string{`list[]`: `2`, `list[0]`: `start`, `list[1]`: `finish`}, map[string]string{`multiform`: `startfinish`}},
		}},
	{`errTestMessage`, `contract errTestMessage {
			conditions {
			}
			action { qvar ivar int}
		}`,
		[]smartParams{
			{nil, map[string]string{`error`: `{"type":"panic","error":"unknown variable qvar"}`}},
		}},

	{`EditProfile9`, `contract EditProfile9 {
			data {
			}
			conditions {
			}
			action {
				var ar array
				ar = Split("point 1,point 2", ",")
				Test("split",  Str(ar[1]))
				$ret = DBFind("contracts").Columns("id,value").Where({"id":[{"$gte": 3}, {"$lte":5}]}).Order("id")
				Test("edit",  "edit value 0")
			}
		}`,
		[]smartParams{
			{nil, map[string]string{`edit`: `edit value 0`, `split`: `point 2`}},
		}},
	{`testEmpty`, `contract testEmpty {
					action { Test("empty",  "empty value")}}`,
		[]smartParams{
			{nil, map[string]string{`empty`: `empty value`}},
		}},
	{`testUpd`, `contract testUpd {
						action { Test("date",  "-2006.01.02-")}}`,
		[]smartParams{
			{nil, map[string]string{`date`: `-` + time.Now().Format(`2006.01.02`) + `-`}},
		}},
	{`testLong`, `contract testLong {
			action { Test("long",  "long result")
				$result = DBFind("contracts").WhereId(2).One("value") + DBFind("contracts").WhereId(4).One("value")
				Println("Result", $result)
				Test("long",  "long result")
			}}`,
		[]smartParams{
			{nil, map[string]string{`long`: `long result`}},
		}},
	{`testSimple`, `contract testSimple {
					data {
						Amount int
						Name   string
					}
					conditions {
						Test("scond", $Amount, $Name)
					}
					action { Test("sact", $Name, $Amount)}}`,
		[]smartParams{
			{map[string]string{`Name`: `Simple name`, `Amount`: `-56781`},
				map[string]string{`scond`: `-56781Simple name`,
					`sact`: `Simple name-56781`}},
		}},
	{`errTestVar`, `contract errTestVar {
				conditions {
				}
				action { var ivar int}
			}`,
		nil},
	{`testGetContract`, `contract testGetContract {
			action { Test("ByName", GetContractByName(""), GetContractByName("ActivateContract"))
				Test("ById", GetContractById(10000000), GetContractById(16))}}`,
		[]smartParams{
			{nil, map[string]string{`ByName`: `0 4`,
				`ById`: `EditTable`}},
		}},
	{
		`testDateTime`, `contract testDateTime {
				data {
					Date string
					Unix int
				}
				action {
					Test("DateTime", DateTime($Unix))
					Test("UnixDateTime", UnixDateTime($Date))
				}
			}`,
		[]smartParams{
			{map[string]string{
				"Unix": "1257894000",
				"Date": "2009-11-11 04:00:00",
			}, map[string]string{
				"DateTime":     "2009-11-11 04:00:00",
				"UnixDateTime": timeMustParse("2009-11-11 04:00:00"),
			}},
		},
	},
}

func timeMustParse(value string) string {
	t, _ := time.Parse("2006-01-02 15:04:05", value)
	return converter.Int64ToStr(t.Unix())
}

func TestEditContracts(t *testing.T) {

	//wanted := func(name, want string) bool {
	//	var ret getTestResult
	//	err := sendPost(`test/`+name, nil, &ret)
	//	if err != nil {
	//		t.Error(err)
	//		return false
	//	}
	//	if ret.Value != want {
	//		t.Error(fmt.Errorf(`%s != %s`, ret.Value, want))
	//		return false
	//	}
	//	return true
	//}

	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	var cntlist contractsResult
	err := sendGet(`contracts`, nil, &cntlist)
	if err != nil {
		t.Error(err)
		return
	}
	var ret getContractResult
	err = sendGet(`contract/testUpd`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	sid := ret.TableID
	var row rowResult
	err = sendGet(`row/contracts/`+sid, nil, &row)
	if err != nil {
		t.Error(err)
		return
	}
	code := row.Value[`value`]
	off := strings.IndexByte(code, '-')
	newCode := code[:off+1] + time.Now().Format(`2006.01.02`) + code[off+11:]
	form := url.Values{`Id`: {sid}, `Value`: {newCode}, `Conditions`: {row.Value[`conditions`]}, `WalletId`: {"01231234123412341230"}}
	if err := postTx(`EditContract`, &form); err != nil {
		t.Error(err)
		return
	}

	for _, item := range contracts {
		if !strings.HasSuffix(item.Name, `testUpd`) {
			continue
		}
		for _, par := range item.Params {
			form := url.Values{}
			for key, value := range par.Params {
				form[key] = []string{value}
			}
			if err := postTx(item.Name, &form); err != nil {
				t.Error(err)
				return
			}
			//for key, value := range par.Results {
			//	if !wanted(key, value) {
			//		return
			//	}
			//}
		}
	}
}

func TestNewTableWithEmptyName(t *testing.T) {
	require.NoError(t, keyLogin(1))
	sql1 := `new_column varchar(10); update block_chain set key_id='1234' where id='1' --`
	sql2 := `new_column varchar(10); update block_chain set key_id='12' where id='1' --`
	name := randName(`tbl`)
	form := url.Values{
		"Name":          {name},
		"Columns":       {"[{\"name\":\"" + sql1 + "\",\"type\":\"varchar\", \"index\": \"0\", \"conditions\":{\"update\":\"true\", \"read\":\"true\"}}]"},
		"ApplicationId": {"1"},
		"Permissions":   {"{\"insert\": \"true\", \"update\" : \"true\", \"new_column\": \"true\"}"},
	}

	require.NoError(t, postTx("NewTable", &form))

	form = url.Values{"TableName": {name}, "Name": {sql2},
		"Type": {"varchar"}, "Index": {"0"}, "Permissions": {"true"}}
	assert.NoError(t, postTx(`NewColumn`, &form))

	form = url.Values{
		"Name":          {""},
		"Columns":       {"[{\"name\":\"MyName\",\"type\":\"varchar\", \"index\": \"0\", \"conditions\":{\"update\":\"true\", \"read\":\"true\"}}]"},
		"ApplicationId": {"1"},
		"Permissions":   {"{\"insert\": \"true\", \"update\" : \"true\", \"new_column\": \"true\"}"},
	}

	if err := postTx("NewTable", &form); err == nil || err.Error() !=
		`400 {"error": "E_SERVER", "msg": "Name is empty" }` {
		t.Error(`wrong error`, err)
	}

	form = url.Values{
		"Name":          {"Digit" + name},
		"Columns":       {"[{\"name\":\"1\",\"type\":\"varchar\", \"index\": \"0\", \"conditions\":{\"update\":\"true\", \"read\":\"true\"}}]"},
		"ApplicationId": {"1"},
		"Permissions":   {"{\"insert\": \"true\", \"update\" : \"true\", \"new_column\": \"true\"}"},
	}

	assert.EqualError(t, postTx("NewTable", &form), `{"type":"panic","error":"Column name cannot begin with digit"}`)
}

func TestContracts(t *testing.T) {

	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}

	var ret contractsResult
	err := sendGet(`contracts`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
}

func TestSignature(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	rnd := `rnd` + crypto.RandSeq(6)
	form := url.Values{`Value`: {`contract ` + rnd + `Transfer {
		    data {
				Recipient int
				Amount    money
				Signature string "optional hidden"
			}
			action { 
				$result = "OK " + Str($Amount)
			}}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	if err := postTx(`NewContract`, &form); err != nil {
		t.Error(err)
		return
	}
	form = url.Values{`Value`: {`contract ` + rnd + `Test {
			data {
				Recipient int "hidden"
				Amount  money
				Signature string "signature:` + rnd + `Transfer"
			}
			func action {
				` + rnd + `Transfer("Recipient,Amount,Signature",$Recipient,$Amount,$Signature )
				$result = "OOOPS " + Str($Amount)
			}
		  }
		`}, `Conditions`: {`true`}, "ApplicationId": {"1"}}
	if err := postTx(`NewContract`, &form); err != nil {
		t.Error(err)
		return
	}

	form = url.Values{`Name`: {rnd + `Transfer`}, `Value`: {`{"title": "Would you like to sign",
		"params":[
			{"name": "Receipient", "text": "Wallet"},
			{"name": "Amount", "text": "Amount(money)"}
			]}`}, `Conditions`: {`true`}}
	if err := postTx(`NewSign`, &form); err != nil {
		t.Error(err)
		return
	}
	err := postTx(rnd+`Test`, &url.Values{`Amount`: {`12345`}, `Recipient`: {`98765`}})
	if err != nil {
		t.Error(err)
		return
	}
}

var (
	imp = `{
		"menus": [
			{
				"Name": "test_%s",
				"Conditions": "ContractAccess(\"@1EditMenu\")",
				"Value": "MenuItem(main, Default Ecosystem Menu)"
			}
		],
		"contracts": [
			{
				"Name": "testContract%[1]s",
				"Value": "contract testContract%[1]s {\n    data {}\n    conditions {}\n    action {\n        var res array\n        res = DBFind(\"pages\").Columns(\"name\").Where({id: 1}).Order(\"id\")\n        $result = res\n    }\n    }",
				"Conditions": "ContractConditions(` + "`MainCondition`" + `)"
			}
		],
		"pages": [
			{
				"Name": "test_%[1]s",
				"Conditions": "ContractAccess(\"@1EditPage\")",
				"Menu": "default_menu",
				"Value": "P(class, Default Ecosystem Page)\nImage().Style(width:100px;)"
			}
		],
		"blocks": [
			{
				"Name": "test_%[1]s",
				"Conditions": "true",
				"Value": "block content"
			},
			{
				"Name": "test_a%[1]s",
				"Conditions": "true",
				"Value": "block content"
			},
			{
				"Name": "test_b%[1]s",
				"Conditions": "true",
				"Value": "block content"
			}
		],
		"tables": [
			{
				"Name": "members%[1]s",
				"Columns": "[{\"name\":\"name\",\"type\":\"varchar\",\"conditions\":\"true\"},{\"name\":\"birthday\",\"type\":\"datetime\",\"conditions\":\"true\"},{\"name\":\"member_id\",\"type\":\"number\",\"conditions\":\"true\"},{\"name\":\"val\",\"type\":\"text\",\"conditions\":\"true\"},{\"name\":\"name_first\",\"type\":\"text\",\"conditions\":\"true\"},{\"name\":\"name_middle\",\"type\":\"text\",\"conditions\":\"true\"}]",
				"Permissions": "{\"insert\":\"true\",\"update\":\"true\",\"new_column\":\"true\"}"
			}
		],
		"parameters": [
			{
				"Name": "host%[1]s",
				"Value": "",
				"Conditions": "ContractConditions(` + "`MainCondition`" + `)"
			},
			{
				"Name": "host0%[1]s",
				"Value": "test",
				"Conditions": "ContractConditions(` + "`MainCondition`" + `)"
			}
		],
		"data": [
			{
				"Table": "members%[1]s",
				"Columns": ["name","val"],
				"Data": [
					["Bob","Richard mark"],
					["Mike Winter","Alan summer"]
				 ]
			}
		]
}`
)

func TestImport(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	name := crypto.RandSeq(4)
	form := url.Values{"Data": {fmt.Sprintf(imp, name)}}
	err := postTx(`@1Import`, &form)
	if err != nil {
		t.Error(err)
		return
	}

}

func TestEditContracts_ChangeWallet(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	rnd := `rnd` + crypto.RandSeq(6)
	code := `contract ` + rnd + ` {
		data {
			Par string "optional"
		}
		action { $result = $par}}`
	form := url.Values{`Value`: {code}, `Conditions`: {`true`}}
	if err := postTx(`NewContract`, &form); err != nil {
		t.Error(err)
		return
	}

	var ret getContractResult
	err := sendGet(`contract/`+rnd, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	keyID := ret.WalletID
	sid := ret.TableID
	var row rowResult
	err = sendGet(`row/contracts/`+sid, nil, &row)
	if err != nil {
		t.Error(err)
		return
	}

	if err := postTx(`ActivateContract`, &url.Values{`Id`: {sid}}); err != nil {
		t.Error(err)
		return
	}

	code = row.Value[`value`]
	form = url.Values{`Id`: {sid}, `Value`: {code}, `Conditions`: {row.Value[`conditions`]}, `WalletId`: {"1248-5499-7861-4204-5166"}}
	err = postTx(`EditContract`, &form)
	if err == nil {
		t.Error("Expected `Contract activated` error")
		return
	}
	err = sendGet(`contract/`+rnd, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	if ret.WalletID != keyID {
		t.Error(`wrong walletID`, ret.WalletID, keyID)
		return
	}
	if err := postTx(`DeactivateContract`, &url.Values{`Id`: {sid}}); err != nil {
		t.Error(err)
		return
	}

	if err := postTx(`EditContract`, &form); err != nil {
		t.Error(err)
		return
	}
	err = sendGet(`contract/`+rnd, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	if ret.Address != "1248-5499-7861-4204-5166" {
		t.Error(`wrong address`, ret.Address, "!= 1248-5499-7861-4204-5166")
		return
	}
}

func TestUpdateFunc(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	rnd := `rnd` + crypto.RandSeq(6)
	form := url.Values{`Value`: {`contract f` + rnd + ` {
		data {
			par string
		}
		func action {
			$result = Sprintf("X=%s %s %s", $par, $original_contract, $this_contract)
		}}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	_, id, err := postTxResult(`NewContract`, &form)
	assert.NoError(t, err)

	form = url.Values{`Value`: {`
		contract one` + rnd + ` {
			action {
				var ret map
				ret = DBFind("contracts").Columns("id,value").WhereId(10).Row()
				$result = ret["id"]
		}}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	form = url.Values{`Value`: {`contract row` + rnd + ` {
				action {
					var ret string
					ret = DBFind("contracts").Columns("id,value").WhereId(11).One("id")
					$result = ret
				}}
		
			`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	_, msg, err := postTxResult(`one`+rnd, &url.Values{})
	assert.NoError(t, err)
	assert.Equal(t, "10", msg)

	_, msg, err = postTxResult(`row`+rnd, &url.Values{})
	assert.NoError(t, err)
	assert.Equal(t, "11", msg)

	form = url.Values{`Value`: {`
		contract ` + rnd + ` {
		    data {
				Par string
			}
			action {
				$result = f` + rnd + `("par",$Par) + " " + $this_contract
			}}
		`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	_, idcnt, err := postTxResult(`NewContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	_, msg, err = postTxResult(rnd, &url.Values{`Par`: {`my param`}})
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf(`X=my param %s f%[1]s %[1]s`, rnd), msg)

	form = url.Values{`Id`: {id}, `Value`: {`
		func MyTest2(input string) string {
			return "Y="+input
		}`}, `Conditions`: {`true`}}
	err = postTx(`EditContract`, &form)
	assert.EqualError(t, postTx(`EditContract`, &form), `{"type":"panic","error":"Contracts or functions names cannot be changed"}`)

	form = url.Values{`Id`: {id}, `Value`: {`contract f` + rnd + `{
		data {
			par string
		}
		action {
			$result = "Y="+$par
		}}`}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`EditContract`, &form))

	_, msg, err = postTxResult(rnd, &url.Values{`Par`: {`new param`}})
	assert.NoError(t, err)
	assert.Equal(t, `Y=new param `+rnd, msg)

	form = url.Values{`Id`: {idcnt}, `Value`: {`
		contract ` + rnd + ` {
		    data {
				Par string
			}
			action {
				$result = f` + rnd + `("par",$Par) + f` + rnd + `("par","OK")
			}}
		`}, `Conditions`: {`true`}}
	_, idcnt, err = postTxResult(`EditContract`, &form)
	assert.NoError(t, err)

	_, msg, err = postTxResult(rnd, &url.Values{`Par`: {`finish`}})
	assert.NoError(t, err)
	assert.Equal(t, `Y=finishY=OK`, msg)
}

func TestGlobalVars(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	rnd := `rnd` + crypto.RandSeq(4)

	form := url.Values{`Value`: {`
		contract ` + rnd + ` {
		    data {
				Par string
			}
			action {
				$Par = $Par + "end"
				$key_id = 1234
				$result = Str($key_id) + $Par
			}}
		`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	err := postTx(`NewContract`, &form)
	if err == nil {
		t.Errorf(`must be error`)
		return
	} else if err.Error() != `{"type":"panic","error":"system variable $key_id cannot be changed"}` {
		t.Error(err)
		return
	}
	form = url.Values{`Value`: {`contract c_` + rnd + ` {
		data { Test string }
		action {
			$result = $Test + Str($ecosystem_id)
		}
	}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	err = postTx(`NewContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}

	form = url.Values{`Value`: {`
		contract a_` + rnd + ` {
			data { Par string}
			conditions {}
			action {
				var params map
				params["Test"] = "TEST"
				$aaa = 123
				if $Par == "b" {
				    $result = CallContract("b_` + rnd + `", params)
				} else {
				    $result = CallContract("c_` + rnd + `", params) + c_` + rnd + `("Test","OK")
				}
			}
		}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	err = postTx(`NewContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	form = url.Values{`Value`: {`contract b_` + rnd + ` {
			data { Test string }
			action {
				$result = $Test + $aaa
			}
		}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	err = postTx(`NewContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}

	err = postTx(`a_`+rnd, &url.Values{"Par": {"b"}})
	if err == nil {
		t.Errorf(`must be error aaa`)
		return
	} else if err.Error() != `{"type":"panic","error":"unknown extend identifier aaa"}` {
		t.Error(err)
		return
	}
	_, msg, err := postTxResult(`a_`+rnd, &url.Values{"Par": {"c"}})
	if err != nil {
		t.Error(err)
		return
	}
	if msg != `TEST1OK1` {
		t.Errorf(`wrong result %s`, msg)
		return
	}
}

func TestContractChain(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	rnd := `rnd` + crypto.RandSeq(4)

	form := url.Values{"Name": {rnd}, "ApplicationId": {"1"}, "Columns": {`[{"name":"value","type":"varchar", "index": "0", 
	  "conditions":"true"},
	{"name":"amount", "type":"number","index": "0", "conditions":"true"},
	{"name":"dt","type":"datetime", "index": "0", "conditions":"true"}]`},
		"Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	err := postTx(`NewTable`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	form = url.Values{`Value`: {`contract sub` + rnd + ` {
		data {
			Id int
		}
		action {
			$row = DBFind("` + rnd + `").Columns("value").WhereId($Id)
			if Len($row) != 1 {
				error "sub contract getting error"
			}
			$record = $row[0]
			$new = $record["value"]
			var val string
			val = $new+"="+$new
			DBUpdate("` + rnd + `", $Id, {"value": val })
		}
	}`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	err = postTx(`NewContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}

	form = url.Values{`Value`: {`contract ` + rnd + ` {
		data {
			Initial string
		}
		action {
			$id = DBInsert("` + rnd + `", {value: $Initial, amount:"0"})
			sub` + rnd + `($id)
			$row = DBFind("` + rnd + `").Columns("value").WhereId($id)
			if Len($row) != 1 {
				error "contract getting error"
			}
			$record = $row[0]
			$result = $record["value"]
		}
	}
		`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	err = postTx(`NewContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	_, msg, err := postTxResult(rnd, &url.Values{`Initial`: {rnd}})
	if err != nil {
		t.Error(err)
		return
	}
	if msg != rnd+`=`+rnd {
		t.Error(fmt.Errorf(`wrong result %s`, msg))
	}

	form = url.Values{`Value`: {`contract ` + rnd + `1 {
		action {
			DBInsert("` + rnd + `", {amount: 0,dt: "timestamp NOW()"})
		}
	}
		`}, "ApplicationId": {"1"}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	assert.EqualError(t, postTx(rnd+`1`, &url.Values{}),
		`{"type":"panic","error":"It is prohibited to use Now() function"}`)
}

func TestLoopCond(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	rnd := `rnd` + crypto.RandSeq(4)

	form := url.Values{`Value`: {`contract ` + rnd + `1 {
		conditions {
	    
		}
	}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	err := postTx(`NewContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	form = url.Values{`Value`: {`contract ` + rnd + `2 {
				conditions {
					ContractConditions("` + rnd + `1")
				}
			}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	err = postTx(`NewContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	var ret getContractResult
	err = sendGet(`contract/`+rnd+`1`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	sid := ret.TableID
	form = url.Values{`Value`: {`contract ` + rnd + `1 {
				conditions {
					ContractConditions("` + rnd + `2")
				}
			}`}, `Id`: {sid}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	err = postTx(`EditContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	assert.EqualError(t, postTx(rnd+`2`, &url.Values{}), `{"type":"panic","error":"There is loop in `+rnd+`1 contract"}`)

	form = url.Values{"Name": {`ecosystems`}, "InsertPerm": {`ContractConditions("MainCondition")`},
		"UpdatePerm":    {`EditEcosysName(1, "HANG")`},
		"NewColumnPerm": {`ContractConditions("MainCondition")`}}
	assert.NoError(t, postTx(`EditTable`, &form))
	assert.EqualError(t, postTx(`EditEcosystemName`, &url.Values{"EcosystemID": {`1`},
		"NewName": {`Hang`}}), `{"type":"panic","error":"There is loop in EditEcosysName contract"}`)

	form = url.Values{`Value`: {`contract ` + rnd + `shutdown {
		action
		{ DBInsert("` + rnd + `table", {"test": "SHUTDOWN"}) }
	}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	form = url.Values{
		"Name":          {rnd + `table`},
		"Columns":       {`[{"name":"test","type":"varchar", "index": "0", "conditions":"true"}]`},
		"ApplicationId": {"1"},
		"Permissions":   {`{"insert": "` + rnd + `shutdown()", "update" : "true", "new_column": "true"}`},
	}
	require.NoError(t, postTx("NewTable", &form))

	assert.EqualError(t, postTx(rnd+`shutdown`, &url.Values{}), `{"type":"panic","error":"There is loop in @1`+rnd+`shutdown contract"}`)
}

func TestRand(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	rnd := `rnd` + crypto.RandSeq(4)

	form := url.Values{`Value`: {`contract ` + rnd + ` {
		action {
			var result i int
			i = 3
			while i < 15 {
				var rnd int
				rnd = Random(0, 3*i)
				result = result + rnd
				i=i+1
			}
			$result = result
		}
	}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	_, val1, err := postTxResult(rnd, &url.Values{})
	assert.NoError(t, err)
	_, val2, err := postTxResult(rnd, &url.Values{})
	assert.NoError(t, err)
	// val1 == val2 for seed = blockId % 1
	if val1 != val2 {
		t.Errorf(`%s!=%s`, val1, val2)
	}
}
func TestKillNode(t *testing.T) {
	require.NoError(t, keyLogin(1))
	form := url.Values{"Name": {`MyTestContract1`}, "Value": {`contract MyTestContract1 {action {}}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}, "nowait": {`true`}}
	require.NoError(t, postTx(`NewContract`, &form))
	require.NoError(t, postTx("Kill", &url.Values{"nowait": {`true`}}))
}

func TestLoopCondExt(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	rnd := `rnd` + crypto.RandSeq(4)

	form := url.Values{`Value`: {`contract ` + rnd + `1 {
		conditions {

		}
	}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	err := postTx(`NewContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	form = url.Values{`Value`: {`contract ` + rnd + `2 {
		conditions {
			ContractConditions("` + rnd + `1")
		}
	}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	err = postTx(`NewContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	var ret getContractResult
	err = sendGet(`contract/`+rnd+`1`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	sid := ret.TableID
	form = url.Values{`Value`: {`contract ` + rnd + `1 {
		conditions {
			ContractConditions("` + rnd + `2")
		}
	}`}, `Id`: {sid}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	err = postTx(`EditContract`, &form)
	if err != nil {
		t.Error(err)
		return
	}
	err = postTx(rnd+`2`, &url.Values{})
	if err != nil {
		t.Error(err)
		return
	}
}

func TestBlockTransactions(t *testing.T) {
	require.NoError(t, keyLogin(1))

	rnd := `rnd` + crypto.RandSeq(4)

	form := url.Values{`Value`: {`contract ` + rnd + `1 {
		conditions {
	    
		}
	}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}

	require.NoError(t, postTx(`NewContract`, &form))

	var ret getContractResult
	require.NoError(t, sendGet(`contract/`+rnd+`1`, nil, &ret))

	var result map[int64][]TxInfo
	require.NoError(t, sendGet(`blocks?block_id=1&count=10`, nil, &result))

	fmt.Printf("%+v", result)
}

func TestCost(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	name := randName(`cnt`)

	form := url.Values{`Value`: {`contract ` + name + `1 {
		func my() {
			var i int
			while i < 1000 {
				i = i + 1
			}
		}
		conditions {
			var i int
			while i < 1000 {
				i = i + 1
			}
		}
		action {
			var i int
			while i < 10000 {
				i = i + 1
			}
			my()
			$result = "OK"
		}
	}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}

	require.NoError(t, postTx(`NewContract`, &form))

	form = url.Values{`Value`: {`contract ` + name + `2 {
		conditions {
			var i int
			while i < 1000 {
				i = i + 1
			}
		}
		action {
			var i int
			while i < 10000 {
				i = i + 1
			}
			` + name + `1()
			$result = "OK"
		}
	}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}

	require.NoError(t, postTx(`NewContract`, &form))

	require.NoError(t, postTx(name+`1`, &url.Values{}))
	require.NoError(t, postTx(name+`2`, &url.Values{}))
	t.Error(`OK`)

}

func TestHard(t *testing.T) {
	require.NoError(t, keyLogin(1))
	name := randName(`h`)
	form := url.Values{"Name": {name}, "Value": {`contract ` + name + ` {
		data {
			Par int
		}
		action {}}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	_, msg, err := postTxResult(`NewContract`, &form)
	require.NoError(t, err)
	fmt.Println(`MSg=`, msg, name)

	for i := 0; i < 1000; i++ {
		form = url.Values{"Id": {msg}, "Value": {fmt.Sprintf(`contract %s {action { 
			Println("OK %d")
		}}`, name, i)}, "Conditions": {`true`}, "nowait": {`true`}, "Par": {fmt.Sprintf("%d", i)}}
		if err = postTx( /*name */ `EditContract`, &form); err != nil {
			t.Error(err)
		}
	}
	t.Error(`OK`)
}

func TestInsert(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	name := randName(`cnt`)

	form := url.Values{`Value`: {`contract ` + name + `1 {
		conditions {
		}
		action {
			NewTable("Name,Columns,ApplicationId,Permissions", "` + name + `2",
				"[{\"name\":\"MyName\",\"type\":\"varchar\", \"index\": \"0\", \"conditions\":{\"update\":\"true\", \"read\":\"true\"}}]", 100,
				 "{\"insert\": \"true\", \"update\" : \"true\", \"new_column\": \"true\"}")
		}
	}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	require.NoError(t, postTx(`NewContract`, &form))

	form = url.Values{`Value`: {`contract ` + name + `2 {
		action {
			DBInsert("` + name + `2", {MyName: "insert"})
		}
	}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	require.NoError(t, postTx(`NewContract`, &form))

	require.NoError(t, postTx(name+`1`, &url.Values{}))
	require.NoError(t, postTx(name+`2`, &url.Values{}))
	t.Error(`OK`)
}

func TestErrors(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	name := randName(`cnt`)

	form := url.Values{`Value`: {`contract ` + name + `1 {
		action {
			// comment
			 DBFind("qq")
		}}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	assert.EqualError(t, postTx(name+`1`, &url.Values{}),
		`{"type":"panic","error":"pq: relation \"1_qq\" does not exist [DBSelect @1`+name+`1:4]"}`)

	form = url.Values{`Value`: {`contract ` + name + `2 {
				action {
					// comment
					var i int
					i = 1/0
				}}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	assert.EqualError(t, postTx(name+`2`, &url.Values{}),
		`{"type":"panic","error":"divided by zero [@1`+name+`2:5]"}`)

	form = url.Values{`Value`: {`contract ` + name + `5 {
			action {
				// comment
				Throw("Problem", "throw message")
			}}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	assert.EqualError(t, postTx(name+`5`, &url.Values{}),
		`{"type":"panic","error":"throw message [Throw @1`+name+`5:4]"}`)

	form = url.Values{`Value`: {`contract ` + name + `4 {
			action {
				// comment
				error("error message")
			}}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	assert.EqualError(t, postTx(name+`4`, &url.Values{}),
		`{"type":"error","error":"error message"}`)

	form = url.Values{`Value`: {`contract ` + name + `3 {
		        data {
					Par int
				}
				action {
					if $Par == 1 {
					   ` + name + `1()
					}
					if $Par == 2 {
						` + name + `2()
					 }
				 }}`}, `Conditions`: {`true`}, `ApplicationId`: {`1`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	assert.EqualError(t, postTx(name+`3`, &url.Values{`Par`: {`1`}}),
		`{"type":"panic","error":"pq: relation \"1_qq\" does not exist [DBSelect @1`+name+`1:4 @1`+name+`3:6]"}`)
	assert.EqualError(t, postTx(name+`3`, &url.Values{`Par`: {`2`}}),
		`{"type":"panic","error":"divided by zero [@1`+name+`2:5 @1`+name+`3:9]"}`)

	t.Error(`OK`)
}

func TestExternalNetwork(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	var form url.Values
	name := `cnt` + crypto.RandSeq(4)
	form = url.Values{"Name": {name}, "Value": {`contract ` + name + `Hashes {
		data {
			hash string
			block int
			UID    string
		}
		action { 
			Println("SUCCESS", $UID, $hash, $block )
			if $UID == "123456" {
				$result = "ok"
			}
		}
	}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	form = url.Values{"Name": {name}, "Value": {`contract ` + name + `Result {
		data {
			UID  string
			Status int
			Block int
			Msg   string "optional"
		}
		action { 
			Println("Result Contract", $UID, $Status, $Block, $Msg )
		}
	}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	form = url.Values{"Name": {name}, "Value": {`contract ` + name + `Errors {
		data {
			hash string
			block int
			UID    string "optional"
		}
		action { 
			if $UID == "stop" {
				error("Error message")
			}
			$result = 1/0
		}
	}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	form = url.Values{"Name": {name}, "Value": {`contract ` + name + `2 {
		action { 
			var params map
			params["hash"] = PubToHex($txhash)
			params["block"] = $block
			SendExternalTransaction( "123456", "http://localhost:7079", "@1` + name + `Hashes",   
			    params, "@1` + name + `Result")
			SendExternalTransaction( "654321", "http://localhost:7079", "@1` + name + `Hashes",  
			    params, "@1` + name + `Result")
			SendExternalTransaction( "stop", "http://localhost:7079", "@1` + name + `Errors", 
			    params, "@1` + name + `Result")
			SendExternalTransaction( "zero", "http://localhost:7079", "@1` + name + `Errors", 
			    params, "@1` + name + `Result")
		}
	}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	assert.NoError(t, postTx(name+`2`, &url.Values{}))

	form = url.Values{"Name": {name}, "Value": {`contract ` + name + `3 {
		action { 
			var params map
			params["hash"] = PubToHex($txhash)
			params["block"] = $block
			SendExternalTransaction( "77", "http://localhost:7079", "@1` + name + `Hashes",   
			    params, "@1None")
		}
	}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	assert.NoError(t, postTx(name+`3`, &url.Values{}))
}

func TestApos(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	name := randName(`cnt`)
	form := url.Values{"Name": {name}, "Value": {`contract ` + name + ` {
		data {
			Address string
		}
		action {
			var m map
			var id int
			m["member_name"] = "test"
			m["member_info->country"] = $Address 
			m["member_info->ooops"] = "seses' seseses "
			id = DBInsert("members", m)
			m["member_info->new"] = "ok'; ok"
			m["memb'er_info->ne'wq"] = "stop'"
			DBUpdate("members", id, m)
		}}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	assert.NoError(t, postTx(name, &url.Values{`Address`: {"Name d'Company"}}))
}

func TestCondition(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	var form url.Values

	name := `cnt` + crypto.RandSeq(4)
	form = url.Values{"Name": {name}, "Value": {`contract ` + name + ` {
		action { 
			Println("COND" )
		}
	}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))
	form = url.Values{"Name": {name + `2`}, "Value": {`contract ` + name + `2 {
		conditions {
		}
		action { 
			Println("COND 2" )
		}
	}`},
		"ApplicationId": {`1`}, "Conditions": {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	assert.NoError(t, postTx(`NewPage`, &url.Values{
		"ApplicationId": {`1`},
		"Name":          {name},
		"Value":         {`Div(Body: "Condition 2 - test")`},
		"Menu":          {`default_menu`},
		"Conditions":    {`ContractConditions("` + name + `2")`},
	}))
	var ret listResult
	err := sendGet(`list/pages`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	id := strconv.FormatInt(ret.Count, 10)

	assert.NoError(t, postTx(`EditPage`, &url.Values{
		"Id":         {id},
		"Value":      {`Div(Body: "Condition 1 - test")`},
		"Conditions": {`ContractConditions("` + name + `")`},
	}))

	assert.NoError(t, postTx(`EditPage`, &url.Values{
		"Id":         {id},
		"Value":      {`Div(Body: "Condition - test")`},
		"Conditions": {`true`},
	}))
}

func TestCurrentKeyFromAccount(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	name := randName(t.Name())
	form := url.Values{
		"Name": {name},
		"Value": {`contract ` + name + ` {
			data {
				Account string
			}
			action {
				info CurrentKeyFromAccount($Account)
			}
		}`},
		"ApplicationId": {"1"},
		"Conditions":    {"true"},
	}
	assert.NoError(t, postTx("NewContract", &form))
	expected := fmt.Sprintf(`{"type":"info","error":"%d"}`, converter.StringToAddress(gAddress))
	assert.Error(t, postTx(name, &url.Values{`Account`: {gAddress}}), expected)
}
