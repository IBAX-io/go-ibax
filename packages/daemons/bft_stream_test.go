/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/stretchr/testify/require"

	log "github.com/sirupsen/logrus"
)

func testTxHash(i int) []byte {
	h := make([]byte, bft.HashSize)
	copy(h, fmt.Sprintf("tx%d", i))
	return h
}

// The proposer sends the shell first, then a batch each bftBatchTxs transactions and the rest
// when it flushes, each signed by its key; a shell above the bound turns the stream off
func TestBftStream(t *testing.T) {
	crypto.InitAsymAlgo("ECC_P256")
	private, public, err := crypto.GenNodeKeyPair()
	require.NoError(t, err)
	var sent []*bft.Batch
	bodies := newBftBodies()
	s := &bftStream{send: func(b *bft.Batch) { sent = append(sent, b) }, key: private, networkID: 7,
		height: 5, round: 1, validator: 2, kept: bodies.stream(5, 1)}
	s.Header([]byte("shell"))
	s.sent = time.Now().Add(time.Hour) // no batch is due by the time
	for i := range bftBatchTxs + 3 {
		s.commit(testTxHash(i), nil)
	}
	s.flush()
	s.flush()
	require.Len(t, sent, 3)
	for i, b := range sent {
		require.Equal(t, uint32(i), b.Seq)
		require.Equal(t, uint64(5), b.Height)
		require.Equal(t, uint32(1), b.Round)
		require.Equal(t, uint32(2), b.Validator)
		got, err := bft.UnmarshalBatch(b.Marshal(), bftBatchTxs)
		require.NoError(t, err)
		require.NoError(t, got.Verify(public, 7))
	}
	require.Equal(t, []byte("shell"), sent[0].Header)
	require.Empty(t, sent[0].Hashes)
	require.Len(t, sent[1].Hashes, bftBatchTxs)
	require.Equal(t, [][]byte{testTxHash(bftBatchTxs), testTxHash(bftBatchTxs + 1), testTxHash(bftBatchTxs + 2)}, sent[2].Hashes)
	require.Len(t, bodies.streamed[bftRound{5, 1}].txs, bftBatchTxs+3)

	// the bodies of a round the node streamed nothing of come from the queue
	queue := func(hashes [][]byte) [][]byte { return make([][]byte, len(hashes)) }
	require.Equal(t, [][]byte{nil}, bodies.streamedBodies(5, 2, [][]byte{testTxHash(0)}, queue))
	bodies.prune(6)
	require.Empty(t, bodies.streamed)

	sent = nil
	off := &bftStream{send: func(b *bft.Batch) { sent = append(sent, b) }, key: private, kept: bodies.stream(6, 0)}
	off.Header(make([]byte, bft.MaxBatchHeader+1))
	off.commit(testTxHash(0), nil)
	off.flush()
	require.Empty(t, sent, "a stream with a shell above the bound sends nothing")
}

// testSpeculation records what a speculation of the speculator ran and how it ended
type testSpeculation struct {
	mu       sync.Mutex
	shell    string
	ran      [][]byte
	finished bool
	aborted  bool
}

func (s *testSpeculation) Add(txs []*transaction.Transaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range txs {
		s.ran = append(s.ran, t.FullData)
	}
}

func (s *testSpeculation) Finish([]byte, int64, bft.Hash) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finished = true
	return nil
}

func (s *testSpeculation) Abort() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aborted = true
}

func (s *testSpeculation) state() (ran int, finished, aborted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ran), s.finished, s.aborted
}

// testSpeculator is a speculator whose speculations are testSpeculation, whose queue holds the
// transactions but those named missing, and whose proposer has none
type testSpeculator struct {
	*bftSpeculator
	mu      sync.Mutex
	specs   []*testSpeculation
	missing map[string]bool
	fetched int
}

func newTestSpeculator(idle time.Duration) *testSpeculator {
	ts := &testSpeculator{missing: map[string]bool{}}
	ts.bftSpeculator = &bftSpeculator{
		logger: log.WithFields(log.Fields{}),
		begin: func(shell []byte, height int64) (bftSpeculation, error) {
			if string(shell) == "bad" {
				return nil, errors.New("a bad shell")
			}
			s := &testSpeculation{shell: string(shell)}
			ts.mu.Lock()
			ts.specs = append(ts.specs, s)
			ts.mu.Unlock()
			return s, nil
		},
		parse: func(bodies [][]byte) ([]*transaction.Transaction, error) {
			txs := make([]*transaction.Transaction, len(bodies))
			for i, b := range bodies {
				txs[i] = &transaction.Transaction{FullData: b}
			}
			return txs, nil
		},
		raw: func(hashes [][]byte) [][]byte {
			bodies := make([][]byte, len(hashes))
			ts.mu.Lock()
			defer ts.mu.Unlock()
			for i, h := range hashes {
				if !ts.missing[string(h)] {
					bodies[i] = h
				}
			}
			return bodies
		},
		fetch: func(_ *bft.Batch, _ []byte, hashes [][]byte) ([][]byte, error) {
			ts.mu.Lock()
			ts.fetched += len(hashes)
			ts.mu.Unlock()
			return nil, errors.New("the proposer is gone")
		},
		idle: func() time.Duration { return idle },
	}
	return ts
}

