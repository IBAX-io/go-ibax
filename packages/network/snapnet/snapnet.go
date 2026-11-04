/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package snapnet transfers the snapshots of the state (design 4.4): a node serves the
// snapshots it keeps, and a node that joins downloads the snapshot of a height from its peers,
// each chunk checked against the manifest, the manifest against the state hash the chain
// certified.
//
// A request is RequestTypeSnapshot, then an operation and its arguments, big-endian:
//
//	opHeights:  -> u16 n, n × u64 height
//	opManifest: u64 height -> u32 n, n bytes of state.json (0: none)
//	opChunks:   u64 height, u8 n, n bytes of a table name, u32 index, u8 count
//	            -> u8 n, n × (u32 size, size bytes)
package snapnet

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/faults"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/snapshot"
)

const (
	opHeights byte = iota
	opManifest
	opChunks
)

const (
	// MaxChunks is the most chunks a request takes
	MaxChunks = 16
	// maxManifest is the longest manifest a node takes, of a state of 2^20 chunks
	maxManifest = 256 << 20
	// maxHeights is the most heights a node lists
	maxHeights = 64
	// maxFailures is the failures of a peer after which a download no more asks it
	maxFailures = 3
)

// manifests keeps the manifest of the snapshots a node served last, so a request of chunks
// parses none: the directory of a snapshot does not change once complete
type manifests struct {
	mu       sync.Mutex
	dir      string
	byHeight map[int64]*snapshot.State
}

var served manifests

func (m *manifests) get(store snapshot.Store, height int64) (*snapshot.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dir != store.Dir {
		m.dir, m.byHeight = store.Dir, map[int64]*snapshot.State{}
	}
	if st, ok := m.byHeight[height]; ok && store.Has(height) {
		return st, nil
	}
	delete(m.byHeight, height)
	data, err := store.Manifest(height)
	if err != nil || data == nil {
		return nil, err
	}
	st := &snapshot.State{}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, err
	}
	if len(m.byHeight) >= 4 {
		clear(m.byHeight)
	}
	m.byHeight[height] = st
	return st, nil
}

