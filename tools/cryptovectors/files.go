/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// vectorFile recomputes the node side of one vector file. update returns the file with every
// node-side field recomputed, and the problems that no regeneration can fix (a client signature the
// node refuses).
type vectorFile struct {
	name   string
	update func(raw []byte) (any, []string, error)
}

var vectorFiles = []vectorFile{
	{"go-ibax-vectors.json", updateSuites},
	{"go-ibax-addresses.json", updateAddresses},
	{"go-ibax-transfers.json", updateTransfers},
	{"go-ibax-contract-params.json", updateContractParams},
}

// run regenerates (write) or checks every vector file in dir and returns what differs.
func run(dir string, write bool) ([]string, error) {
	var diffs []string
	for _, f := range vectorFiles {
		path := filepath.Join(dir, f.name)
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		updated, problems, err := f.update(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
		for _, p := range problems {
			diffs = append(diffs, fmt.Sprintf("%s: %s", f.name, p))
		}
		out, err := encode(updated)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
		if bytes.Equal(out, raw) {
			continue
		}
		if write {
			if err := os.WriteFile(path, out, 0o644); err != nil {
				return nil, err
			}
			continue
		}
		diffs = append(diffs, fmt.Sprintf("%s: out of date, run `go run ./tools/cryptovectors gen`", f.name))
	}
	return diffs, nil
}

func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decode(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	return dec.Decode(v)
}
