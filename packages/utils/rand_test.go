/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package utils

import (
	}

	rand := NewRand(0)
	for _, values := range cases {
		r := rand.BytesSeed([]byte("reset"))
		for _, v := range values {
			assert.Equal(t, v, r.Int63())
		}
	}
}
