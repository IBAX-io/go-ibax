/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

	if len(ret.Contracts) == 0 {
		t.Error("incorrect contracts count")
	}

	if len(ret.Pages) == 0 {
		t.Error("incorrent pages count")
	}
}
