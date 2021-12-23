/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppContent(t *testing.T) {
	assert.NoError(t, keyLogin(1))

	var ret appContentResult
	err := sendGet(`appcontent/1`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}

	if len(ret.Snippets) == 0 {
		t.Error("incorrect snippets count")
	}

	if len(ret.Contracts) == 0 {
		t.Error("incorrect contracts count")
	}

	if len(ret.Pages) == 0 {
		t.Error("incorrent pages count")
	}
}
