/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package utils

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestBanError(t *testing.T) {
	cases := map[error]bool{
		errors.New("case 1"):                                  false,
		WithBan(errors.New("case 2")):                         true,
		errors.Wrap(errors.New("case 3"), "message"):          false,
		errors.Wrap(WithBan(errors.New("case 4")), "message"): true,
	}

	for err, ok := range cases {
		assert.Equal(t, ok, IsBanError(err), err.Error())
	}
}
