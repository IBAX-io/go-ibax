/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package syspar

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHonorNode(t *testing.T) {
	cases := []struct {
		value,
		err string
		formattingErr bool
	}{
		{value: `[{"tcp_address":"127.0.0.1", "api_address":"https://127.0.0.1", "key_id":"100", "public_key":"c1a9e7b2fb8cea2a272e183c3e27e2d59a3ebe613f51873a46885c9201160bd263ef43b583b631edd1284ab42483712fd2ccc40864fe9368115ceeee47a7c7d0", "unban_time": 111111}]`, err: ``},
		{value: `[{"tcp_address":"", "api_address":"https://127.0.0.1", "key_id":"100", "public_key":"c1a9e7b2fb8cea2a272e183c3e27e2d59a3ebe613f51873a46885c9201160bd263ef43b583b631edd1284ab42483712fd2ccc40864fe9368115ceeee47a7c7d0", "unban_time": 111111}]`, err: `Invalid values of the honor_nodes parameter`},
		{value: `[{"tcp_address":"127.0.0.1", "api_address":"127.0.0.1", "key_id":"100", "public_key":"c1a9e7b2fb8cea2a272e183c3e27e2d59a3ebe613f51873a46885c9201160bd263ef43b583b631edd1284ab42483712fd2ccc40864fe9368115ceeee47a7c7d0", "unban_time": 111111}]`, err: `parse 127.0.0.1: invalid URI for request`},
		{value: `[{"tcp_address":"127.0.0.1", "api_address":"https://", "key_id":"100", "public_key":"c1a9e7b2fb8cea2a272e183c3e27e2d59a3ebe613f51873a46885c9201160bd263ef43b583b631edd1284ab42483712fd2ccc40864fe9368115ceeee47a7c7d0", "unban_time": 111111}]`, err: `Invalid host: https://`},
		{value: `[{"tcp_address":"127.0.0.1", "api_address":"https://127.0.0.1", "key_id":"0", "public_key":"c1a9e7b2fb8cea2a272e183c3e27e2d59a3ebe613f51873a46885c9201160bd263ef43b583b631edd1284ab42483712fd2ccc40864fe9368115ceeee47a7c7d0", "unban_time": 111111}]`, err: `Invalid values of the honor_nodes parameter`},
		{value: `[{"tcp_address":"127.0.0.1", "api_address":"https://127.0.0.1", "key_id":"100", "public_key":"c1a9e7b2fb8cea2a272e183c3e27e2d59a3ebe613f51873a46885c9201160bd263ef43b583b631edd1284ab42483712fd2ccc40864fe9368115ceeee47a7c7d00000000000", "unban_time": 111111}]`, err: `Invalid values of the honor_nodes parameter`},
		{value: `[{}}]`, err: `invalid character '}' after array element`, formattingErr: true},
	}
	for _, v := range cases {
		// Testing Unmarshalling string -> struct
		var fs []*HonorNode
		err := json.Unmarshal([]byte(v.value), &fs)
		if len(v.err) == 0 {
			assert.NoError(t, err)
		} else {
			assert.EqualError(t, err, v.err)
		}

		// Testing Marshalling struct -> string
		blah, err := json.Marshal(fs)
		require.NoError(t, err)

		// Testing Unmarshaling string (from struct) -> struct
		var unfs []HonorNode
		err = json.Unmarshal(blah, &unfs)
		if !v.formattingErr && len(v.err) != 0 {
			assert.EqualError(t, err, v.err)
		}
	}
}
