/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import (
	"testing"
		{[]string{"test", "test"}, 20},
		{map[string]string{"test": "test"}, 12},
	}

	for _, v := range cases {
		assert.Equal(t, v.mem, calcMem(v.v))
	}
}
