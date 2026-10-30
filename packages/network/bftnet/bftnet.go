/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package bftnet carries the F18 consensus messages between validators (design 4.5, 4.6, 4.8)
// on the TCP service of the node.
//
// A node keeps one stream to each other validator: a connection that opens with the request type
// RequestTypeBftMessages and then carries frames, kind u8 ‖ len u32 ‖ payload, the payload being
// the wire form of a proposal, a vote, an evidence or a batch of a proposal. A stream carries at most Config.Rate frames
// a second; past that the receiver closes it and refuses the address of the sender for
// Config.Block. A frame longer than its kind allows closes the stream before it is read.
//
// A proposal carries its block in the compact form of package block, the hashes of its
// transactions in the place of their bodies. A node asks the proposer for the bodies it does not
// have by the request RequestTypeBftTransactions: height u64 ‖ round u32 ‖ hash 32 ‖ count u32 ‖
// count × hash, answered by count u32 ‖ count × (len u32 ‖ body), a body of no bytes one the
// proposer does not have; the nil hash asks for the transactions of the batches of the round. A node fetches the certificates of a range of heights by the request
// RequestTypeBftCertificates: from u64 ‖ count u16, answered by count u16 ‖ count × (len u32 ‖
// certificate).
//
// The handler, the node, admits the messages (bft.Pool) and tells whether a message is new; the
// service relays a new message to the other validators, so that a message reaches every honest
// validator once one has it. A batch is the proposer's alone: the service relays none.
package bftnet

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/faults"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/service/bftmon"
)

// The kinds of the frames of a stream
const (
	FrameProposal uint8 = 1
	FrameVote     uint8 = 2
	FrameEvidence uint8 = 3
	FrameBatch    uint8 = 4
)

// MaxCertificates is the most certificates one request fetches
const MaxCertificates = 100

// maxCertificate bounds the length of a certificate a node takes: 8 + 4 + 32 + 2 bytes and, for
// each of at most 1000 validators, 4 + 2 bytes and a signature of at most 4096 bytes
const maxCertificate = 8 + 4 + bft.HashSize + 2 + 1000*(4+2+4096)

var (
	// ErrFrame is a frame of an unknown kind or longer than its kind allows
	ErrFrame = errors.New("bftnet: a malformed frame")
	// ErrRate is a stream past its rate
	ErrRate = errors.New("bftnet: the stream is past its rate")
	// ErrRefused is a stream of an address refused for its rate
	ErrRefused = errors.New("bftnet: the address is refused")
)

// Handler is the node behind the service
type Handler interface {
	// Proposal takes a decoded proposal whose block is in the compact form, and reports whether
	// it is new: the pool would admit it once its block is filled in
	Proposal(p *bft.Proposal) bool
	// Compact is the compact form of the block of a proposal of the node
	Compact(p *bft.Proposal) ([]byte, error)
	// Transactions is the bodies of the transactions of the hashes in the block of the hash at
	// the height, or in the batches of the round if the hash is nil, as the block holds them, nil
	// for one the node does not have
	Transactions(height uint64, round uint32, hash bft.Hash, hashes [][]byte) [][]byte
	// Vote takes a decoded vote, and reports whether it is new
	Vote(v *bft.Vote) bool
	// Evidence takes the body of an evidence transaction, and reports whether it is new
	Evidence(body []byte) bool
	// Batch takes a decoded batch of a proposal
	Batch(b *bft.Batch)
	// Certificates is the stored certificates of the heights from, from+1, … up to count of
	// them, stopping at the first height without one
	Certificates(from uint64, count int) ([][]byte, error)
	// MaxBlockSize is max_block_size
	MaxBlockSize() int64
	// MaxBlockTxs is max_tx_block
	MaxBlockTxs() int
}

// Config is the limits of the service
type Config struct {
	// Rate is bft_peer_msg_rate, the frames a stream carries a second at most
	Rate int
	// Block is how long the address of a stream past its rate is refused
	Block time.Duration
	// Idle closes a stream without a frame for this long
	Idle time.Duration
	// Queue is the frames waiting for each peer; past it the newest are dropped, the resends
	// of the node make up for them
	Queue int
	// Dial and Write are the timeouts of a connection and of a write
	Dial, Write time.Duration
}

// DefaultConfig is the starting values of the design, section 4.6
var DefaultConfig = Config{
	Rate:  500,
	Block: 60 * time.Second,
	Idle:  60 * time.Second,
	Queue: 4096,
	Dial:  5 * time.Second,
	Write: 5 * time.Second,
}

