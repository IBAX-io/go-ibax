/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

const suitesSource = "go-ibax packages/common/crypto via tools/cryptovectors: PrivateToPublic, Address and Sign " +
	"(goSignature, over message) for every cryptoer and hasher go-ibax implements; clientSignature is the client's " +
	"transaction signature over payload, checked like utils.CheckSign: Verify(publicKey, DoubleHash(payload)). Test keys only."

type suitesFile struct {
	Source  string        `json:"source"`
	Vectors []suiteVector `json:"vectors"`
}

type suiteVector struct {
	// inputs
	Cryptoer   string `json:"cryptoer"`
	Hasher     string `json:"hasher"`
	PrivateKey string `json:"privateKey"`
	Message    string `json:"message"`
	Payload    string `json:"payload"`
	// node side
	PublicKey   string  `json:"publicKey"`
	KeyID       string  `json:"keyID"`
	GoSignature *string `json:"goSignature"`
	// client side, written by the client; the node must accept it
	ClientSignature string `json:"clientSignature,omitempty"`
}

func useSuite(cryptoer, hasher string) error {
	if _, ok := crypto.AsymAlgo_value[cryptoer]; !ok {
		return fmt.Errorf("unknown cryptoer %q", cryptoer)
	}
	if _, ok := crypto.HashAlgo_value[hasher]; !ok {
		return fmt.Errorf("unknown hasher %q", hasher)
	}
	crypto.InitAsymAlgo(cryptoer)
	crypto.InitHashAlgo(hasher)
	return nil
}

func updateSuites(raw []byte) (any, []string, error) {
	var f suitesFile
	if err := decode(raw, &f); err != nil {
		return nil, nil, err
	}
	var problems []string
	f.Source = suitesSource
	for i := range f.Vectors {
		v := &f.Vectors[i]
		label := fmt.Sprintf("%s/%s %s", v.Cryptoer, v.Hasher, v.PrivateKey[:6])
		if err := useSuite(v.Cryptoer, v.Hasher); err != nil {
			return nil, nil, err
		}
		priv, err := hex.DecodeString(v.PrivateKey)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: private key: %w", label, err)
		}
		pub, err := crypto.PrivateToPublic(priv)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", label, err)
		}
		v.PublicKey = crypto.PubToHex(pub)
		v.KeyID = strconv.FormatInt(crypto.Address(pub), 10)

		if v.GoSignature == nil || !verifies(pub, []byte(v.Message), *v.GoSignature) {
			sig, err := crypto.Sign(priv, []byte(v.Message))
			if err != nil {
				return nil, nil, fmt.Errorf("%s: sign: %w", label, err)
			}
			s := hex.EncodeToString(sig)
			v.GoSignature = &s
		}

		if v.ClientSignature != "" {
			payload, err := hex.DecodeString(v.Payload)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: payload: %w", label, err)
			}
			if !verifies(pub, crypto.DoubleHash(payload), v.ClientSignature) {
				problems = append(problems, fmt.Sprintf("%s: node refuses clientSignature", label))
			}
		}
	}
	return f, problems, nil
}

func verifies(pub, data []byte, signatureHex string) bool {
	sig, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	ok, err := crypto.Verify(pub, data, sig)
	return ok && err == nil
}
