/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	stdErrors "errors"
	"testing"
)

func TestHistory(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}

	}

	err = sendGet("history/pages/1000", nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	if len(ret.List) != 0 {
		t.Error(stdErrors.New("History should be empty"))
	}
}
