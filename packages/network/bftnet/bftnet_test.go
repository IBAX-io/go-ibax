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
)

// testHandler admits each message once, by its wire form
type testHandler struct {
	mu    sync.Mutex
	seen  map[string]bool
	votes []*bft.Vote
	props []*bft.Proposal
	evids [][]byte
	certs [][]byte
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

func (h *testHandler) MaxBlockSize() int64 { return 1000 }

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
		{0, 1}, {4, 1},
		{FrameVote, maxPayload(FrameVote, 1000) + 1},
		{FrameProposal, maxPayload(FrameProposal, 1000) + 1},
		{FrameEvidence, maxPayload(FrameEvidence, 1000) + 1},
	} {
		head := []byte{c.kind}
		head = binary.BigEndian.AppendUint32(head, uint32(c.size))
		// no payload follows: reading it would fail with io.ErrUnexpectedEOF, not ErrFrame
		if _, _, err := ReadFrame(bytes.NewReader(head), 1000); !errors.Is(err, ErrFrame) {
			t.Fatalf("kind %d of %d bytes: %v", c.kind, c.size, err)
		}
	}
	for _, kind := range []uint8{FrameProposal, FrameVote, FrameEvidence} {
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

	// the node's own messages
	a.SendVote(testVote(1))
	waitFor(t, "the vote of the node", func() bool { _, votes, _ := hb.counts(); return votes == 2 })
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
