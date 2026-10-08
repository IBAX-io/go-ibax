/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package syspar

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHonorNode(t *testing.T) {
	crypto.InitAsymAlgo("ECC_P256")
	const key = "c1a9e7b2fb8cea2a272e183c3e27e2d59a3ebe613f51873a46885c9201160bd263ef43b583b631edd1284ab42483712fd2ccc40864fe9368115ceeee47a7c7d0"
	node := func(tcp, api, pub string) string {
		return fmt.Sprintf(`[{"tcp_address":%q, "api_address":%q, "public_key":%q, "unban_time": 111111}]`, tcp, api, pub)
	}
	cases := []struct {
		value,
		err string
		formattingErr bool
	}{
		{value: node("127.0.0.1", "https://127.0.0.1", key)},
		{value: node("127.0.0.1", "https://127.0.0.1", "04"+key)},
		{value: node("", "https://127.0.0.1", key), err: `invalid values of the honor_nodes parameter`},
		{value: node("127.0.0.1", "127.0.0.1", key), err: `parse "127.0.0.1": invalid URI for request`},
		{value: node("127.0.0.1", "https://", key), err: `invalid host: https://`},
		{value: node("127.0.0.1", "https://127.0.0.1", key+"00000000"), err: `invalid values of the honor_nodes parameter`},
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

// A node key is a public key of the network's own cryptoer: an ML-DSA-65 network takes 1952-byte
// keys and refuses curve keys, and the other way round
func TestHonorNodeKeyOfNetworkCryptoer(t *testing.T) {
	defer crypto.InitAsymAlgo("ECC_P256")
	keys := map[string]string{}
	for _, cryptoer := range []string{"ECC_P256", "MLDSA65"} {
		crypto.InitAsymAlgo(cryptoer)
		_, pub, err := crypto.GenKeyPair()
		require.NoError(t, err)
		keys[cryptoer] = crypto.PubToHex(pub)
	}
	node := func(key string) []byte {
		return []byte(fmt.Sprintf(`[{"tcp_address":"127.0.0.1:7078","api_address":"http://127.0.0.1:7079","public_key":%q}]`, key))
	}
	for _, cryptoer := range []string{"ECC_P256", "MLDSA65"} {
		crypto.InitAsymAlgo(cryptoer)
		for keyType, key := range keys {
			var nodes []*HonorNode
			err := json.Unmarshal(node(key), &nodes)
			if keyType == cryptoer {
				require.NoError(t, err, "%s key on a %s network", keyType, cryptoer)
				pub, _ := hex.DecodeString(key)
				assert.Equal(t, crypto.PublicKeySize(), len(crypto.CutPub(pub)))
			} else {
				assert.EqualError(t, err, errHonorNodeInvalidValues.Error(), "%s key on a %s network", keyType, cryptoer)
			}
		}
	}
}
