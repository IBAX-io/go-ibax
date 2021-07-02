/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
			Filter:       ` WHERE closed = false AND recipient_id IN (?) `,
			ParamsLength: 1,
		},
		testItem{
			Input:        nil,
			Filter:       ` WHERE closed = false `,
			ParamsLength: 0,
		},
	}

	for i, item := range testTable {
		filter, params := getNotificationCountFilter(item.Input, 1)
		assert.Equal(t, item.Filter, filter, "on %d step wrong filter %s", i, filter)
		assert.Equal(t, item.ParamsLength, len(params), "on %d step wrong params length %d", i, len(params))
	}

}
