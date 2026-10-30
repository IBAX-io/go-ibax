/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bftnet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/service/bftmon"
)

// testHandler admits each message once, by its wire form
type testHandler struct {
	mu      sync.Mutex
	seen    map[string]bool
	votes   []*bft.Vote
	props   []*bft.Proposal
	evids   [][]byte
	certs   [][]byte
	txs     map[string][]byte // the bodies of the transactions by their hashes
	asked   []uint64          // the heights of the transactions asked
	rounds  []uint32          // the rounds of the transactions asked
	batches []*bft.Batch
}

func newTestHandler() *testHandler { return &testHandler{seen: map[string]bool{}} }

func (h *testHandler) first(b []byte) bool {
	if h.seen[string(b)] {
		return false
	}
	h.seen[string(b)] = true
	return true
}

func (h *testHandler) Proposal(p *bft.Proposal) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.props = append(h.props, p)
	return h.first(p.Marshal())
}

func (h *testHandler) Vote(v *bft.Vote) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.votes = append(h.votes, v)
	return h.first(v.Marshal())
}

func (h *testHandler) Evidence(body []byte) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.evids = append(h.evids, body)
	return h.first(body)
}

func (h *testHandler) Certificates(from uint64, count int) ([][]byte, error) {
	if from >= uint64(len(h.certs)) {
		return nil, nil
	}
	// more than asked: the service cuts them
	return h.certs[from:], nil
}

// Compact is the block of the proposal behind a mark
func (h *testHandler) Compact(p *bft.Proposal) ([]byte, error) {
	if len(p.Block) == 0 {
		return nil, errors.New("no block")
	}
	return append([]byte("compact:"), p.Block...), nil
}

func (h *testHandler) Transactions(height uint64, round uint32, _ bft.Hash, hashes [][]byte) [][]byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.asked = append(h.asked, height)
	h.rounds = append(h.rounds, round)
	bodies := make([][]byte, len(hashes))
	for i, x := range hashes {
		bodies[i] = h.txs[string(x)]
	}
	return bodies
}

func (h *testHandler) MaxBlockSize() int64 { return 1000 }

func (h *testHandler) MaxBlockTxs() int { return 10 }

func (h *testHandler) Batch(b *bft.Batch) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.batches = append(h.batches, b)
}

func (h *testHandler) batchCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.batches)
}

func (h *testHandler) counts() (int, int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.props), len(h.votes), len(h.evids)
}

func testVote(round uint32) *bft.Vote {
	return &bft.Vote{Type: bft.TypePrevote, Height: 5, Round: round, Validator: 1, Hash: bft.Hash{1},
		Signature: make([]byte, crypto.NodeSignatureSize())}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// testServer is the TCP server of a node with the service, as tcpserver dispatches the requests
func testServer(t *testing.T, s *Service) (string, func() int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	accepted := 0
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			accepted++
			mu.Unlock()
			go func() {
				defer conn.Close()
				rt := &network.RequestType{}
				if rt.Read(conn) != nil {
					return
				}
				switch rt.Type {
				case network.RequestTypeBftMessages:
					s.Serve(conn)
				case network.RequestTypeBftCertificates:
					s.ServeCertificates(conn)
				case network.RequestTypeBftTransactions:
					s.ServeTransactions(conn)
				}
			}()
		}
	}()
	t.Cleanup(func() { l.Close() })
	return l.Addr().String(), func() int { mu.Lock(); defer mu.Unlock(); return accepted }
}

