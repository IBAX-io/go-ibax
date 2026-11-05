/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package snapshot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

// The files of a snapshot of a height S are in the directory S of a store: the encoding of each
// table, <table>.copy, and the manifest, state.json, the State of the snapshot in JSON. A snapshot
// is written in S.tmp and renamed to S once complete, so a directory S is a complete snapshot.
const (
	// DirName is the directory of the store of a node in its data directory
	DirName      = "snapshots"
	manifestName = "state.json"
	tableSuffix  = ".copy"
	tmpSuffix    = ".tmp"
)

// ChunkBytes is the length of the chunks of the encoding of a table, the unit of the transfer of
// a snapshot
func ChunkBytes() int { return chunkBytes }

// Store is the directory of the snapshots of a node
type Store struct {
	Dir string
}

func (s Store) path(height int64, name string) string {
	return filepath.Join(s.Dir, strconv.FormatInt(height, 10), name)
}

// Heights is the heights of the complete snapshots of the store, ascending
func (s Store) Heights() ([]int64, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var heights []int64
	for _, e := range entries {
		if h, err := strconv.ParseInt(e.Name(), 10, 64); err == nil && h > 0 && e.IsDir() {
			heights = append(heights, h)
		}
	}
	slices.Sort(heights)
	return heights, nil
}

// Has reports whether the store has the complete snapshot of a height
func (s Store) Has(height int64) bool {
	_, err := os.Stat(s.path(height, manifestName))
	return err == nil
}

