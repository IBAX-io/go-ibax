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
		t.Error("incorrent pages count")
	}
}