// Serve answers a request RequestTypeSnapshot from the snapshots of a store
func Serve(rw io.ReadWriter, store snapshot.Store) error {
	var op [1]byte
	if _, err := io.ReadFull(rw, op[:]); err != nil {
		return err
	}
	switch op[0] {
	case opHeights:
		heights, err := store.Heights()
		if err != nil {
			return err
		}
		heights = heights[max(0, len(heights)-maxHeights):]
		out := binary.BigEndian.AppendUint16(nil, uint16(len(heights)))
		for _, h := range heights {
			out = binary.BigEndian.AppendUint64(out, uint64(h))
		}
		_, err = rw.Write(out)
		return err
	case opManifest:
		height, err := readHeight(rw)
		if err != nil {
			return err
		}
		data, err := store.Manifest(height)
		if err != nil {
			return err
		}
		if _, err := rw.Write(binary.BigEndian.AppendUint32(nil, uint32(len(data)))); err != nil {
			return err
		}
		_, err = rw.Write(data)
		return err
	case opChunks:
		height, err := readHeight(rw)
		if err != nil {
			return err
		}
		var n [1]byte
		if _, err := io.ReadFull(rw, n[:]); err != nil {
			return err
		}
		name := make([]byte, n[0])
		if _, err := io.ReadFull(rw, name); err != nil {
			return err
		}
		var arg [5]byte
		if _, err := io.ReadFull(rw, arg[:]); err != nil {
			return err
		}
		index, count := int(binary.BigEndian.Uint32(arg[:4])), min(int(arg[4]), MaxChunks)
		var chunks [][]byte
		if st, err := served.get(store, height); err != nil {
			return err
		} else if st != nil {
			// a table not of the manifest, or chunks past its last, are none
			chunks, _ = store.ReadChunks(height, st, string(name), index, count)
		}
		if _, err := rw.Write([]byte{byte(len(chunks))}); err != nil {
			return err
		}
		for _, c := range chunks {
			if _, err := rw.Write(binary.BigEndian.AppendUint32(nil, uint32(len(c)))); err != nil {
				return err
			}
			if _, err := rw.Write(c); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("snapnet: operation %d", op[0])
}

func readHeight(r io.Reader) (int64, error) {
	var b [8]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(b[:])), nil
}

// request dials a peer and writes a request of an operation
func request(addr string, timeout time.Duration, req []byte) (io.ReadCloser, error) {
	conn, err := faults.Dial(addr, timeout)
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		conn.Close()
		return nil, err
	}
	if err := (&network.RequestType{Type: network.RequestTypeSnapshot}).Write(conn); err != nil {
		conn.Close()
		return nil, err
	}
	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// Heights lists the heights of the snapshots a peer keeps, ascending
func Heights(addr string, timeout time.Duration) ([]int64, error) {
	conn, err := request(addr, timeout, []byte{opHeights})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var n [2]byte
	if _, err := io.ReadFull(conn, n[:]); err != nil {
		return nil, err
	}
	count := int(binary.BigEndian.Uint16(n[:]))
	if count > maxHeights {
		return nil, fmt.Errorf("snapnet: %d heights", count)
	}
	heights := make([]int64, count)
	for i := range heights {
		if heights[i], err = readHeight(conn); err != nil {
			return nil, err
		}
	}
	return heights, nil
}

// FetchManifest fetches the manifest of the snapshot of a height from a peer, nil if it has none
func FetchManifest(addr string, height int64, timeout time.Duration) ([]byte, error) {
	conn, err := request(addr, timeout, binary.BigEndian.AppendUint64([]byte{opManifest}, uint64(height)))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var n [4]byte
	if _, err := io.ReadFull(conn, n[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(n[:])
	if size == 0 {
		return nil, nil
	}
	if size > maxManifest {
		return nil, fmt.Errorf("snapnet: a manifest of %d bytes", size)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}
	return data, nil
}

// FetchChunks fetches up to count chunks of a table of the snapshot of a height from a peer,
// from the chunk index on
func FetchChunks(addr string, height int64, table string, index, count int, timeout time.Duration) ([][]byte, error) {
	if len(table) > 255 {
		return nil, fmt.Errorf("snapnet: a table named %q", table)
	}
	count = min(max(count, 0), MaxChunks)
	req := binary.BigEndian.AppendUint64([]byte{opChunks}, uint64(height))
	req = append(append(req, byte(len(table))), table...)
	req = append(binary.BigEndian.AppendUint32(req, uint32(index)), byte(count))
	conn, err := request(addr, timeout, req)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var n [1]byte
	if _, err := io.ReadFull(conn, n[:]); err != nil {
		return nil, err
	}
	if int(n[0]) > count {
		return nil, fmt.Errorf("snapnet: %d chunks for %d asked", n[0], count)
	}
	chunks := make([][]byte, n[0])
	for i := range chunks {
		var size [4]byte
		if _, err := io.ReadFull(conn, size[:]); err != nil {
			return nil, err
		}
		l := binary.BigEndian.Uint32(size[:])
		if l > uint32(snapshot.ChunkBytes()) {
			return nil, fmt.Errorf("snapnet: a chunk of %d bytes", l)
		}
		chunks[i] = make([]byte, l)
		if _, err := io.ReadFull(conn, chunks[i]); err != nil {
			return nil, err
		}
	}
	return chunks, nil
}

// Download is a download of the snapshot of a height from peers
type Download struct {
	Peers []string
	// Height and StateHash are the snapshot height and the state hash a certified block carries
	Height    int64
	StateHash []byte
	// Store is the store the snapshot is written to, as the node would write its own
	Store snapshot.Store
	// Timeout bounds each request; Workers is the requests at once
	Timeout time.Duration
	Workers int
}

// span is chunks of a table a request fetches
type span struct {
	table *snapshot.Table
	file  *os.File
	index int
	count int
}

// ErrNoPeer is the error of a download that no peer could serve
var ErrNoPeer = errors.New("snapnet: no peer serves the snapshot")

// Run downloads the snapshot: the manifest of the first peer whose manifest is of the state
// hash, then the chunks it lacks from all peers, each checked against the manifest, a peer that
// fails maxFailures times no more asked. What a download that stopped wrote is kept, its chunks
// checked again, so a download resumes. Once complete the snapshot is in the store as the node
// would have written it.
func (d *Download) Run(ctx context.Context) (*snapshot.State, error) {
	if len(d.Peers) == 0 {
		return nil, ErrNoPeer
	}
	var state *snapshot.State
	var errs []error
	for _, p := range d.Peers {
		data, err := FetchManifest(p, d.Height, d.Timeout)
		if err == nil && data == nil {
			err = fmt.Errorf("no snapshot %d", d.Height)
		}
		if err == nil {
			state, err = snapshot.ParseManifest(data, d.StateHash)
		}
		if err == nil {
			break
		}
		errs = append(errs, fmt.Errorf("%s: %w", p, err))
	}
	if state == nil {
		return nil, errors.Join(append([]error{ErrNoPeer}, errs...)...)
	}
	files, err := d.Store.Resume(d.Height)
	if err != nil {
		return nil, err
	}
	var opened []*os.File
	defer func() {
		for _, f := range opened {
			f.Close()
		}
	}()
	var spans []span
	for i := range state.Tables {
		t := &state.Tables[i]
		f, err := files.Table(t.Name)
		if err != nil {
			return nil, err
		}
		opened = append(opened, f)
		if err := f.Truncate(t.Bytes); err != nil {
			return nil, err
		}
		for _, k := range missing(f, t) {
			if n := len(spans); n > 0 && spans[n-1].table == t && spans[n-1].index+spans[n-1].count == k &&
				spans[n-1].count < MaxChunks {
				spans[n-1].count++
				continue
			}
			spans = append(spans, span{table: t, file: f, index: k, count: 1})
		}
	}
	if err := d.fetch(ctx, spans); err != nil {
		return nil, err
	}
	for i, t := range state.Tables {
		f := opened[i]
		if err := f.Sync(); err != nil {
			return nil, err
		}
		if err := snapshot.CheckRows(t, io.NewSectionReader(f, 0, t.Bytes)); err != nil {
			return nil, err
		}
	}
	if err := files.Commit(state); err != nil {
		return nil, err
	}
	return state, nil
}

// missing is the chunks of a table its file lacks or has other than of the manifest
func missing(f *os.File, t *snapshot.Table) []int {
	var out []int
	buf := make([]byte, snapshot.ChunkBytes())
	for k := range t.Chunks {
		off := int64(k) * int64(len(buf))
		c := buf[:min(int64(len(buf)), t.Bytes-off)]
		if _, err := f.ReadAt(c, off); err != nil || snapshot.CheckChunk(*t, k, c) != nil {
			out = append(out, k)
		}
	}
	return out
}

// fetch fetches the spans from the peers, Workers at once, each span asked of the peers in turn
// from one of its own
func (d *Download) fetch(ctx context.Context, spans []span) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		mu       sync.Mutex
		failures = make([]int, len(d.Peers))
		first    error
	)
	fail := func(err error) {
		mu.Lock()
		if first == nil {
			first = err
		}
		mu.Unlock()
		cancel()
	}
	work := make(chan int)
	var wg sync.WaitGroup
	for range max(1, d.Workers) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				if err := d.fetchSpan(ctx, spans[i], i, &mu, failures); err != nil {
					fail(err)
				}
			}
		}()
	}
	for i := range spans {
		select {
		case work <- i:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(work)
	wg.Wait()
	if first != nil {
		return first
	}
	return ctx.Err()
}

func (d *Download) fetchSpan(ctx context.Context, s span, n int, mu *sync.Mutex, failures []int) error {
	var errs []error
	for k := range d.Peers {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p := (n + k) % len(d.Peers)
		mu.Lock()
		banned := failures[p] >= maxFailures
		mu.Unlock()
		if banned {
			continue
		}
		chunks, err := d.fetchFrom(d.Peers[p], s)
		if err == nil {
			for i, c := range chunks {
				if _, err := s.file.WriteAt(c, int64(s.index+i)*int64(snapshot.ChunkBytes())); err != nil {
					return err
				}
			}
			return nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", d.Peers[p], err))
		mu.Lock()
		failures[p]++
		mu.Unlock()
	}
	return errors.Join(append([]error{fmt.Errorf("%w: chunks %d+%d of table %q", ErrNoPeer, s.index, s.count, s.table.Name)},
		errs...)...)
}

// fetchFrom fetches the chunks of a span from a peer and checks them
func (d *Download) fetchFrom(peer string, s span) ([][]byte, error) {
	chunks, err := FetchChunks(peer, d.Height, s.table.Name, s.index, s.count, d.Timeout)
	if err != nil {
		return nil, err
	}
	if len(chunks) != s.count {
		return nil, fmt.Errorf("%d chunks of %d", len(chunks), s.count)
	}
	for i, c := range chunks {
		if err := snapshot.CheckChunk(*s.table, s.index+i, c); err != nil {
			return nil, err
		}
	}
	return chunks, nil
}
