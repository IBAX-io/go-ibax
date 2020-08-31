/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import (
	"testing"

	"github.com/stretchr/testify/assert"
)


	for _, v := range cases {
		assert.Equal(t, v.mem, calcMem(v.v))
	}
}
