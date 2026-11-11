/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"os"
	"path/filepath"
	"testing"
)

// On a file system that takes O_DIRECT the slots are written directly, in whole blocks, and read
// back as with fdatasync: the padding of the last block leaves the other slot whole
func TestSignStateFileDirect(t *testing.T) {
	path := filepath.Join(t.TempDir(), SignStateFile)
	f := reopen(t, path, nil)
	c := newCorner(t)
	c.lockA()
	first := c.m.TakeSignState()
	if err := f.Write(first); err != nil {
		t.Fatal(err)
	}
	if f.w == nil {
		f.Close()
		t.Skip("the file system of the temporary directory does not take O_DIRECT (TMPDIR on a disk tests it)")
	}
	second := *first
	second.Round = 3
	if err := f.Write(&second); err != nil {
		t.Fatal(err)
	}
	f.Close()
	f = reopen(t, path, &second)
	if f.w == nil {
		t.Fatal("reopened without the direct writes")
	}
	third := second
	third.Round = 4
	third.Votes = append(third.Votes, third.Votes[0])
	if err := f.Write(&third); err != nil {
		t.Fatal(err)
	}
	f.Close()
	f = reopen(t, path, &third)
	f.Close()

	// the slot of the third (sequence 3, slot 1) damaged: the second, in the other slot, stays
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b[minSlot+slotHeader+10] ^= 0xff
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	reopen(t, path, &second).Close()
}
