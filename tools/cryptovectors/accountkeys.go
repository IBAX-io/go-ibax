/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

const accountKeysSource = "go-ibax packages/common/crypto via tools/cryptovectors: account keys of the private " +
	"keys of the SHA256 vectors of go-ibax-vectors.json (AccountKeyOf), each with the goSignature of its message: the key itself (accepted), " +
	"the key relabelled as every other algorithm, and malformed encodings. node.accepted is ParseAccountKey and " +
	"AccountKey.Verify(message, signature) both succeeding; node.error is why the node refuses. A client must " +
	"refuse every key the node refuses. Test keys only."

// accountKeysHasher is the hasher of the suite vectors the account key vectors come from
const accountKeysHasher = "SHA256"

type accountKeysFile struct {
	Source  string             `json:"source"`
	Vectors []accountKeyVector `json:"vectors"`
}

type accountKeyVector struct {
	Name      string        `json:"name"`
	Hasher    string        `json:"hasher"`
	PublicKey string        `json:"publicKey"`
	Message   string        `json:"message"`
	Signature string        `json:"signature"`
	Node      accountKeyRes `json:"node"`
}

type accountKeyRes struct {
	Accepted bool   `json:"accepted"`
	Error    string `json:"error,omitempty"`
}

// updateAccountKeys derives the account key vectors from the suite vectors in dir, so it rewrites
// the whole file
func updateAccountKeys(dir string) func(raw []byte) (any, []string, error) {
	return func([]byte) (any, []string, error) {
		raw, err := os.ReadFile(filepath.Join(dir, "go-ibax-vectors.json"))
		if err != nil {
			return nil, nil, err
		}
		var suites suitesFile
		if err := decode(raw, &suites); err != nil {
			return nil, nil, err
		}
		if err := useSuite(crypto.AsymAlgo_ECC_P256.String(), accountKeysHasher); err != nil {
			return nil, nil, err
		}
		f := accountKeysFile{Source: accountKeysSource, Vectors: []accountKeyVector{}}
		var problems []string
		seen := map[string]bool{}
		for _, s := range suites.Vectors {
			if s.Hasher != accountKeysHasher || seen[s.Cryptoer] || s.GoSignature == nil {
				continue
			}
			seen[s.Cryptoer] = true
			priv, err := hex.DecodeString(s.PrivateKey)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: private key: %w", s.Cryptoer, err)
			}
			key, err := crypto.AccountKeyOf(crypto.AsymAlgo(crypto.AsymAlgo_value[s.Cryptoer]), priv)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", s.Cryptoer, err)
			}
			for _, c := range accountKeyCases(key) {
				v := accountKeyVector{Name: c.name, Hasher: accountKeysHasher, PublicKey: hex.EncodeToString(c.encoded),
					Message: s.Message, Signature: *s.GoSignature}
				v.Node = checkAccountKey(c.encoded, []byte(v.Message), *s.GoSignature)
				if v.Node.Accepted != c.accepted {
					problems = append(problems, fmt.Sprintf("%s: accepted %v: %s", c.name, v.Node.Accepted, v.Node.Error))
				}
				f.Vectors = append(f.Vectors, v)
			}
		}
		return f, problems, nil
	}
}

type accountKeyCase struct {
	name     string
	encoded  []byte
	accepted bool
}

// accountKeyCases are the key itself, relabelled as every other algorithm, and malformed
func accountKeyCases(key crypto.AccountKey) []accountKeyCase {
	tagged := func(algo byte, raw []byte) []byte {
		return append([]byte{crypto.AccountKeyMarker, algo}, raw...)
	}
	cases := []accountKeyCase{{key.Algo.String() + " key", key.Bytes(), true}}
	var algos []crypto.AsymAlgo
	for v := range crypto.AsymAlgo_name {
		if a := crypto.AsymAlgo(v); a != key.Algo && crypto.AccountAlgoImplemented(a) {
			algos = append(algos, a)
		}
	}
	slices.Sort(algos)
	for _, a := range algos {
		cases = append(cases, accountKeyCase{fmt.Sprintf("%s key relabelled %s", key.Algo, a), tagged(byte(a), key.Raw), false})
	}
	cases = append(cases,
		accountKeyCase{key.Algo.String() + " bare key", bytes.Clone(key.Raw), false},
		accountKeyCase{key.Algo.String() + " key relabelled ECC_P512", tagged(byte(crypto.AsymAlgo_ECC_P512), key.Raw), false},
		accountKeyCase{key.Algo.String() + " key with an unknown algorithm", tagged(99, key.Raw), false},
		accountKeyCase{key.Algo.String() + " key short of a byte", key.Bytes()[:len(key.Bytes())-1], false},
		accountKeyCase{key.Algo.String() + " key with a byte more", append(key.Bytes(), 0), false},
	)
	if len(key.Raw) == 64 {
		cases = append(cases, accountKeyCase{key.Algo.String() + " SEC1 key (04, X, Y)", append([]byte{4}, key.Raw...), false})
	}
	return cases
}

func checkAccountKey(encoded, message []byte, signatureHex string) accountKeyRes {
	key, err := crypto.ParseAccountKey(encoded)
	if err != nil {
		return accountKeyRes{Error: err.Error()}
	}
	sig, err := hex.DecodeString(signatureHex)
	if err != nil {
		return accountKeyRes{Error: err.Error()}
	}
	ok, err := key.Verify(message, sig)
	if err != nil {
		return accountKeyRes{Error: err.Error()}
	}
	if !ok {
		return accountKeyRes{Error: "the signature does not verify"}
	}
	return accountKeyRes{Accepted: true}
}