// Service is the consensus messages of a node on the network
type Service struct {
	cfg     Config
	handler Handler

	mu      sync.Mutex
	peers   map[string]*peer
	refused map[string]time.Time // by the IP of a sender past its rate
	now     func() time.Time
}

// NewService is the service of a node
func NewService(cfg Config, handler Handler) *Service {
	return &Service{cfg: cfg, handler: handler, peers: map[string]*peer{}, refused: map[string]time.Time{}, now: time.Now}
}

var current atomic.Pointer[Service]

// Register makes the service the one the TCP server gives the consensus requests to; nil
// unregisters it
func Register(s *Service) { current.Store(s) }

// Current is the registered service, nil if none
func Current() *Service { return current.Load() }

// maxPayload is the longest payload a frame of a kind may have
func maxPayload(kind uint8, maxBlock int64) int64 {
	sig := int64(crypto.NodeSignatureSize())
	switch kind {
	case FrameProposal:
		return 8 + 4 + 4 + 4 + bft.HashSize + 4 + maxBlock + 2 + sig
	case FrameVote:
		return 1 + 8 + 4 + 4 + bft.HashSize + 2 + sig
	case FrameEvidence:
		return int64(len(bft.EvidencePrefix)) + 1 + 8 + 8 + 4 + 4 + 2*bft.HashSize + 4 + 4 + 2*(2+sig)
	case FrameBatch:
		// the hashes of a batch are bounded by the block; UnmarshalBatch counts them by max_tx_block
		return 8 + 4 + 4 + 4 + 4 + bft.MaxBatchHeader + 4 + maxBlock + 2 + sig
	}
	return -1
}

// AppendFrame appends a frame to b
func AppendFrame(b []byte, kind uint8, payload []byte) []byte {
	b = append(b, kind)
	b = binary.BigEndian.AppendUint32(b, uint32(len(payload)))
	return append(b, payload...)
}

