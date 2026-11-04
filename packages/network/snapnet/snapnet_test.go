/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package snapnet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/snapshot"
	"github.com/stretchr/testify/require"
)

const height = 20000

// tableOf is a table of the state whose encoding is data
func tableOf(name string, data []byte) snapshot.Table {
	t := snapshot.Table{Name: name, Desc: "id bigint", Rows: int64(bytes.Count(data, []byte{'\n'})), Bytes: int64(len(data))}
	for off := 0; off < len(data); off += snapshot.ChunkBytes() {
		t.Chunks = append(t.Chunks, crypto.Hash(data[off:min(len(data), off+snapshot.ChunkBytes())]))
	}
	t.Hash = snapshot.TableHash(t)
	return t
}

// rows is n bytes of the encoding of rows, a row of each 100 bytes
func rows(seed byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + (seed+byte(i))%26
		if i%100 == 99 {
			b[i] = '\n'
		}
	}
	return b
}

// writeSnapshot writes the snapshot of the tables to a store as a node does
func writeSnapshot(t *testing.T, store snapshot.Store, data map[string][]byte) *snapshot.State {
	t.Helper()
	files, err := store.Create(height)
	require.NoError(t, err)
	state := &snapshot.State{}
	for _, name := range []string{"1_history", "1_keys", "info_block"} {
		if d, ok := data[name]; ok {
			w, err := files.Sink()(name)
			require.NoError(t, err)
			_, err = w.Write(d)
			require.NoError(t, err)
			require.NoError(t, w.Close())
			state.Tables = append(state.Tables, tableOf(name, d))
		}
	}
	state.Hash = snapshot.StateHash(state.Tables)
	require.NoError(t, files.Commit(state))
	return state
}

func testData() map[string][]byte {
	crypto.InitHashAlgo("SHA256")
	return map[string][]byte{
		"1_history":  rows(1, 2*snapshot.ChunkBytes()+snapshot.ChunkBytes()/2),
		"1_keys":     rows(2, snapshot.ChunkBytes()),
		"info_block": rows(3, 300),
	}
}

// peer serves a store at an address of its own and counts the chunks it served
type peer struct {
	addr   string
	chunks atomic.Int64
}

func serve(t *testing.T, store snapshot.Store) *peer {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	p := &peer{addr: l.Addr().String()}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				rt := &network.RequestType{}
				if rt.Read(c) != nil || rt.Type != network.RequestTypeSnapshot {
					return
				}
				Serve(counter{c, p}, store)
			}()
		}
	}()
	return p
}

// counter counts the chunks a peer writes: a chunk is of 300 bytes at least, as no manifest
// starts with a letter
type counter struct {
	net.Conn
	p *peer
}

func (c counter) Write(b []byte) (int, error) {
	if len(b) >= 300 && b[0] != '{' {
		c.p.chunks.Add(1)
	}
	return c.Conn.Write(b)
}

func download(peers []*peer, hash []byte, dir string) *Download {
	d := &Download{Height: height, StateHash: hash, Store: snapshot.Store{Dir: dir}, Timeout: 5 * time.Second, Workers: 3}
	for _, p := range peers {
		d.Peers = append(d.Peers, p.addr)
	}
	return d
}

// requireSame requires the snapshot of a store to be that of another, file by file
func requireSame(t *testing.T, want, got string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(want, fmt.Sprint(height)))
	require.NoError(t, err)
	gotEntries, err := os.ReadDir(filepath.Join(got, fmt.Sprint(height)))
	require.NoError(t, err)
	require.Len(t, gotEntries, len(entries))
	for _, e := range entries {
		a, err := os.ReadFile(filepath.Join(want, fmt.Sprint(height), e.Name()))
		require.NoError(t, err)
		b, err := os.ReadFile(filepath.Join(got, fmt.Sprint(height), e.Name()))
		require.NoError(t, err)
		require.True(t, bytes.Equal(a, b), e.Name())
	}
	_, err = os.Stat(filepath.Join(got, fmt.Sprint(height)+".tmp"))
	require.True(t, os.IsNotExist(err))
}

