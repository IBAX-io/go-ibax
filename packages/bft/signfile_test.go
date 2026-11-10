/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The file of the sign state has the last state written, whole: none before the first, a write
// replaces it, a temporary file a crash left does not count, and a machine that restores the
// state read back keeps its lock
func TestSignStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), SignStateFile)
	if s, err := ReadSignState(path, 1<<20); s != nil || err != nil {
		t.Fatalf("no file: %v %v", s, err)
	}
	c := newCorner(t)
	c.lockA()
	first := c.m.TakeSignState()
	if err := WriteSignState(path, first); err != nil {
		t.Fatal(err)
	}
	second := *first
	second.Round = 3
	if err := WriteSignState(path, &second); err != nil {
		t.Fatal(err)
	}
	// a crash in a write leaves its temporary file
	if err := os.WriteFile(path+".tmp", []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := ReadSignState(path, 1<<20)
	if err != nil || !bytes.Equal(s.Marshal(), second.Marshal()) {
		t.Fatalf("read back: %v %v", s, err)
	}
	if err := WriteSignState(path, first); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("the temporary file after a write: %v", err)
	}
	s, err = ReadSignState(path, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	c.restart(s)
	if r, h := c.m.Locked(); r != 0 || h != hashA || c.m.Round() != 1 {
		t.Fatalf("restored at round %d, locked %d %x", c.m.Round(), r, h[:2])
	}
	// a file that is no sign state is an error, not an empty state
	if err := os.WriteFile(path, []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSignState(path, 1<<20); err == nil {
		t.Fatal("a damaged file read")
	}
}