// Manifest reads the manifest of the snapshot of a height, nil if the store has none
func (s Store) Manifest(height int64) ([]byte, error) {
	data, err := os.ReadFile(s.path(height, manifestName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// ReadChunks reads up to n chunks of the encoding of a table of the snapshot of a height from the
// chunk index on; the table is one of the manifest, so no name reaches outside the directory
func (s Store) ReadChunks(height int64, state *State, table string, index, n int) ([][]byte, error) {
	i := slices.IndexFunc(state.Tables, func(t Table) bool { return t.Name == table })
	if i < 0 {
		return nil, fmt.Errorf("snapshot %d has no table %q", height, table)
	}
	t := state.Tables[i]
	if index < 0 || index >= len(t.Chunks) {
		return nil, nil
	}
	f, err := os.Open(s.path(height, t.Name+tableSuffix))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var chunks [][]byte
	for k := index; k < min(len(t.Chunks), index+n); k++ {
		size := min(int64(chunkBytes), t.Bytes-int64(k)*int64(chunkBytes))
		c := make([]byte, size)
		if _, err := f.ReadAt(c, int64(k)*int64(chunkBytes)); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// Create begins the files of the snapshot of a height, in its temporary directory, emptied
func (s Store) Create(height int64) (*Files, error) {
	if err := os.RemoveAll(filepath.Join(s.Dir, strconv.FormatInt(height, 10)+tmpSuffix)); err != nil {
		return nil, err
	}
	return s.Resume(height)
}

// Resume continues the files of the snapshot of a height in its temporary directory, what was
// written in it kept
func (s Store) Resume(height int64) (*Files, error) {
	tmp := filepath.Join(s.Dir, strconv.FormatInt(height, 10)+tmpSuffix)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return nil, err
	}
	return &Files{store: s, height: height, tmp: tmp}, nil
}

// Files is a snapshot being written
type Files struct {
	store  Store
	height int64
	tmp    string
}

// Sink is the sink of Hash that writes the tables
func (f *Files) Sink() Sink {
	return func(table string) (io.WriteCloser, error) {
		return os.Create(filepath.Join(f.tmp, table+tableSuffix))
	}
}

// Table opens the file of a table to write at offsets, as a download does
func (f *Files) Table(table string) (*os.File, error) {
	return os.OpenFile(filepath.Join(f.tmp, table+tableSuffix), os.O_RDWR|os.O_CREATE, 0o644)
}

// Commit writes the manifest and makes the snapshot complete
func (f *Files) Commit(state *State) error {
	manifest, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(f.tmp, manifestName), manifest, 0o644); err != nil {
		return err
	}
	final := filepath.Join(f.store.Dir, strconv.FormatInt(f.height, 10))
	if err := os.RemoveAll(final); err != nil {
		return err
	}
	return os.Rename(f.tmp, final)
}

// Abort removes what was written
func (f *Files) Abort() { os.RemoveAll(f.tmp) }

// Prune removes the snapshots of the store but the latest keep, and the temporary directories
// but those of the heights busy, which are being written
func (s Store) Prune(keep int, busy func(height int64) bool) error {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if base, ok := strings.CutSuffix(e.Name(), tmpSuffix); ok {
			if h, err := strconv.ParseInt(base, 10, 64); err == nil && !busy(h) {
				os.RemoveAll(filepath.Join(s.Dir, e.Name()))
			}
		}
	}
	heights, err := s.Heights()
	if err != nil {
		return err
	}
	for _, h := range heights[:max(0, len(heights)-keep)] {
		if err := os.RemoveAll(filepath.Join(s.Dir, strconv.FormatInt(h, 10))); err != nil {
			return err
		}
	}
	return nil
}

// ParseManifest parses a manifest and checks it is of the state hash: its tables in the order of
// their names, each with as many chunks as its bytes take and its hash that of its description,
// rows, bytes and chunks, and the state hash that of its tables. What it checks of the tables
// alone is what the chunks verify against: each chunk against its hash, the rows by the
// newlines of the encoding (CheckRows).
func ParseManifest(data, hash []byte) (*State, error) {
	state := &State{}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("parsing the manifest: %w", err)
	}
	for i, t := range state.Tables {
		if t.Name == "" || strings.ContainsAny(t.Name, `/\`) || t.Name == "." || t.Name == ".." {
			return nil, fmt.Errorf("the manifest: a table named %q", t.Name)
		}
		if i > 0 && state.Tables[i-1].Name >= t.Name {
			return nil, fmt.Errorf("the manifest: table %q out of order", t.Name)
		}
		if t.Bytes < 0 || t.Rows < 0 || int64(len(t.Chunks)) != (t.Bytes+int64(chunkBytes)-1)/int64(chunkBytes) {
			return nil, fmt.Errorf("the manifest: table %q of %d bytes in %d chunks", t.Name, t.Bytes, len(t.Chunks))
		}
		if !bytes.Equal(TableHash(t), t.Hash) {
			return nil, fmt.Errorf("the manifest: the hash of table %q", t.Name)
		}
	}
	if !bytes.Equal(StateHash(state.Tables), hash) || !bytes.Equal(state.Hash, hash) {
		return nil, fmt.Errorf("the manifest is not of the state hash %x", hash)
	}
	return state, nil
}

// CheckChunk checks a chunk of a table of a manifest
func CheckChunk(t Table, index int, chunk []byte) error {
	if index < 0 || index >= len(t.Chunks) {
		return fmt.Errorf("table %q has no chunk %d", t.Name, index)
	}
	if size := min(int64(chunkBytes), t.Bytes-int64(index)*int64(chunkBytes)); int64(len(chunk)) != size {
		return fmt.Errorf("chunk %d of table %q: %d bytes, not %d", index, t.Name, len(chunk), size)
	}
	if !bytes.Equal(crypto.Hash(chunk), t.Chunks[index]) {
		return fmt.Errorf("chunk %d of table %q: not of its hash", index, t.Name)
	}
	return nil
}

// CheckRows checks the encoding of a table, its chunks all checked, has the rows of the manifest
func CheckRows(t Table, r io.Reader) error {
	buf := make([]byte, chunkBytes)
	var rows int64
	for {
		n, err := r.Read(buf)
		rows += int64(bytes.Count(buf[:n], []byte{'\n'}))
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if rows != t.Rows {
		return fmt.Errorf("table %q: %d rows, not %d", t.Name, rows, t.Rows)
	}
	return nil
}
