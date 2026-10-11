package daemons

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFirstBlock(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "1block")
	if err := os.WriteFile(present, []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "none")
	fetched := func() ([]byte, error) { return []byte("fetched"), nil }
	unreachable := func() ([]byte, error) { return nil, errors.New("All nodes unvailabale") }

	// a node with its own 1block and boot nodes reads the file, with no boot node up
	got, err := readFirstBlock(present, true, unreachable)
	if err != nil || string(got) != "local" {
		t.Fatalf("own file with boot nodes: %q, %v", got, err)
	}
	got, err = readFirstBlock(present, false, unreachable)
	if err != nil || string(got) != "local" {
		t.Fatalf("own file: %q, %v", got, err)
	}
	// only a node without the file fetches block 1 from its boot nodes
	got, err = readFirstBlock(missing, true, fetched)
	if err != nil || string(got) != "fetched" {
		t.Fatalf("no file with boot nodes: %q, %v", got, err)
	}
	if _, err = readFirstBlock(missing, false, fetched); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no file and no boot nodes: %v", err)
	}
	// a file that exists but cannot be read is an error, not a reason to fetch
	if _, err = readFirstBlock(dir, true, fetched); err == nil {
		t.Fatal("unreadable file: no error")
	}
}