// A frame is refused by its kind and its length before its payload is read
func TestReadFrame(t *testing.T) {
	v := testVote(0).Marshal()
	kind, payload, err := ReadFrame(bytes.NewReader(AppendFrame(nil, FrameVote, v)), 1000)
	if err != nil || kind != FrameVote || !bytes.Equal(payload, v) {
		t.Fatalf("a vote frame: %d, %v", kind, err)
	}
	for _, c := range []struct {
		kind uint8
		size int64
	}{
		{0, 1}, {5, 1},
		{FrameVote, maxPayload(FrameVote, 1000) + 1},
		{FrameProposal, maxPayload(FrameProposal, 1000) + 1},
		{FrameEvidence, maxPayload(FrameEvidence, 1000) + 1},
		{FrameBatch, maxPayload(FrameBatch, 1000) + 1},
	} {
		head := []byte{c.kind}
		head = binary.BigEndian.AppendUint32(head, uint32(c.size))
		// no payload follows: reading it would fail with io.ErrUnexpectedEOF, not ErrFrame
		if _, _, err := ReadFrame(bytes.NewReader(head), 1000); !errors.Is(err, ErrFrame) {
			t.Fatalf("kind %d of %d bytes: %v", c.kind, c.size, err)
		}
	}
	for _, kind := range []uint8{FrameProposal, FrameVote, FrameEvidence, FrameBatch} {
		max := maxPayload(kind, 1000)
		frame := AppendFrame(nil, kind, make([]byte, max))
		if _, _, err := ReadFrame(bytes.NewReader(frame), 1000); err != nil {
			t.Fatalf("kind %d of the longest length: %v", kind, err)
		}
	}
	// the longest messages fit their frames
	p := &bft.Proposal{Height: 1, Round: 1, ValidRound: 0, Hash: bft.Hash{1}, Block: make([]byte, 1000),
		Signature: make([]byte, crypto.NodeSignatureSize())}
	if int64(len(p.Marshal())) != maxPayload(FrameProposal, 1000) {
		t.Fatalf("a proposal of %d bytes, the frame allows %d", len(p.Marshal()), maxPayload(FrameProposal, 1000))
	}
	if int64(len(v)) != maxPayload(FrameVote, 1000) {
		t.Fatalf("a vote of %d bytes, the frame allows %d", len(v), maxPayload(FrameVote, 1000))
	}
	e := &bft.Evidence{SigA: make([]byte, crypto.NodeSignatureSize()), SigB: make([]byte, crypto.NodeSignatureSize())}
	if int64(len(e.Marshal())) != maxPayload(FrameEvidence, 1000) {
		t.Fatalf("an evidence of %d bytes, the frame allows %d", len(e.Marshal()), maxPayload(FrameEvidence, 1000))
	}
}

// A stream past its rate is closed, and its address refused for Block, on every stream
func TestServeRate(t *testing.T) {
	h := newTestHandler()
	cfg := DefaultConfig
	cfg.Rate = 5
	s := NewService(cfg, h)
	now := time.Unix(1000, 0)
	s.now = func() time.Time { return now }
	refused := bftmon.Get().Messages[bftmon.StreamsRefused]

	serve := func(frames int) (error, int) {
		server, client := net.Pipe()
		done := make(chan error, 1)
		go func() { done <- s.Serve(server); server.Close() }()
		sent := 0
		for i := range frames {
			if _, err := client.Write(AppendFrame(nil, FrameVote, testVote(uint32(i)).Marshal())); err != nil {
				break
			}
			sent++
		}
		client.Close()
		err := <-done
		if errors.Is(err, io.ErrClosedPipe) {
			// a pipe closed by the client, the EOF of a TCP stream
			err = nil
		}
		return err, sent
	}
	if err, sent := serve(5); err != nil || sent != 5 {
		t.Fatalf("5 frames at a rate of 5: %v, %d sent", err, sent)
	}
	if err, sent := serve(6); !errors.Is(err, ErrRate) || sent != 6 {
		t.Fatalf("6 frames at a rate of 5: %v, %d sent", err, sent)
	}
	if _, votes, _ := h.counts(); votes != 10 {
		t.Fatalf("%d votes taken, want 10", votes)
	}
	if err, _ := serve(1); !errors.Is(err, ErrRefused) {
		t.Fatalf("a new stream of the refused address: %v", err)
	}
	if err, _ := serve(0); !errors.Is(err, ErrRefused) {
		t.Fatalf("a new stream of the refused address, before its first frame: %v", err)
	}
	now = now.Add(cfg.Block - time.Second)
	if err, _ := serve(1); !errors.Is(err, ErrRefused) {
		t.Fatalf("a stream before the refusal ends: %v", err)
	}
	now = now.Add(time.Second)
	if err, sent := serve(5); err != nil || sent != 5 {
		t.Fatalf("a stream after the refusal: %v, %d sent", err, sent)
	}
	if _, votes, _ := h.counts(); votes != 15 {
		t.Fatalf("%d votes taken, want 15", votes)
	}
	if n := bftmon.Get().Messages[bftmon.StreamsRefused] - refused; n != 4 {
		t.Fatalf("%d streams counted refused, want 4", n)
	}

	// a frame too long closes the stream
	server, client := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- s.Serve(server) }()
	client.Write([]byte{FrameVote, 0xff, 0xff, 0xff, 0xff})
	if err := <-done; !errors.Is(err, ErrFrame) {
		t.Fatalf("a frame too long: %v", err)
	}
	client.Close()
}

