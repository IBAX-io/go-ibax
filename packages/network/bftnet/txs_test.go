/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bftnet

import (
	"bytes"
	"encoding/binary"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/service/bftmon"
)

// A frame of transactions decodes to its bodies, and one of more bodies than max_tx_block, of an
// empty body, of a body past its end or of bytes after its last body is refused
func TestUnmarshalTxs(t *testing.T) {
	bodies := [][]byte{[]byte("a"), []byte("bc"), bytes.Repeat([]byte{7}, 300)}
	got, err := UnmarshalTxs(MarshalTxs(bodies), 3)
	if err != nil || len(got) != 3 {
		t.Fatalf("decoded %d bodies: %v", len(got), err)
	}
	for i := range bodies {
		if !bytes.Equal(got[i], bodies[i]) {
			t.Fatalf("body %d is %q", i, got[i])
		}
	}
	if got, err := UnmarshalTxs(MarshalTxs(nil), 3); err != nil || len(got) != 0 {
		t.Fatalf("no bodies: %d, %v", len(got), err)
	}
	empty := MarshalTxs([][]byte{[]byte("a"), {}})
	past := MarshalTxs([][]byte{[]byte("abc")})
	past = past[:len(past)-1]
	huge := binary.BigEndian.AppendUint32(nil, 1<<31)
	for name, p := range map[string][]byte{
		"more than max_tx_block": MarshalTxs(bodies),
		"an empty body":          empty,
		"a body past its end":    past,
		"bytes after the last":   append(MarshalTxs([][]byte{[]byte("a")}), 0),
		"a count past the end":   huge,
		"no count":               {0, 0},
	} {
		max := 3
		if name == "more than max_tx_block" {
			max = 2
		}
		if _, err := UnmarshalTxs(p, max); err == nil {
			t.Errorf("%s is taken", name)
		}
	}
}

