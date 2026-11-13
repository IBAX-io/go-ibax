/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/network/bftnet"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"
)

// testHost is a host of the blocks 1..last that sends at most per of them a batch, and keeps the
// certificates up to certs
type testHost struct {
	last, per, certs int64
	notKept          int64 // the blocks before it are out of its window
	mu               sync.Mutex
	asked            []int64 // the first block of each request of blocks
	certAsks         []int64 // the counts of the requests of certificates
	playing          bool    // a batch is playing
	askedWhilePlay   int     // requests of blocks while a batch played
}

func (h *testHost) use(t *testing.T) {
	savedBlocks, savedCerts := fetchBlocks, fetchCertificates
	t.Cleanup(func() { fetchBlocks, fetchCertificates = savedBlocks, savedCerts })
	fetchBlocks = func(ctx context.Context, host string, from int64, reverse bool) (<-chan []byte, int64, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.asked = append(h.asked, from)
		if h.playing {
			h.askedWhilePlay++
		}
		if from < h.notKept {
			return nil, 0, tcpclient.ErrNotKept
		}
		if from > h.last {
			return nil, 0, errors.New("no blocks")
		}
		n := min(h.per, h.last-from+1)
		ch := make(chan []byte, n)
		for id := from; id < from+n; id++ {
			ch <- []byte{byte(id)}
		}
		close(ch)
		return ch, n, nil
	}
	fetchCertificates = func(host string, from uint64, count int, timeout time.Duration) ([][]byte, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		if count > bftnet.MaxCertificates {
			return nil, errors.New("too many certificates asked")
		}
		h.certAsks = append(h.certAsks, int64(count))
		var certs [][]byte
		for id := int64(from); id < int64(from)+int64(count) && id <= h.certs; id++ {
			certs = append(certs, []byte{byte(id)})
		}
		return certs, nil
	}
}

// player plays the blocks in order, and fails at the block fail
func (h *testHost) player(t *testing.T, next *int64, fail int64) func(b batch) (int64, error) {
	return func(b batch) (int64, error) {
		h.mu.Lock()
		h.playing = true
		h.mu.Unlock()
		defer func() {
			h.mu.Lock()
			h.playing = false
			h.mu.Unlock()
		}()
		// the next batch is asked for while this one plays: wait for its request, if there is
		// a next batch, rather than for a time a busy machine may not give the request
		if b.from+b.count <= h.last {
			for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
				h.mu.Lock()
				asked := len(h.asked) > 0 && h.asked[len(h.asked)-1] >= b.from+b.count
				h.mu.Unlock()
				if asked || time.Now().After(deadline) {
					break
				}
			}
		}
		var n int64
		for raw := range b.blocks {
			if b.certs != nil && n == b.count {
				break
			}
			if int64(raw[0]) != byte64(*next) {
				t.Fatalf("block %d played for %d", raw[0], *next)
			}
			if b.certs != nil && b.certs[n][0] != raw[0] {
				t.Fatalf("certificate %d for block %d", b.certs[n][0], raw[0])
			}
			if *next == fail {
				return n, errors.New("bad block")
			}
			*next++
			n++
		}
		return n, nil
	}
}

func byte64(id int64) int64 { return int64(byte(id)) }

func TestPlayBatches(t *testing.T) {
	// batches of the counts the host sends, the next asked for while one plays
	h := &testHost{last: 95, per: 20, certs: 95}
	h.use(t)
	next := int64(1)
	n, err := playBatches(context.Background(), "h", 1, 95, false, h.player(t, &next, 0))
	if err != nil || n != 95 || next != 96 {
		t.Fatalf("played %d, next %d: %v", n, next, err)
	}
	if want := []int64{1, 21, 41, 61, 81}; !equal(h.asked, want) {
		t.Fatalf("asked %v, not %v", h.asked, want)
	}
	if h.askedWhilePlay != 4 {
		t.Fatalf("%d batches asked while one played, not 4", h.askedWhilePlay)
	}

	// a block that fails stops the play, and no batch is asked after the one being played
	h = &testHost{last: 95, per: 20, certs: 95}
	h.use(t)
	next = 1
	n, err = playBatches(context.Background(), "h", 1, 95, false, h.player(t, &next, 30))
	if err == nil || n != 29 {
		t.Fatalf("played %d up to a bad block 30: %v", n, err)
	}
	if want := []int64{1, 21, 41}; !equal(h.asked, want) {
		t.Fatalf("asked %v, not %v", h.asked, want)
	}

	// the play goes on to the block to, but not after its batch
	h = &testHost{last: 95, per: 20, certs: 95}
	h.use(t)
	next = 11
	if n, err = playBatches(context.Background(), "h", 11, 40, false, h.player(t, &next, 0)); err != nil || n != 40 {
		t.Fatalf("played %d from 11 to 40: %v", n, err)
	}
}

func TestPlayBatchesCertified(t *testing.T) {
	// batches of 250 blocks take their certificates by 100, and a batch stops at the last
	// certificate
	h := &testHost{last: 600, per: 250, certs: 520}
	h.use(t)
	next := int64(1)
	n, err := playBatches(context.Background(), "h", 1, 600, true, h.player(t, &next, 0))
	if !errors.Is(err, tcpclient.ErrNotKept) || n != 520 {
		t.Fatalf("played %d of 520 certified: %v", n, err)
	}
	if want := []int64{1, 251, 501, 521}; !equal(h.asked, want) {
		t.Fatalf("asked %v, not %v", h.asked, want)
	}
	if want := []int64{100, 100, 50, 100, 100, 50, 100, 80}; !equal(h.certAsks, want) {
		t.Fatalf("certificates asked %v, not %v", h.certAsks, want)
	}

	// syncFromKeeper skips the hosts that do not keep the blocks
	h = &testHost{last: 60, per: 25, certs: 60, notKept: 20}
	h.use(t)
	host, m, err := syncFromKeeper(context.Background(), []string{"a", "b"}, "a", 10, 60)
	if !errors.Is(err, tcpclient.ErrNotKept) || host != "a" || m != 0 {
		t.Fatalf("no keeper: %s, %d, %v", host, m, err)
	}
}

func equal(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
