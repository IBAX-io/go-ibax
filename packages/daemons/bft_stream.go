/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/transaction"

	log "github.com/sirupsen/logrus"
)

// Streaming proposals (F18 design 4.2): the proposer sends the header of its block and the
// transactions its trial puts in the block in batches while it tries the block (bftStream), and a
// validator runs them in a speculation (bftSpeculator) that its valid(v) of the proposal goes on
// from. The speculation holds DBLock until the proposal comes or it is aborted: the driver aborts
// it before it takes DBLock itself (lockWrites).

const (
	// bftBatchTxs and bftBatchWait bound a batch: the proposer sends one when its trial put
	// bftBatchTxs transactions in the block since the batch before, or when bftBatchWait passed
	bftBatchTxs  = 256
	bftBatchWait = 20 * time.Millisecond
	// bftBatchQueue is the batches of a speculation waiting to run; past it the speculation runs
	// no more batches and the proposal brings the rest
	bftBatchQueue = 1024
)

// bftStream is the Stream of the trial of the block of the proposer at a height and a round
type bftStream struct {
	send      func(*bft.Batch)
	key       []byte
	networkID uint64
	height    uint64
	round     uint32
	validator uint32
	kept      *bftStreamed
	seq       uint32
	hashes    [][]byte
	sent      time.Time
	off       bool
}

// Header sends the first batch, which has the shell of the block. A shell above MaxBatchHeader
// turns the stream off: the validators then try the proposal from the start.
func (s *bftStream) Header(shell []byte) {
	if len(shell) > bft.MaxBatchHeader {
		s.off = true
		return
	}
	s.batch(shell, nil)
}

// Commit takes a transaction the trial put in the block and sends a batch when it is due
func (s *bftStream) Commit(t *transaction.Transaction) {
	if s.off {
		return
	}
	s.commit(t.Hash(), t)
}

func (s *bftStream) commit(h []byte, t *transaction.Transaction) {
	s.kept.add(h, t)
	s.hashes = append(s.hashes, h)
	if len(s.hashes) >= bftBatchTxs || time.Since(s.sent) >= bftBatchWait {
		s.flush()
	}
}

// flush sends the transactions not sent yet, before the proposal
func (s *bftStream) flush() {
	if s.off || len(s.hashes) == 0 {
		return
	}
	s.batch(nil, s.hashes)
	s.hashes = nil
}

func (s *bftStream) batch(header []byte, hashes [][]byte) {
	b := &bft.Batch{Height: s.height, Round: s.round, Validator: s.validator, Seq: s.seq, Header: header, Hashes: hashes}
	if err := b.Sign(s.key, s.networkID); err != nil {
		s.off = true
		return
	}
	s.seq++
	s.send(b)
	s.sent = time.Now()
}

// bftStreamed is the transactions the node streamed for a height and a round: a validator that
// misses one in its queue takes its body from the proposer
type bftStreamed struct {
	mu  sync.Mutex
	txs map[string]*transaction.Transaction
}

func (s *bftStreamed) add(hash []byte, t *transaction.Transaction) {
	s.mu.Lock()
	s.txs[string(hash)] = t
	s.mu.Unlock()
}

// body is the body of a streamed transaction, compressed, or nil
func (s *bftStreamed) body(hash []byte) []byte {
	s.mu.Lock()
	t := s.txs[string(hash)]
	s.mu.Unlock()
	if t == nil {
		return nil
	}
	return t.Compressed()
}

// bftSpeculator runs the batches of the proposals of the height of the node: one speculation at
// a time, of the highest round whose first batch it took
type bftSpeculator struct {
	logger *log.Entry
	// begin begins a speculation, block.Speculate, and parse parses the transactions of a batch,
	// block.ParseTransactions
	begin func(shell []byte, height int64) (bftSpeculation, error)
	parse func(bodies [][]byte) ([]*transaction.Transaction, error)
	// raw is the bodies of the transactions of the queue, fetch takes those it misses from the
	// proposer of a batch, and idle is how long a speculation waits for a batch or the proposal
	raw   func(hashes [][]byte) [][]byte
	fetch func(b *bft.Batch, key []byte, hashes [][]byte) ([][]byte, error)
	idle  func() time.Duration

	mu  sync.Mutex
	cur *bftSpecRun
}

// bftSpecRun is a speculation: its goroutine begins it on the first batch and runs the batches
// that follow in their order. The fields below key are guarded by the mutex of the speculator.
type bftSpecRun struct {
	height    uint64
	round     uint32
	validator uint32
	key       []byte
	batches   chan *bft.Batch
	quit      chan struct{}
	exited    chan struct{}

	next    uint32 // the seq of the next batch it takes
	closed  bool   // it takes no more batches
	stopped bool   // quit is closed
	abort   bool   // the goroutine rolls the speculation back as it quits
	spec    bftSpeculation
}

// bftSpeculation is a speculation of package block
type bftSpeculation interface {
	Add(txs []*transaction.Transaction)
	Finish(binData []byte, height int64, hash bft.Hash) error
	Abort()
}

// speculate is block.Speculate under DBLock
func speculate(shell []byte, height int64) (bftSpeculation, error) {
	spec, err := block.Speculate(shell, height, dbWrites{})
	if err != nil {
		return nil, err
	}
	return spec, nil
}

// stop ends the goroutine of the speculation, which rolls it back if abort
func (r *bftSpecRun) stop(abort bool) {
	r.closed = true
	if !r.stopped {
		r.stopped, r.abort = true, abort
		close(r.quit)
	}
}

