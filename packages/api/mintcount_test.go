/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package api

import (
	"github.com/IBAX-io/go-ibax/packages/model"
	"testing"
	err := sendGet(`mintcount/163`, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}

}