// A node relays a message the handler takes as new to its peers, once, over the streams it
// keeps to them; a payload that does not decode is neither taken nor relayed
func TestRelay(t *testing.T) {
	ha, hb := newTestHandler(), newTestHandler()
	a, b := NewService(DefaultConfig, ha), NewService(DefaultConfig, hb)
	addrB, _ := testServer(t, b)
	a.SetPeers([]string{addrB})
	defer a.Close()

	server, client := net.Pipe()
	go a.Serve(server)
	defer client.Close()
	v := testVote(0)
	p := &bft.Proposal{Height: 5, Round: 1, ValidRound: 0, Validator: 1, Hash: bft.Hash{2}, Block: []byte{1, 2},
		Signature: make([]byte, crypto.NodeSignatureSize())}
	for _, f := range [][]byte{
		AppendFrame(nil, FrameVote, v.Marshal()),
		AppendFrame(nil, FrameVote, v.Marshal()),
		AppendFrame(nil, FrameProposal, p.Marshal()),
		AppendFrame(nil, FrameEvidence, []byte("evidence")),
		AppendFrame(nil, FrameVote, []byte{1, 2, 3}),
	} {
		if _, err := client.Write(f); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "the relayed messages", func() bool {
		props, votes, evids := hb.counts()
		return props == 1 && votes == 1 && evids == 1
	})
	if props, votes, evids := ha.counts(); props != 1 || votes != 2 || evids != 1 {
		t.Fatalf("the relaying node took %d, %d, %d", props, votes, evids)
	}
	time.Sleep(100 * time.Millisecond)
	if props, votes, evids := hb.counts(); props != 1 || votes != 1 || evids != 1 {
		t.Fatalf("the peer took %d, %d, %d, want one of each", props, votes, evids)
	}
	if got := hb.votes[0]; got.Marshal() == nil || !bytes.Equal(got.Marshal(), v.Marshal()) {
		t.Fatal("the relayed vote differs")
	}

	// the node's own messages, a proposal with its block in the compact form
	a.SendVote(testVote(1))
	waitFor(t, "the vote of the node", func() bool { _, votes, _ := hb.counts(); return votes == 2 })
	own := &bft.Proposal{Height: 5, Round: 2, ValidRound: 0, Validator: 1, Hash: bft.Hash{3}, Block: []byte{7},
		Signature: make([]byte, crypto.NodeSignatureSize())}
	if err := a.SendProposal(own); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the proposal of the node", func() bool { props, _, _ := hb.counts(); return props == 2 })
	hb.mu.Lock()
	got := hb.props[1].Block
	hb.mu.Unlock()
	if string(got) != "compact:\x07" {
		t.Fatalf("the proposal carries %q", got)
	}
	if err := a.SendProposal(&bft.Proposal{Height: 5, Round: 3}); err == nil {
		t.Fatal("a proposal without its compact form is sent")
	}

	// a batch of the node goes to its peers; a batch the node takes it does not relay, and one
	// above max_tx_block it does not take
	batch := &bft.Batch{Height: 5, Round: 2, Validator: 1, Seq: 1, Hashes: [][]byte{testHash(1)},
		Signature: make([]byte, crypto.NodeSignatureSize())}
	a.SendBatch(batch)
	waitFor(t, "the batch of the node", func() bool { return hb.batchCount() == 1 })
	big := &bft.Batch{Height: 5, Round: 2, Validator: 1, Seq: 2, Hashes: make([][]byte, 11),
		Signature: make([]byte, crypto.NodeSignatureSize())}
	for i := range big.Hashes {
		big.Hashes[i] = testHash(byte(i))
	}
	for _, f := range [][]byte{AppendFrame(nil, FrameBatch, big.Marshal()), AppendFrame(nil, FrameBatch, batch.Marshal())} {
		if _, err := client.Write(f); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "the batch taken", func() bool { return ha.batchCount() == 1 })
	time.Sleep(100 * time.Millisecond)
	if n := hb.batchCount(); n != 1 {
		t.Fatalf("the peer took %d batches, want the 1 of the node", n)
	}
	if got := ha.batches[0]; !bytes.Equal(got.Marshal(), batch.Marshal()) {
		t.Fatal("the batch taken differs")
	}
}