func (ts *testSpeculator) spec(t *testing.T, i int) *testSpeculation {
	t.Helper()
	var s *testSpeculation
	require.Eventually(t, func() bool {
		ts.mu.Lock()
		defer ts.mu.Unlock()
		if i < len(ts.specs) {
			s = ts.specs[i]
		}
		return s != nil
	}, time.Second, time.Millisecond)
	return s
}

func testBatch(round, validator, seq uint32, from, n int) *bft.Batch {
	b := &bft.Batch{Height: 5, Round: round, Validator: validator, Seq: seq}
	if seq == 0 {
		b.Header = []byte(fmt.Sprintf("shell %d", round))
	}
	for i := range n {
		b.Hashes = append(b.Hashes, testTxHash(from+i))
	}
	return b
}

// ran waits until the speculation ran n transactions
func ran(t *testing.T, s *testSpeculation, n int) {
	t.Helper()
	require.Eventually(t, func() bool { r, _, _ := s.state(); return r == n }, time.Second, time.Millisecond)
}

// The speculator runs the batches of a round in their order, stops at a lost batch, at one whose
// bodies it misses or at a validator not of the first batch, goes over to the first batch of a
// higher round and keeps to its round, and the proposal finishes the speculation of its height
func TestBftSpeculator(t *testing.T) {
	ts := newTestSpeculator(time.Hour)
	ts.take(testBatch(1, 2, 0, 0, 2), nil)
	s := ts.spec(t, 0)
	require.Equal(t, "shell 1", s.shell)
	ts.take(testBatch(1, 2, 1, 2, 3), nil)
	ts.take(testBatch(1, 3, 2, 5, 1), nil) // another validator
	ts.take(testBatch(1, 2, 2, 5, 1), nil)
	ran(t, s, 6)
	ts.take(testBatch(1, 2, 4, 6, 1), nil) // batch 3 lost
	ts.take(testBatch(1, 2, 3, 6, 1), nil)
	ran(t, s, 6)

	// a lower round or the same round again starts nothing
	ts.take(testBatch(0, 1, 0, 0, 1), nil)
	ts.take(testBatch(1, 2, 0, 0, 1), nil)
	require.Len(t, ts.specs, 1)

	ts.take(testBatch(2, 3, 0, 0, 0), nil)
	s2 := ts.spec(t, 1)
	require.Eventually(t, func() bool { _, _, a := s.state(); return a }, time.Second, time.Millisecond,
		"the speculation of a lower round is aborted")
	ts.mu.Lock()
	ts.missing[string(testTxHash(11))] = true
	ts.mu.Unlock()
	ts.take(testBatch(2, 3, 1, 10, 2), nil)
	ts.take(testBatch(2, 3, 2, 12, 2), nil)
	require.Eventually(t, func() bool { ts.mu.Lock(); defer ts.mu.Unlock(); return ts.fetched == 1 }, time.Second, time.Millisecond)
	ran(t, s2, 0)
	ts.take(testBatch(2, 3, 3, 14, 2), nil)
	time.Sleep(10 * time.Millisecond)
	ran(t, s2, 0)

	require.ErrorIs(t, ts.finish(6, bft.Hash{}, nil), block.ErrSpeculation, "a proposal of another height")
	require.Eventually(t, func() bool { _, _, a := s2.state(); return a }, time.Second, time.Millisecond)
	require.ErrorIs(t, ts.finish(5, bft.Hash{}, nil), block.ErrSpeculation, "no speculation")

	ts.take(testBatch(3, 0, 0, 0, 1), nil)
	s3 := ts.spec(t, 2)
	ran(t, s3, 1)
	require.NoError(t, ts.finish(5, bft.Hash{}, nil))
	_, finished, aborted := s3.state()
	require.True(t, finished)
	require.False(t, aborted)

	var none *bftSpeculator
	none.abort()
	require.ErrorIs(t, none.finish(5, bft.Hash{}, nil), block.ErrSpeculation)
}

// abort waits until the speculation is rolled back; an idle speculation rolls back by itself, and
// a speculation that cannot begin decides nothing
func TestBftSpeculatorEnds(t *testing.T) {
	ts := newTestSpeculator(time.Hour)
	ts.take(testBatch(0, 1, 0, 0, 1), nil)
	s := ts.spec(t, 0)
	ran(t, s, 1)
	ts.abort()
	_, _, aborted := s.state()
	require.True(t, aborted)

	idle := newTestSpeculator(20 * time.Millisecond)
	idle.take(testBatch(0, 1, 0, 0, 1), nil)
	s = idle.spec(t, 0)
	require.Eventually(t, func() bool { _, _, a := s.state(); return a }, time.Second, time.Millisecond)
	require.ErrorIs(t, idle.finish(5, bft.Hash{}, nil), block.ErrSpeculation)

	bad := newTestSpeculator(time.Hour)
	b := testBatch(0, 1, 0, 0, 0)
	b.Header = []byte("bad")
	bad.take(b, nil)
	bad.mu.Lock()
	c := bad.cur
	bad.mu.Unlock()
	<-c.exited
	require.ErrorIs(t, bad.finish(5, bft.Hash{}, nil), block.ErrSpeculation)
}
