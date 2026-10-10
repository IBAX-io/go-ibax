/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"crypto/mldsa"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

const suitesSource = "go-ibax packages/common/crypto via tools/cryptovectors: PrivateToPublic, Address and Sign " +
	"(goSignature, over message) for every cryptoer and hasher go-ibax implements; clientSignature is the client's " +
	"transaction signature over payload, checked like utils.CheckSign: Verify(publicKey, DoubleHash(payload)); " +
	"contextFreeSignature (ML-DSA only) is a valid FIPS 204 signature of Hash(message) under the empty context, " +
	"which the node and the client must refuse. Test keys only."

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
	// ML-DSA only: the same key and digest signed under the empty context instead of the chain's
	// context string; a valid FIPS 204 signature that both sides must refuse
	ContextFreeSignature string `json:"contextFreeSignature,omitempty"`
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
		pub, err := crypto.NodePrivateToPublic(priv)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", label, err)
		}
		v.PublicKey = crypto.PubToHex(pub)
		v.KeyID = strconv.FormatInt(crypto.Address(pub), 10)

		if v.GoSignature == nil || !verifies(pub, []byte(v.Message), *v.GoSignature) {
			sig, err := crypto.NodeSign(priv, []byte(v.Message))
			if err != nil {
				return nil, nil, fmt.Errorf("%s: sign: %w", label, err)
			}
			s := hex.EncodeToString(sig)
			v.GoSignature = &s
		}

		if params, ok := mldsaParameters[v.Cryptoer]; ok {
			if v.ContextFreeSignature, err = contextFreeSignature(params(), priv, []byte(v.Message), v.ContextFreeSignature); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", label, err)
			}
			if verifies(pub, []byte(v.Message), v.ContextFreeSignature) {
				problems = append(problems, fmt.Sprintf("%s: node accepts a signature made under the empty context", label))
			}
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
	ok, err := crypto.NodeVerify(pub, data, sig)
	return ok && err == nil
}

// mldsaParameters are the FIPS 204 parameter sets of the ML-DSA cryptoers
var mldsaParameters = map[string]func() mldsa.Parameters{
	"MLDSA65": mldsa.MLDSA65,
	"MLDSA87": mldsa.MLDSA87,
}

// contextFreeSignature returns stored while it is still a valid empty-context signature of
// Hash(message), and a fresh one otherwise.
func contextFreeSignature(params mldsa.Parameters, seed, message []byte, stored string) (string, error) {
	sk, err := mldsa.NewPrivateKey(params, seed)
	if err != nil {
		return "", err
	}
	digest := crypto.Hash(message)
	if sig, err := hex.DecodeString(stored); err == nil && stored != "" && mldsa.Verify(sk.PublicKey(), digest, sig, nil) == nil {
		return stored, nil
	}
	sig, err := sk.Sign(nil, digest, nil)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sig), nil
}