// The transactions of a node reach its peers in frames of at most max_block_size and
// max_tx_block bodies, a body too long for a frame left out; a peer takes them and relays none
func TestSendTxs(t *testing.T) {
	ha, hb, hc := newTestHandler(), newTestHandler(), newTestHandler()
	a, b, c := NewService(DefaultConfig, ha), NewService(DefaultConfig, hb), NewService(DefaultConfig, hc)
	addrB, _ := testServer(t, b)
	addrC, _ := testServer(t, c)
	a.SetPeers([]string{addrB})
	b.SetPeers([]string{addrC})
	defer a.Close()
	defer b.Close()

	// MaxBlockSize 1000 and MaxBlockTxs 10: 25 bodies of 10 bytes and 3 of 400 go in frames of
	// 10, 10, 5 + 2 (4 + 5×14 + 2×404 = 882 bytes) and 1; one of 993 bytes, past 1000 with its
	// count and length, and an empty one are left out
	var bodies [][]byte
	for i := range 25 {
		bodies = append(bodies, []byte(strings.Repeat(string(rune('a'+i)), 10)))
	}
	for i := range 3 {
		bodies = append(bodies, bytes.Repeat([]byte{byte(i)}, 400))
	}
	bodies = append(bodies, make([]byte, 993), nil)
	a.SendTxs(bodies)
	waitFor(t, "the frames of transactions", func() bool { return len(hb.txFrames()) == 4 })
	var sizes []int
	var got [][]byte
	for _, f := range hb.txFrames() {
		sizes = append(sizes, len(f))
		got = append(got, f...)
	}
	if want := []int{10, 10, 7, 1}; !slices.Equal(sizes, want) {
		t.Fatalf("frames of %v bodies, want %v", sizes, want)
	}
	if len(got) != 28 {
		t.Fatalf("%d bodies taken, want 28", len(got))
	}
	for i := range got {
		if !bytes.Equal(got[i], bodies[i]) {
			t.Fatalf("body %d differs", i)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if n := len(hc.txFrames()); n != 0 {
		t.Fatalf("the peer relayed %d frames of transactions", n)
	}
	if n := len(ha.txFrames()); n != 0 {
		t.Fatalf("the sender took %d frames", n)
	}
}

// The frames of transactions of a peer are bounded by TxQueueBytes, the newest dropped and
// counted, and the stream writes the consensus frames before them
func TestTxQueue(t *testing.T) {
	cfg := DefaultConfig
	cfg.TxQueueBytes = 100
	s := NewService(cfg, newTestHandler())
	p := newPeer("unused", cfg)
	s.peers["unused"] = p
	dropped := bftmon.Get().Messages[bftmon.TxFramesDropped]
	// a frame of one body of 40 bytes is 53 bytes: the second is past 100
	s.SendTxs([][]byte{make([]byte, 40)})
	s.SendTxs([][]byte{make([]byte, 40)})
	if n := bftmon.Get().Messages[bftmon.TxFramesDropped] - dropped; n != 1 || len(p.txs) != 1 || p.txBytes.Load() != 53 {
		t.Fatalf("dropped %d, queued %d frames of %d bytes", n, len(p.txs), p.txBytes.Load())
	}
	vote := testVote(0).Marshal()
	s.SendVote(testVote(0))
	s.SendVote(testVote(0))

	server, client := net.Pipe()
	defer client.Close()
	go p.stream(server)
	defer p.close()
	var kinds []uint8
	for range 3 {
		kind, payload, err := ReadFrame(client, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if kind == FrameVote && !bytes.Equal(payload, vote) {
			t.Fatal("the vote differs")
		}
		kinds = append(kinds, kind)
	}
	if want := []uint8{FrameVote, FrameVote, FrameTxs}; !slices.Equal(kinds, want) {
		t.Fatalf("frames of kinds %v, want %v", kinds, want)
	}
	if p.txBytes.Load() != 0 {
		t.Fatalf("%d bytes still counted", p.txBytes.Load())
	}
	// the room is back once the frame is written
	if !p.sendTxs(AppendFrame(nil, FrameTxs, MarshalTxs([][]byte{make([]byte, 80)}))) {
		t.Fatal("a frame is dropped from an empty queue")
	}
}

// A frame of transactions longer than max_block_size closes the stream before it is read
func TestTxFrameLimit(t *testing.T) {
	h := newTestHandler()
	addr, _ := testServer(t, NewService(DefaultConfig, h))
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	(&network.RequestType{Type: network.RequestTypeBftMessages}).Write(conn)
	conn.Write(AppendFrame(nil, FrameTxs, MarshalTxs([][]byte{[]byte("ok")})))
	waitFor(t, "the frame", func() bool { return len(h.txFrames()) == 1 })
	conn.Write(AppendFrame(nil, FrameTxs, MarshalTxs([][]byte{make([]byte, 993)})))
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("the stream is not closed")
	}
	if n := len(h.txFrames()); n != 1 {
		t.Fatalf("%d frames taken", n)
	}
}

// A validator says to its peers that it has transactions at a height; a peer takes the word and
// relays none, and a frame of another length does not decode
func TestSendPending(t *testing.T) {
	ha, hb, hc := newTestHandler(), newTestHandler(), newTestHandler()
	a, b, c := NewService(DefaultConfig, ha), NewService(DefaultConfig, hb), NewService(DefaultConfig, hc)
	addrB, _ := testServer(t, b)
	addrC, _ := testServer(t, c)
	a.SetPeers([]string{addrB})
	b.SetPeers([]string{addrC})
	defer a.Close()
	defer b.Close()

	a.SendPending(288, 3)
	waitFor(t, "the hint", func() bool { return len(hb.pendingHints()) == 1 })
	if got := hb.pendingHints()[0]; got != [2]uint64{288, 3} {
		t.Fatalf("the hint is %v", got)
	}
	undecoded := bftmon.Get().Messages[bftmon.MessagesUndecoded]
	a.Broadcast(FramePending, make([]byte, pendingPayload-1))
	waitFor(t, "the short hint", func() bool { return bftmon.Get().Messages[bftmon.MessagesUndecoded] > undecoded })
	time.Sleep(100 * time.Millisecond)
	if n := len(hc.pendingHints()); n != 0 {
		t.Fatalf("the peer relayed %d hints", n)
	}
	if n := len(hb.pendingHints()); n != 1 {
		t.Fatalf("the peer took %d hints", n)
	}
}

// A node gives the first transactions of its pool, at most the count asked, max_tx_block and
// max_block_size together; the client refuses an answer past what it asked
func TestFetchPending(t *testing.T) {
	h := newTestHandler()
	s := NewService(DefaultConfig, h)
	defer s.Close()
	addr, _ := testServer(t, s)
	// MaxBlockSize 1000 and MaxBlockTxs 10
	for i := range 12 {
		h.pending = append(h.pending, bytes.Repeat([]byte{byte(i + 1)}, 90))
	}
	got, err := FetchPending(addr, 3, 1000, time.Second)
	if err != nil || len(got) != 3 {
		t.Fatalf("3 asked: %d, %v", len(got), err)
	}
	for i := range got {
		if !bytes.Equal(got[i], h.pending[i]) {
			t.Fatalf("transaction %d is not the pool's", i)
		}
	}
	// 10 of 90 bytes take 900 of the 1000: all of max_tx_block
	if got, err := FetchPending(addr, 50, 1000, time.Second); err != nil || len(got) != 10 {
		t.Fatalf("50 asked: %d, %v", len(got), err)
	}
	// the server sends the transactions that fit max_block_size, the first 4 of 250 bytes
	h.mu.Lock()
	for i := range h.pending {
		h.pending[i] = bytes.Repeat([]byte{byte(i + 1)}, 250)
	}
	h.mu.Unlock()
	if got, err := FetchPending(addr, 10, 1000, time.Second); err != nil || len(got) != 4 {
		t.Fatalf("by the bytes: %d, %v", len(got), err)
	}
	// a client of a lower max_block_size refuses what is past it
	if _, err := FetchPending(addr, 10, 600, time.Second); err == nil {
		t.Fatal("an answer past max_block_size is taken")
	}
	h.mu.Lock()
	h.pending = nil
	h.mu.Unlock()
	if got, err := FetchPending(addr, 10, 1000, time.Second); err != nil || len(got) != 0 {
		t.Fatalf("an empty pool: %d, %v", len(got), err)
	}
}
