/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	log "github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	log.SetLevel(log.PanicLevel)
	os.Exit(m.Run())
}

// The committed vectors are exactly what this node computes
func TestVectorsUpToDate(t *testing.T) {
	diffs, err := run("testdata", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diffs {
		t.Error(d)
	}
}

// Any edit to a node-side field, or a client signature the node refuses, must fail the check
func TestTamperedVectorsFail(t *testing.T) {
	mutations := []struct {
		name   string
		file   string
		mutate func(doc map[string]any)
	}{
		{"public key", "go-ibax-vectors.json", func(d map[string]any) { setAt(d, "vectors", 0, "publicKey", "04"+repeat("ab", 64)) }},
		{"key id", "go-ibax-vectors.json", func(d map[string]any) { setAt(d, "vectors", 5, "keyID", "1") }},
		{"P256 key id", "go-ibax-vectors.json", func(d map[string]any) { setAt(d, "vectors", 13, "keyID", "-1") }},
		{"SM2 public key", "go-ibax-vectors.json", func(d map[string]any) { setAt(d, "vectors", 30, "publicKey", "04"+repeat("cd", 64)) }},
		{"ML-DSA-65 public key", "go-ibax-vectors.json", func(d map[string]any) { setAt(d, "vectors", 37, "publicKey", repeat("ab", 1952)) }},
		{"ML-DSA-65 client signature", "go-ibax-vectors.json", func(d map[string]any) { setAt(d, "vectors", 40, "clientSignature", repeat("22", 3309)) }},
		{"ML-DSA-65 context-free signature", "go-ibax-vectors.json", func(d map[string]any) {
			v := d["vectors"].([]any)[36].(map[string]any)
			v["contextFreeSignature"] = v["goSignature"]
		}},
		{"node signature", "go-ibax-vectors.json", func(d map[string]any) { setAt(d, "vectors", 1, "goSignature", repeat("11", 64)) }},
		{"client signature", "go-ibax-vectors.json", func(d map[string]any) { setAt(d, "vectors", 2, "clientSignature", repeat("22", 64)) }},
		{"address id", "go-ibax-addresses.json", func(d map[string]any) { setAt(d, "cases", 0, "id", "597920150864192935") }},
		{"address text", "go-ibax-addresses.json", func(d map[string]any) { setAt(d, "cases", 1, "address", "0059-7920-1508-6419-2935") }},
		{"transfer hash", "go-ibax-transfers.json", func(d map[string]any) { nodeOf(d, 0)["hash"] = repeat("00", 32) }},
		{"transfer accepted instead of refused", "go-ibax-transfers.json", func(d map[string]any) {
			for i := range d["cases"].([]any) {
				if _, refused := nodeOf(d, i)["error"]; refused {
					d["cases"].([]any)[i].(map[string]any)["node"] = map[string]any{"type": 5}
					return
				}
			}
		}},
		{"transfer data", "go-ibax-transfers.json", func(d map[string]any) {
			c := d["cases"].([]any)[0].(map[string]any)
			data := []byte(c["data"].(string))
			data[len(data)-1] = map[bool]byte{true: '1', false: '0'}[data[len(data)-1] == '0']
			c["data"] = string(data)
		}},
		{"contract header", "go-ibax-contract-params.json", func(d map[string]any) { d["header"] = "ID=1" }},
		{"contract param", "go-ibax-contract-params.json", func(d map[string]any) {
			d["node"].(map[string]any)["F1"] = "int64 5"
		}},
	}
	for _, m := range mutations {
		m := m
		t.Run(m.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range vectorFiles {
				raw, err := os.ReadFile(filepath.Join("testdata", f.name))
				if err != nil {
					t.Fatal(err)
				}
				if f.name == m.file {
					var doc map[string]any
					if err := json.Unmarshal(raw, &doc); err != nil {
						t.Fatal(err)
					}
					m.mutate(doc)
					if raw, err = encode(doc); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(filepath.Join(dir, f.name), raw, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			diffs, err := run(dir, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(diffs) == 0 {
				t.Fatal("check passed a tampered vector")
			}
		})
	}
}

// Every cryptoer signs hedged: the same key and message give a new signature each time, and each
// one verifies
func TestNodeSignaturesAreHedged(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "go-ibax-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f suitesFile
	if err := decode(raw, &f); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, v := range f.Vectors {
		if seen[v.Cryptoer] {
			continue
		}
		seen[v.Cryptoer] = true
		if err := useSuite(v.Cryptoer, v.Hasher); err != nil {
			t.Fatal(err)
		}
		priv, _ := hex.DecodeString(v.PrivateKey)
		pub, err := crypto.NodePrivateToPublic(priv)
		if err != nil {
			t.Fatal(err)
		}
		first, err := crypto.NodeSign(priv, []byte(v.Message))
		if err != nil {
			t.Fatal(err)
		}
		second, err := crypto.NodeSign(priv, []byte(v.Message))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(first, second) {
			t.Errorf("%s: two signatures of one message are identical", v.Cryptoer)
		}
		for _, sig := range [][]byte{first, second} {
			if !verifies(pub, []byte(v.Message), hex.EncodeToString(sig)) {
				t.Errorf("%s: a hedged signature does not verify", v.Cryptoer)
			}
		}
	}
	if len(seen) != len(crypto.AsymAlgo_value)-1 {
		t.Errorf("checked %d cryptoers, go-ibax names %d", len(seen), len(crypto.AsymAlgo_value))
	}
}

func setAt(doc map[string]any, list string, i int, key string, value any) {
	doc[list].([]any)[i].(map[string]any)[key] = value
}

func nodeOf(doc map[string]any, i int) map[string]any {
	return doc["cases"].([]any)[i].(map[string]any)["node"].(map[string]any)
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
