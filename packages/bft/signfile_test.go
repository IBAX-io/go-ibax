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

// reopen opens the file of path as a restarted node does and checks it reads want
func reopen(t *testing.T, path string, want *SignState) *SignFile {
	t.Helper()
	f, s, err := OpenSignFile(path, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if want == nil && s != nil || want != nil && (s == nil || !bytes.Equal(s.Marshal(), want.Marshal())) {
		t.Fatalf("read back %+v, want %+v", s, want)
	}
	return f
}

// The file of the sign state has the last state written, whole: none before the first, a write
// replaces it in the other slot, a write a crash cut leaves the state before, a state larger than
// the slots makes the file anew, and a machine that restores the state read back keeps its lock
func TestSignStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), SignStateFile)
	f := reopen(t, path, nil)
	c := newCorner(t)
	c.lockA()
	first := c.m.TakeSignState()
	if err := f.Write(first); err != nil {
		t.Fatal(err)
	}
	second := *first
	second.Round = 3
	if err := f.Write(&second); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() != 2*minSlot {
		t.Fatalf("the file: %v %v", info, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("the temporary file after a write: %v", err)
	}
	f.Close()
	f = reopen(t, path, &second)

	// a crash in the write of the third state damages its slot, the one of the first (the state of
	// sequence n is in slot n mod 2): the second stays
	third := second
	third.Round = 4
	if err := f.Write(&third); err != nil {
		t.Fatal(err)
	}
	f.Close()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b[minSlot+slotHeader+10] ^= 0xff
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	f = reopen(t, path, &second)
	// the next write goes to the damaged slot, not over the second
	if err := f.Write(&third); err != nil {
		t.Fatal(err)
	}
	f.Close()
	f = reopen(t, path, &third)

	// a state that outgrows the slots
	big := third
	locked := *third.Locked
	locked.Block = bytes.Repeat([]byte{7}, 3*minSlot)
	big.Locked, big.Round = &locked, 5
	if err := f.Write(&big); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() != 16*minSlot {
		t.Fatalf("the file after a large state: %v %v", info, err)
	}
	if err := f.Write(&third); err != nil {
		t.Fatal(err)
	}
	f.Close()
	f = reopen(t, path, &third)
	f.Close()

	_, s, err := OpenSignFile(path, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	c.restart(s)
	if r, h := c.m.Locked(); r != 0 || h != hashA || c.m.Round() != third.Round+1 {
		t.Fatalf("restored at round %d, locked %d %x", c.m.Round(), r, h[:2])
	}

	// a file that is no sign state, and one whose slots are both damaged, are errors, not an
	// empty state
	if err := os.WriteFile(path, []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenSignFile(path, 1<<20); err == nil {
		t.Fatal("a file of 3 bytes read")
	}
	if err := os.WriteFile(path, make([]byte, 2*minSlot), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenSignFile(path, 1<<20); err == nil {
		t.Fatal("a file of two empty slots read")
	}
}