// take takes a batch the pool admitted, signed by the key of its proposer
func (s *bftSpeculator) take(b *bft.Batch, key []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cur
	if b.Seq == 0 {
		if c != nil && (c.height > b.Height || c.height == b.Height && c.round >= b.Round) {
			return
		}
		if c != nil {
			c.stop(true)
		}
		c = &bftSpecRun{height: b.Height, round: b.Round, validator: b.Validator, key: key, next: 1,
			batches: make(chan *bft.Batch, bftBatchQueue), quit: make(chan struct{}), exited: make(chan struct{})}
		s.cur = c
		go s.run(c, b)
		return
	}
	if c == nil || c.closed || c.height != b.Height || c.round != b.Round || c.validator != b.Validator {
		return
	}
	if b.Seq != c.next {
		// a batch lost: the speculation runs what it has, the proposal brings the rest
		c.closed = true
		return
	}
	select {
	case c.batches <- b:
		c.next++
	default:
		c.closed = true
	}
}

// run begins the speculation of the first batch and runs the batches that follow until it is
// stopped, finished or idle
func (s *bftSpeculator) run(c *bftSpecRun, first *bft.Batch) {
	defer close(c.exited)
	spec, err := s.begin(first.Header, int64(first.Height))
	s.mu.Lock()
	if err != nil || c.abort {
		c.closed = true
		s.mu.Unlock()
		if err != nil {
			s.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err, "height": first.Height, "round": first.Round}).
				Warn("beginning a speculation")
		} else {
			spec.Abort()
		}
		return
	}
	c.spec = spec
	s.mu.Unlock()
	idle := time.NewTimer(s.idle())
	defer idle.Stop()
	// a batch that fails ends what the speculation runs: the batches after it do not follow
	// what it ran
	failed := false
	next := first
	for {
		if next != nil && !failed && len(next.Hashes) > 0 && !s.add(c, spec, next) {
			failed = true
			s.mu.Lock()
			c.closed = true
			s.mu.Unlock()
		}
		next = nil
		select {
		case <-c.quit:
			s.mu.Lock()
			abort := c.abort
			s.mu.Unlock()
			if abort {
				spec.Abort()
				metrics.Speculations.WithLabelValues("aborted").Inc()
			}
			return
		case <-idle.C:
			s.mu.Lock()
			if s.cur == c {
				s.cur = nil
			}
			c.stop(false)
			s.mu.Unlock()
			spec.Abort()
			metrics.Speculations.WithLabelValues("aborted").Inc()
			return
		case next = <-c.batches:
			idle.Reset(s.idle())
		}
	}
}

// add runs the transactions of a batch, and reports whether it had their bodies
func (s *bftSpeculator) add(c *bftSpecRun, spec bftSpeculation, b *bft.Batch) bool {
	bodies := s.raw(b.Hashes)
	var missing [][]byte
	var at []int
	for i, body := range bodies {
		if body == nil {
			missing, at = append(missing, b.Hashes[i]), append(at, i)
		}
	}
	if len(missing) > 0 {
		fetched, err := s.fetch(b, c.key, missing)
		if err == nil && len(fetched) != len(missing) {
			err = fmt.Errorf("%d bodies for %d transactions", len(fetched), len(missing))
		}
		if err != nil {
			s.logger.WithFields(log.Fields{"type": consts.NetworkError, "error": err, "height": b.Height, "round": b.Round}).
				Debug("taking the transactions of a batch")
			return false
		}
		for j, i := range at {
			bodies[i] = fetched[j]
		}
	}
	txs, err := s.parse(bodies)
	if err != nil {
		s.logger.WithFields(log.Fields{"type": consts.ParseError, "error": err, "height": b.Height, "round": b.Round}).
			Debug("parsing the transactions of a batch")
		return false
	}
	spec.Add(txs)
	return true
}

// finish is valid(v) of the proposal of the block of the hash at the height from the speculation
// of the height; block.ErrSpeculation if there is none or it does not decide the proposal. The
// speculation is over either way.
func (s *bftSpeculator) finish(height uint64, hash bft.Hash, bin []byte) error {
	if s == nil {
		return block.ErrSpeculation
	}
	s.mu.Lock()
	c := s.cur
	s.cur = nil
	if c == nil {
		s.mu.Unlock()
		return block.ErrSpeculation
	}
	if c.height != height || c.spec == nil {
		c.stop(true)
		s.mu.Unlock()
		return block.ErrSpeculation
	}
	spec := c.spec
	c.stop(false)
	s.mu.Unlock()
	err := spec.Finish(bin, int64(height), hash)
	switch {
	case err == nil:
		metrics.Speculations.WithLabelValues("finished").Inc()
	case errors.Is(err, block.ErrSpeculation):
		metrics.Speculations.WithLabelValues("validated").Inc()
	default:
		metrics.Speculations.WithLabelValues("refused").Inc()
	}
	return err
}

// abort rolls the speculation back, if one runs, and waits until it releases DBLock
func (s *bftSpeculator) abort() {
	if s == nil {
		return
	}
	s.mu.Lock()
	c := s.cur
	s.cur = nil
	if c != nil {
		c.stop(true)
	}
	s.mu.Unlock()
	if c != nil {
		<-c.exited
	}
}

// speculations is the speculator of the driver, nil on a chain without it
var speculations *bftSpeculator

// lockWrites takes DBLock for the driver: it aborts the speculation first, which holds it
func lockWrites() {
	speculations.abort()
	DBLock()
}

var errNoProposer = errors.New("no address of the proposer")