// A node downloads the snapshot from peers and has it as the peers wrote it
func TestDownload(t *testing.T) {
	data := testData()
	a, b := t.TempDir(), t.TempDir()
	state := writeSnapshot(t, snapshot.Store{Dir: a}, data)
	writeSnapshot(t, snapshot.Store{Dir: b}, data)
	pa, pb := serve(t, snapshot.Store{Dir: a}), serve(t, snapshot.Store{Dir: b})

	heights, err := Heights(pa.addr, time.Second)
	require.NoError(t, err)
	require.Equal(t, []int64{height}, heights)
	none, err := FetchManifest(pa.addr, height+1, time.Second)
	require.NoError(t, err)
	require.Nil(t, none)

	dir := t.TempDir()
	got, err := download([]*peer{pa, pb}, state.Hash, dir).Run(t.Context())
	require.NoError(t, err)
	require.Equal(t, state, got)
	requireSame(t, a, dir)
	// the chunks came from both peers, 3 + 1 + 1 of them
	require.Equal(t, int64(5), pa.chunks.Load()+pb.chunks.Load())
	require.NotZero(t, pa.chunks.Load())
	require.NotZero(t, pb.chunks.Load())
}

// A chunk of a peer other than of the manifest is asked of another peer; with no other peer
// the download fails and completes nothing
func TestDownloadTamperedChunk(t *testing.T) {
	data := testData()
	good, bad := t.TempDir(), t.TempDir()
	state := writeSnapshot(t, snapshot.Store{Dir: good}, data)
	writeSnapshot(t, snapshot.Store{Dir: bad}, data)
	f := filepath.Join(bad, fmt.Sprint(height), "1_history.copy")
	b, err := os.ReadFile(f)
	require.NoError(t, err)
	b[snapshot.ChunkBytes()+7] ^= 1
	require.NoError(t, os.WriteFile(f, b, 0o644))
	pg, pb := serve(t, snapshot.Store{Dir: good}), serve(t, snapshot.Store{Dir: bad})

	dir := t.TempDir()
	_, err = download([]*peer{pb, pg}, state.Hash, dir).Run(t.Context())
	require.NoError(t, err)
	requireSame(t, good, dir)

	dir = t.TempDir()
	_, err = download([]*peer{pb}, state.Hash, dir).Run(t.Context())
	require.ErrorIs(t, err, ErrNoPeer)
	require.ErrorContains(t, err, "not of its hash")
	_, err = os.Stat(filepath.Join(dir, fmt.Sprint(height)))
	require.True(t, os.IsNotExist(err))
}

// A manifest not of the state hash the chain certified is refused, whatever it changed
func TestDownloadTamperedManifest(t *testing.T) {
	data := testData()
	good := t.TempDir()
	state := writeSnapshot(t, snapshot.Store{Dir: good}, data)
	manifest, err := snapshot.Store{Dir: good}.Manifest(height)
	require.NoError(t, err)
	_, err = snapshot.ParseManifest(manifest, state.Hash)
	require.NoError(t, err)

	tamper := func(change func(s *snapshot.State)) []byte {
		s := &snapshot.State{}
		require.NoError(t, json.Unmarshal(manifest, s))
		change(s)
		b, err := json.Marshal(s)
		require.NoError(t, err)
		return b
	}
	for name, c := range map[string]struct {
		change func(s *snapshot.State)
		err    string
	}{
		"a chunk hash":         {func(s *snapshot.State) { s.Tables[0].Chunks[1][0] ^= 1 }, "the hash of table"},
		"the rows":             {func(s *snapshot.State) { s.Tables[0].Rows++ }, "the hash of table"},
		"a chunk dropped":      {func(s *snapshot.State) { s.Tables[0].Chunks = s.Tables[0].Chunks[1:] }, "in 2 chunks"},
		"a table dropped":      {func(s *snapshot.State) { s.Tables = s.Tables[1:] }, "not of the state hash"},
		"the tables reordered": {func(s *snapshot.State) { s.Tables[0], s.Tables[1] = s.Tables[1], s.Tables[0] }, "out of order"},
		"a table renamed out":  {func(s *snapshot.State) { s.Tables[2].Name = "../../x" }, "a table named"},
		"the state hash":       {func(s *snapshot.State) { s.Hash = s.Tables[0].Hash }, "not of the state hash"},
		"a table rehashed": {func(s *snapshot.State) {
			s.Tables[0].Rows++
			s.Tables[0].Hash = snapshot.TableHash(s.Tables[0])
		}, "not of the state hash"},
	} {
		_, err := snapshot.ParseManifest(tamper(c.change), state.Hash)
		require.ErrorContains(t, err, c.err, name)
	}

	// a peer whose manifest is of another state is passed over
	bad := t.TempDir()
	other := map[string][]byte{"1_history": data["1_history"], "1_keys": rows(9, 500), "info_block": data["info_block"]}
	writeSnapshot(t, snapshot.Store{Dir: bad}, other)
	pb, pg := serve(t, snapshot.Store{Dir: bad}), serve(t, snapshot.Store{Dir: good})
	dir := t.TempDir()
	_, err = download([]*peer{pb, pg}, state.Hash, dir).Run(t.Context())
	require.NoError(t, err)
	requireSame(t, good, dir)
	_, err = download([]*peer{pb}, state.Hash, t.TempDir()).Run(t.Context())
	require.ErrorIs(t, err, ErrNoPeer)
	require.ErrorContains(t, err, "not of the state hash")
}

