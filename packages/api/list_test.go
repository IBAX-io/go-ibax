/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

func TestList(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}
	var ret listResult
	err := sendGet(`list/contracts`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	if converter.StrToInt64(strconv.FormatInt(ret.Count, 10)) < 7 {
		t.Error(fmt.Errorf(`The number of records %d < 7`, ret.Count))
		return
	}
	err = sendGet(`list/qwert`, nil, &ret)
	if err.Error() != `404 {"error":"E_TABLENOTFOUND","msg":"Table 1_qwert has not been found"}` {
		t.Error(err)
		return
	}
	var retTable tableResult
	for _, item := range []string{`app_params`, `parameters`} {
		err = sendGet(`table/`+item, nil, &retTable)
		if err != nil {
			t.Error(err)
			return
		}
		if retTable.Name != item {
			t.Errorf(`wrong table name %s != %s`, retTable.Name, item)
			return
		}
	}
	var sec listResult
	err = sendGet(`sections`, nil, &sec)
	if err != nil {
		t.Error(err)
		return
	}
	if sec.Count == 0 {
		t.Errorf(`section error`)
		return
	}
}
