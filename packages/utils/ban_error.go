/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package utils

import (
	"github.com/pkg/errors"
)

type BanError struct {
	err error
}

func IsBanError(err error) bool {
	err = errors.Cause(err)
	if _, ok := err.(*BanError); ok {
		return true
	}
	return false
}
