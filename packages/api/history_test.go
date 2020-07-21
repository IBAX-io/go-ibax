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

	var ret historyResult
	err := sendGet("history/pages/1", nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
