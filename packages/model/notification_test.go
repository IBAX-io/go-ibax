/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testItem struct {
	Input        []int64
	Filter       string
	ParamsLength int
}

func TestGetNotificationCountFilter(t *testing.T) {
	testTable := []testItem{
		testItem{
			Input:        []int64{3, 5},
			Filter:       ` WHERE closed = false AND recipient_id IN (?) `,
			ParamsLength: 1,
		},
		testItem{
			Input:        nil,
			Filter:       ` WHERE closed = false `,
