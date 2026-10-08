/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
