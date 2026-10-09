/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	stdErrors "errors"
	"strings"
	"testing"
)

func TestHistory(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}

	var ret historyResult
	err := sendGet("history/pages/1", nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	if len(ret.List) == 0 {
		t.Error(stdErrors.New("History should not be empty"))
	}

	// No history of a row the reader does not see
	err = sendGet("history/pages/1000", nil, &ret)
	if err == nil || !strings.Contains(err.Error(), "E_NOTFOUND") {
		t.Error(stdErrors.New("History of a missing row should not be found"), err)
	}
}
