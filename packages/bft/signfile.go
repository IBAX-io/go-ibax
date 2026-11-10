/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// SignStateFile is the file of the sign state in the data directory of the node. The sign state
// is of the node key, not of the chain: it stays with the key when the database is restored or
// joined from a snapshot, and the node rewrites it each time it signs, which a table would keep
// as dead rows of the size of the proposals.
const SignStateFile = "bft_sign_state"

// WriteSignState writes the sign state to path whole or not at all: to a temporary file in its
// directory, synced, renamed over path, and the directory synced, so that after a crash path has
// the state before or this one
func WriteSignState(path string, s *SignState) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(s.Marshal()); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	err = dir.Sync()
	if cerr := dir.Close(); err == nil {
		err = cerr
	}
	return err
}

// ReadSignState reads the sign state of path, whose blocks are at most maxBlock bytes; nil if the
// node has none
func ReadSignState(path string, maxBlock int64) (*SignState, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return UnmarshalSignState(b, maxBlock)
}
