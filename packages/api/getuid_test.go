/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/hex"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"

	"github.com/stretchr/testify/assert"
)

func TestGetUID(t *testing.T) {
	var ret getUIDResult
	err := sendGet(`getuid`, nil, &ret)
	if err != nil {
		var v map[string]string
		json.Unmarshal([]byte(err.Error()[4:]), &v)
		t.Error(err)
		return
	}
	gAuth = ret.Token
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
