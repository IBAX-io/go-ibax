/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package api

import (
	"github.com/IBAX-io/go-ibax/packages/model"
	"testing"
)

func TestMineCount(t *testing.T) {
	var ret model.Response
	err := sendGet(`mintcount/163`, nil, &ret)
	if err != nil {