// A peer connects again after its stream breaks, and SetPeers closes the streams of the peers
// it leaves out
func TestPeerReconnect(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	got := make(chan uint8, 16)
	conns := make(chan net.Conn, 16)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			conns <- conn
			go func() {
				rt := &network.RequestType{}
				if rt.Read(conn) != nil || rt.Type != network.RequestTypeBftMessages {
					return
				}
				for {
					kind, _, err := ReadFrame(conn, 1000)
					if err != nil {
						return
					}
					got <- kind
				}
			}()
		}
	}()
	s := NewService(DefaultConfig, newTestHandler())
	s.SetPeers([]string{l.Addr().String()})
	s.SendEvidence([]byte("a"))
	if k := <-got; k != FrameEvidence {
		t.Fatalf("kind %d", k)
	}
	first := <-conns
	first.Close()
	// the write after the break fails or is lost: the peer connects again, and the next frames
	// go over the new stream
	deadline := time.After(5 * time.Second)
	var second net.Conn
	for second == nil {
		s.SendEvidence([]byte("b"))
		select {
		case second = <-conns:
		case <-deadline:
			t.Fatal("the peer did not connect again")
		case <-time.After(50 * time.Millisecond):
		}
	}
	s.SendEvidence([]byte("c"))
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("no frame over the new stream")
	}
	s.SetPeers(nil)
	second.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.Copy(io.Discard, second); err != nil {
		t.Fatalf("the stream of a peer left out is not closed: %v", err)
	}
}

// A node fetches at most the certificates it asked for, and at most MaxCertificates
func TestCertificates(t *testing.T) {
	h := newTestHandler()
	for i := range 150 {
		h.certs = append(h.certs, []byte{byte(i), 1, 2})
	}
	addr, _ := testServer(t, NewService(DefaultConfig, h))
	certs, err := FetchCertificates(addr, 10, 3, time.Second)
	if err != nil || len(certs) != 3 || !bytes.Equal(certs[0], h.certs[10]) || !bytes.Equal(certs[2], h.certs[12]) {
		t.Fatalf("3 certificates from 10: %v, %d", err, len(certs))
	}
	if certs, err = FetchCertificates(addr, 0, 1000, time.Second); err != nil || len(certs) != MaxCertificates {
		t.Fatalf("1000 certificates: %v, %d", err, len(certs))
	}
	// a request for more than MaxCertificates is answered with MaxCertificates
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	(&network.RequestType{Type: network.RequestTypeBftCertificates}).Write(conn)
	conn.Write(binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint64(nil, 0), 1000))
	var n [2]byte
	if _, err := io.ReadFull(conn, n[:]); err != nil || binary.BigEndian.Uint16(n[:]) != MaxCertificates {
		t.Fatalf("a request of 1000: %v, %d answered", err, binary.BigEndian.Uint16(n[:]))
	}
	conn.Close()
	if certs, err = FetchCertificates(addr, 140, 100, time.Second); err != nil || len(certs) != 10 {
		t.Fatalf("100 certificates from 140: %v, %d", err, len(certs))
	}
	if certs, err = FetchCertificates(addr, 500, 10, time.Second); err != nil || len(certs) != 0 {
		t.Fatalf("certificates past the chain: %v, %d", err, len(certs))
	}

	// a node answering more certificates than asked, or one too long, is refused
	for _, answer := range [][]byte{
		{0, 3, 0, 0, 0, 1, 7, 0, 0, 0, 1, 7, 0, 0, 0, 1, 7},
		// the certificate follows in full: only the bound on its length refuses it
		append(binary.BigEndian.AppendUint32([]byte{0, 1}, maxCertificate+1), make([]byte, maxCertificate+1)...),
	} {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			io.ReadFull(conn, make([]byte, 2+10))
			conn.Write(answer)
		}()
		if _, err := FetchCertificates(l.Addr().String(), 0, 2, time.Second); err == nil {
			t.Fatalf("the answer %x is taken", answer[:6])
		}
		l.Close()
	}
}

