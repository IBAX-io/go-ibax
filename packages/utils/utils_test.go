/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package utils

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestToSnakeCase(t *testing.T) {
	cases := []struct {
		arg, expected string
	}{
		{"Contains", "contains"},
		{"AddressToId", "address_to_id"},
		{"HMac", "h_mac"},
		{"JSONEncode", "json_encode"},
		{"Hash", "hash"},
		{"PubToID", "pub_to_id"},
	}
	for _, tt := range cases {
		assert.Equal(t, tt.expected, ToSnakeCase(tt.arg))
	}
}

		} else {
			fmt.Println("time not ok")
		}
		time.Sleep(2 * time.Second)
	}
}
