/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/hex"
	"encoding/json"
	"net/url"
	priv, pub, err := crypto.GenHexKeys()
	if err != nil {
		t.Error(err)
		return
	}
	sign, err := crypto.SignString(priv, `LOGIN`+ret.NetworkID+ret.UID)
	if err != nil {
		t.Error(err)
		return
	}
	form := url.Values{"pubkey": {pub}, "signature": {hex.EncodeToString(sign)}}
	var lret loginResult
	err = sendPost(`login`, &form, &lret)
	if err != nil {
		t.Error(err)
		return
	}
	gAuth = lret.Token
}

func TestNetwork(t *testing.T) {
	var ret NetworkResult
	assert.NoError(t, sendGet(`network`, nil, &ret))
	if len(ret.NetworkID) == 0 || len(ret.CentrifugoURL) == 0 || len(ret.HonorNodes) == 0 {
		t.Error(`Wrong value`, ret)
	}
}
