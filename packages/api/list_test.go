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