// ReadFrame reads a frame, refusing one of an unknown kind or one longer than its kind allows
// before it reads its payload
func ReadFrame(r io.Reader, maxBlock int64) (uint8, []byte, error) {
	var head [5]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, nil, err
	}
	kind, size := head[0], int64(binary.BigEndian.Uint32(head[1:]))
	if max := maxPayload(kind, maxBlock); max < 0 || size > max {
		return 0, nil, fmt.Errorf("%w: kind %d of %d bytes", ErrFrame, kind, size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return kind, payload, nil
}

func hostOf(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

// isRefused reports whether the address of a sender is refused now
func (s *Service) isRefused(host string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.refused[host]
	if ok && !s.now().Before(until) {
		delete(s.refused, host)
		return false
	}
	return ok
}

func (s *Service) refuse(host string) {
	s.mu.Lock()
	s.refused[host] = s.now().Add(s.cfg.Block)
	s.mu.Unlock()
}

// bucket is the rate of a stream: Rate frames a second, as many in a burst
type bucket struct {
	tokens float64
	last   time.Time
}

func (b *bucket) take(rate int, now time.Time) bool {
	b.tokens += now.Sub(b.last).Seconds() * float64(rate)
	if b.tokens > float64(rate) {
		b.tokens = float64(rate)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Serve reads the frames of a stream the TCP server opened with RequestTypeBftMessages, until
// the stream ends or breaks a limit
func (s *Service) Serve(conn net.Conn) error {
	host := hostOf(conn.RemoteAddr())
	if s.isRefused(host) {
		bftmon.Count(bftmon.StreamsRefused)
		return ErrRefused
	}
	rate := &bucket{tokens: float64(s.cfg.Rate), last: s.now()}
	for {
		if err := conn.SetReadDeadline(time.Now().Add(s.cfg.Idle)); err != nil {
			return err
		}
		kind, payload, err := ReadFrame(conn, s.handler.MaxBlockSize())
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if !rate.take(s.cfg.Rate, s.now()) {
			s.refuse(host)
			bftmon.Count(bftmon.StreamsRefused)
			return ErrRate
		}
		if s.isRefused(host) {
			bftmon.Count(bftmon.StreamsRefused)
			return ErrRefused
		}
		if s.take(kind, payload) {
			s.Broadcast(kind, payload)
		}
	}
}

// take decodes a frame and gives it to the handler; a payload that does not decode is dropped
func (s *Service) take(kind uint8, payload []byte) bool {
	switch kind {
	case FrameProposal:
		if p, err := bft.UnmarshalProposal(payload, s.handler.MaxBlockSize()); err == nil {
			return s.handler.Proposal(p)
		}
	case FrameVote:
		if v, err := bft.UnmarshalVote(payload); err == nil {
			return s.handler.Vote(v)
		}
	case FrameEvidence:
		return s.handler.Evidence(payload)
	case FrameBatch:
		if b, err := bft.UnmarshalBatch(payload, s.handler.MaxBlockTxs()); err == nil {
			s.handler.Batch(b)
			return false
		}
	}
	bftmon.Count(bftmon.MessagesUndecoded)
	return false
}

// ServeCertificates answers a request RequestTypeBftCertificates
func (s *Service) ServeCertificates(rw io.ReadWriter) error {
	var req [10]byte
	if _, err := io.ReadFull(rw, req[:]); err != nil {
		return err
	}
	from, count := binary.BigEndian.Uint64(req[:8]), int(binary.BigEndian.Uint16(req[8:]))
	count = min(count, MaxCertificates)
	certs, err := s.handler.Certificates(from, count)
	if err != nil {
		return err
	}
	if len(certs) > count {
		certs = certs[:count]
	}
	b := binary.BigEndian.AppendUint16(nil, uint16(len(certs)))
	for _, c := range certs {
		b = binary.BigEndian.AppendUint32(b, uint32(len(c)))
		b = append(b, c...)
	}
	_, err = rw.Write(b)
	return err
}

// FetchCertificates fetches the certificates of the heights from, from+1, … up to count of them
// from the node at addr
func FetchCertificates(addr string, from uint64, count int, timeout time.Duration) ([][]byte, error) {
	count = min(max(count, 0), MaxCertificates)
	conn, err := faults.Dial(addr, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	if err := (&network.RequestType{Type: network.RequestTypeBftCertificates}).Write(conn); err != nil {
		return nil, err
	}
	req := binary.BigEndian.AppendUint64(nil, from)
	req = binary.BigEndian.AppendUint16(req, uint16(count))
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	var n [2]byte
	if _, err := io.ReadFull(conn, n[:]); err != nil {
		return nil, err
	}
	got := int(binary.BigEndian.Uint16(n[:]))
	if got > count {
		return nil, fmt.Errorf("bftnet: %d certificates for %d asked", got, count)
	}
	certs := make([][]byte, 0, got)
	for range got {
		var size [4]byte
		if _, err := io.ReadFull(conn, size[:]); err != nil {
			return nil, err
		}
		l := binary.BigEndian.Uint32(size[:])
		if l > maxCertificate {
			return nil, fmt.Errorf("bftnet: a certificate of %d bytes", l)
		}
		c := make([]byte, l)
		if _, err := io.ReadFull(conn, c); err != nil {
			return nil, err
		}
		certs = append(certs, c)
	}
	return certs, nil
}

// ServeTransactions answers a request RequestTypeBftTransactions
func (s *Service) ServeTransactions(rw io.ReadWriter) error {
	var req [8 + 4 + bft.HashSize + 4]byte
	if _, err := io.ReadFull(rw, req[:]); err != nil {
		return err
	}
	height, round := binary.BigEndian.Uint64(req[:8]), binary.BigEndian.Uint32(req[8:])
	count := int64(binary.BigEndian.Uint32(req[12+bft.HashSize:]))
	var hash bft.Hash
	copy(hash[:], req[12:])
	maxBlock := s.handler.MaxBlockSize()
	if count*bft.HashSize > maxBlock {
		return fmt.Errorf("bftnet: %d transactions asked", count)
	}
	hashes := make([][]byte, count)
	all := make([]byte, count*bft.HashSize)
	if _, err := io.ReadFull(rw, all); err != nil {
		return err
	}
	for i := range hashes {
		hashes[i] = all[i*bft.HashSize : (i+1)*bft.HashSize]
	}
	bodies := s.handler.Transactions(height, round, hash, hashes)
	b := binary.BigEndian.AppendUint32(nil, uint32(len(hashes)))
	for i := range hashes {
		var body []byte
		if i < len(bodies) {
			body = bodies[i]
		}
		b = binary.BigEndian.AppendUint32(b, uint32(len(body)))
		b = append(b, body...)
	}
	_, err := rw.Write(b)
	return err
}

// FetchTransactions fetches from the node at addr the bodies of the transactions of the hashes in
// the block of the hash at the height, or in the batches of the round if the hash is nil, nil for
// one it does not have; together they are at most maxBlock bytes
func FetchTransactions(addr string, height uint64, round uint32, hash bft.Hash, hashes [][]byte, maxBlock int64,
	timeout time.Duration) ([][]byte, error) {
	conn, err := faults.Dial(addr, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	if err := (&network.RequestType{Type: network.RequestTypeBftTransactions}).Write(conn); err != nil {
		return nil, err
	}
	req := make([]byte, 0, 8+4+bft.HashSize+4+len(hashes)*bft.HashSize)
	req = binary.BigEndian.AppendUint64(req, height)
	req = binary.BigEndian.AppendUint32(req, round)
	req = append(req, hash[:]...)
	req = binary.BigEndian.AppendUint32(req, uint32(len(hashes)))
	for _, h := range hashes {
		req = append(req, h...)
	}
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	r := bufio.NewReader(conn)
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return nil, err
	}
	if got := binary.BigEndian.Uint32(n[:]); int(got) != len(hashes) {
		return nil, fmt.Errorf("bftnet: %d transactions for %d asked", got, len(hashes))
	}
	bodies := make([][]byte, len(hashes))
	left := maxBlock
	for i := range bodies {
		if _, err := io.ReadFull(r, n[:]); err != nil {
			return nil, err
		}
		l := int64(binary.BigEndian.Uint32(n[:]))
		if l > left {
			return nil, fmt.Errorf("bftnet: the transactions are more than %d bytes", maxBlock)
		}
		left -= l
		if l == 0 {
			continue
		}
		bodies[i] = make([]byte, l)
		if _, err := io.ReadFull(r, bodies[i]); err != nil {
			return nil, err
		}
	}
	return bodies, nil
}

// SetPeers sets the TCP addresses of the other validators: the service opens a stream to each
// new one and closes those of the addresses left out
func (s *Service) SetPeers(addrs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := map[string]bool{}
	for _, a := range addrs {
		keep[a] = true
		if s.peers[a] == nil {
			p := newPeer(a, s.cfg)
			s.peers[a] = p
			go p.run()
		}
	}
	for a, p := range s.peers {
		if !keep[a] {
			p.close()
			delete(s.peers, a)
		}
	}
}

// Close closes the streams of all peers
func (s *Service) Close() { s.SetPeers(nil) }

// Broadcast sends a frame to every peer
func (s *Service) Broadcast(kind uint8, payload []byte) {
	frame := AppendFrame(nil, kind, payload)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.peers {
		p.send(frame)
	}
}

// SendProposal broadcasts a proposal of the node, its block in the compact form
func (s *Service) SendProposal(p *bft.Proposal) error {
	compact, err := s.handler.Compact(p)
	if err != nil {
		return err
	}
	s.Broadcast(FrameProposal, p.MarshalBlock(compact))
	return nil
}

// SendVote broadcasts a vote
func (s *Service) SendVote(v *bft.Vote) { s.Broadcast(FrameVote, v.Marshal()) }

// SendEvidence broadcasts the body of an evidence transaction
func (s *Service) SendEvidence(body []byte) { s.Broadcast(FrameEvidence, body) }

// SendBatch broadcasts a batch of a proposal of the node
func (s *Service) SendBatch(b *bft.Batch) { s.Broadcast(FrameBatch, b.Marshal()) }

// peer is the stream to another validator: it connects, and connects again after a failure
type peer struct {
	addr  string
	cfg   Config
	queue chan []byte
	done  chan struct{}
	once  sync.Once
}

func newPeer(addr string, cfg Config) *peer {
	return &peer{addr: addr, cfg: cfg, queue: make(chan []byte, cfg.Queue), done: make(chan struct{})}
}

func (p *peer) send(frame []byte) {
	select {
	case p.queue <- frame:
	default:
	}
}

func (p *peer) close() { p.once.Do(func() { close(p.done) }) }

func (p *peer) run() {
	wait := 500 * time.Millisecond
	for {
		conn, err := p.connect()
		if err == nil {
			wait = 500 * time.Millisecond
			err = p.stream(conn)
			conn.Close()
			if err == nil {
				return
			}
		}
		select {
		case <-p.done:
			return
		case <-time.After(wait):
		}
		wait = min(2*wait, 5*time.Second)
	}
}

func (p *peer) connect() (net.Conn, error) {
	conn, err := faults.Dial(p.addr, p.cfg.Dial)
	if err != nil {
		return nil, err
	}
	conn.SetWriteDeadline(time.Now().Add(p.cfg.Write))
	if err := (&network.RequestType{Type: network.RequestTypeBftMessages}).Write(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// stream writes the queued frames until the peer is closed, nil, or a write fails
func (p *peer) stream(conn net.Conn) error {
	for {
		select {
		case <-p.done:
			return nil
		case frame := <-p.queue:
			conn.SetWriteDeadline(time.Now().Add(p.cfg.Write))
			if _, err := conn.Write(frame); err != nil {
				return err
			}
		}
	}
}