// The rows of a table are checked against the newlines of its encoding: a manifest of the hash
// that counts others is of a state that no encoding has
func TestDownloadRows(t *testing.T) {
	data := testData()
	src := t.TempDir()
	state := writeSnapshot(t, snapshot.Store{Dir: src}, data)
	state.Tables[2].Rows++
	state.Tables[2].Hash = snapshot.TableHash(state.Tables[2])
	state.Hash = snapshot.StateHash(state.Tables)
	m, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(src, fmt.Sprint(height), "state.json"), m, 0o644))
	p := serve(t, snapshot.Store{Dir: src})
	_, err = download([]*peer{p}, state.Hash, t.TempDir()).Run(t.Context())
	require.ErrorContains(t, err, `table "info_block": 3 rows, not 4`)
}

// A download that stopped resumes: it asks only the chunks its files lack or have wrong
func TestDownloadResume(t *testing.T) {
	data := testData()
	src := t.TempDir()
	state := writeSnapshot(t, snapshot.Store{Dir: src}, data)
	dir := t.TempDir()
	tmp := filepath.Join(dir, fmt.Sprint(height)+".tmp")
	require.NoError(t, os.MkdirAll(tmp, 0o755))
	// the first chunk of 1_history right, the second wrong, the third missing; 1_keys complete
	// but too long; info_block missing
	h := bytes.Clone(data["1_history"][:2*snapshot.ChunkBytes()])
	h[snapshot.ChunkBytes()] ^= 1
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "1_history.copy"), h, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "1_keys.copy"), append(bytes.Clone(data["1_keys"]), "extra"...), 0o644))
	p := serve(t, snapshot.Store{Dir: src})
	_, err := download([]*peer{p}, state.Hash, dir).Run(t.Context())
	require.NoError(t, err)
	requireSame(t, src, dir)
	require.Equal(t, int64(3), p.chunks.Load())
}

// A peer serves the files of the tables of its manifests only
func TestServePaths(t *testing.T) {
	data := testData()
	src := t.TempDir()
	writeSnapshot(t, snapshot.Store{Dir: src}, data)
	require.NoError(t, os.WriteFile(filepath.Join(src, "secret.copy"), []byte("key\n"), 0o644))
	p := serve(t, snapshot.Store{Dir: src})
	for _, name := range []string{"../secret", "1_none", "", strings.Repeat("x", 255)} {
		chunks, err := FetchChunks(p.addr, height, name, 0, 1, time.Second)
		require.NoError(t, err, name)
		require.Empty(t, chunks, name)
	}
	chunks, err := FetchChunks(p.addr, height, "1_history", 2, MaxChunks, time.Second)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.Equal(t, data["1_history"][2*snapshot.ChunkBytes():], chunks[0])
	chunks, err = FetchChunks(p.addr, height, "1_history", 3, 1, time.Second)
	require.NoError(t, err)
	require.Empty(t, chunks)
	chunks, err = FetchChunks(p.addr, height-1, "1_history", 0, 1, time.Second)
	require.NoError(t, err)
	require.Empty(t, chunks)
}
