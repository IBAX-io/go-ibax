/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"fmt"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

func TestLimit(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	rnd := randName(``)
	form := url.Values{"Name": {"tbl" + rnd}, "Columns": {`[{"name":"name","type":"number",   "conditions":"true"},
	{"name":"block", "type":"varchar","conditions":"true"}]`},
		"Permissions": {`{"insert": "true", "update" : "true", "new_column": "true"}`}}
	assert.NoError(t, postTx(`NewTable`, &form))

	form = url.Values{`Value`: {`contract Limit` + rnd + ` {
		data {
			Num int
		}
		conditions {
		}
		action {
		   DBInsert("tbl` + rnd + `", {name: $Num, block: $block}) 
		}
	}`}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	form = url.Values{`Value`: {`contract Upd` + rnd + ` {
		data {
			Name string
			Value string
		}
		conditions {
		}
		action {
		   DBUpdatePlatformParam($Name, $Value, "") 
		}
	}`}, `Conditions`: {`true`}}
	assert.NoError(t, postTx(`NewContract`, &form))

	all := 10
	sendList := func() {
		for i := 0; i < all; i++ {
			assert.NoError(t, postTx(`Limit`+rnd, &url.Values{
				`Num`:    {converter.IntToStr(i)},
				`nowait`: {`true`},
			}))
		}
		time.Sleep(10 * time.Second)
	}
	checkList := func(count, wantBlocks int) (err error) {
		var list listResult
		err = sendGet(`list/tbl`+rnd, nil, &list)
		if err != nil {
			return
		}
		if converter.StrToInt(strconv.FormatInt(list.Count, 10)) != count {
			return fmt.Errorf(`wrong list items %d != %d`, list.Count, count)
		}
		blocks := make(map[string]int)
		for _, item := range list.List {
			if v, ok := blocks[item["block"]]; ok {
				blocks[item["block"]] = v + 1
			} else {
				blocks[item["block"]] = 1
			}
		}
		if wantBlocks > 0 && len(blocks) != wantBlocks {
			return fmt.Errorf(`wrong number of blocks %d != %d`, len(blocks), wantBlocks)
		}
		return nil
	}
	sendList()
	assert.NoError(t, checkList(10, 1))

	var syspar paramsResult
	assert.NoError(t, sendGet(`systemparams?names=max_tx_block,max_tx_block_per_user`, nil, &syspar))

	var maxusers, maxtx string
	if syspar.List[0].Name == "max_tx_block" {
		maxusers = syspar.List[1].Value
		maxtx = syspar.List[0].Value
	} else {
		maxusers = syspar.List[0].Value
		maxtx = syspar.List[1].Value
	}
	restoreMax := func() {
		assert.NoError(t, postTx(`Upd`+rnd, &url.Values{`Name`: {`max_tx_block`}, `Value`: {maxtx}}))
		assert.NoError(t, postTx(`Upd`+rnd, &url.Values{`Name`: {`max_tx_block_per_user`}, `Value`: {maxusers}}))
	}
	defer restoreMax()

	assert.NoError(t, postTx(`Upd`+rnd, &url.Values{`Name`: {`max_tx_block`}, `Value`: {`7`}}))

	sendList()
	assert.NoError(t, checkList(20, 3))
	assert.NoError(t, postTx(`Upd`+rnd, &url.Values{`Name`: {`max_tx_block_per_user`}, `Value`: {`3`}}))

	sendList()
	assert.NoError(t, checkList(30, 7))

	restoreMax()
	assert.NoError(t, sendGet(`systemparams?names=max_block_generation_time`, nil, &syspar))

	var maxtime string
	maxtime = syspar.List[0].Value
	defer func() {
		assert.NoError(t, postTx(`Upd`+rnd, &url.Values{
			`Name`:  {`max_block_generation_time`},
			`Value`: {maxtime},
		}))
	}()
	assert.NoError(t, postTx(`Upd`+rnd, &url.Values{`Name`: {`max_block_generation_time`}, `Value`: {`100`}}))

	sendList()
	assert.NoError(t, checkList(40, 0))
}
