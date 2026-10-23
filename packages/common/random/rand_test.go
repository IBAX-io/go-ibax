/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package random

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRand(t *testing.T) {
	cases := [][]int64{
		{3434102771992637744, 1523931518789473682},
		{3434102771992637744, 1523931518789473682},
	}

	for _, values := range cases {
		r := TxRand([]byte("reset"))
		for _, v := range values {
			assert.Equal(t, v, r.Int63())
		}
	}
}