// The TCP server gives the requests to the registered service only
func TestRegister(t *testing.T) {
	if Current() != nil {
		t.Fatal("a service is registered")
	}
	s := NewService(DefaultConfig, newTestHandler())
	Register(s)
	if Current() != s {
		t.Fatal("the service is not registered")
	}
	Register(nil)
	if Current() != nil {
		t.Fatal("the service is still registered")
	}
}

func testHash(i byte) []byte {
	h := make([]byte, bft.HashSize)
	h[0] = i
	return h
}

// A node fetches the bodies of the transactions it asks the proposer for, nil for one the
// proposer does not have, and refuses an answer that is not of its request or longer than a block
func TestTransactions(t *testing.T) {
	h := newTestHandler()
	h.txs = map[string][]byte{string(testHash(1)): []byte("one"), string(testHash(3)): []byte("three")}
	addr, _ := testServer(t, NewService(DefaultConfig, h))
	bodies, err := FetchTransactions(addr, 9, 3, bft.Hash{5}, [][]byte{testHash(3), testHash(2), testHash(1)}, 1000, time.Second)
	if err != nil || len(bodies) != 3 || string(bodies[0]) != "three" || bodies[1] != nil || string(bodies[2]) != "one" {
		t.Fatalf("3 transactions: %v, %q", err, bodies)
	}
	if len(h.asked) != 1 || h.asked[0] != 9 || h.rounds[0] != 3 {
		t.Fatalf("asked %v in rounds %v", h.asked, h.rounds)
	}
	// the bodies together longer than the block
	if _, err := FetchTransactions(addr, 9, 3, bft.Hash{5}, [][]byte{testHash(3), testHash(1)}, 7, time.Second); err == nil {
		t.Fatal("bodies longer than the block are taken")
	}
	// a request for more hashes than a block holds is not answered
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	(&network.RequestType{Type: network.RequestTypeBftTransactions}).Write(conn)
	req := binary.BigEndian.AppendUint64(nil, 9)
	req = binary.BigEndian.AppendUint32(req, 3)
	req = append(req, make([]byte, bft.HashSize)...)
	conn.Write(binary.BigEndian.AppendUint32(req, 1000/bft.HashSize+1))
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, err := conn.Read(make([]byte, 4)); err == nil {
		t.Fatalf("a request of too many hashes is answered with %d bytes", n)
	}
	conn.Close()

	// an answer of another count of transactions is refused
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.ReadFull(conn, make([]byte, 2+8+4+bft.HashSize+4+bft.HashSize))
		conn.Write([]byte{0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0})
	}()
	if _, err := FetchTransactions(l.Addr().String(), 9, 3, bft.Hash{5}, [][]byte{testHash(1)}, 1000, time.Second); err == nil {
		t.Fatal("an answer of 2 transactions for 1 is taken")
	}
}
