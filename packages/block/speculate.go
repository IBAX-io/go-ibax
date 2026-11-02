/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/gogo/protobuf/proto"
)

// A streaming proposal (F18 design 4.2): the proposer sends the header of its block before its
// trial and the transactions the trial puts in the block in batches as it puts them; a validator
// runs them in a speculation while the proposer tries the block, and its trial of the proposal
// goes on from what the speculation ran when the proposal begins with the transactions of the
// batches under the header. The block is the same as without the batches: the speculation runs
// a prefix of the block in its order, as the trial of the block would, and checks it as Check
// would.

// Stream takes the parts of the block the proposer tries as it tries it
type Stream interface {
	// Header takes the shell of the block before its trial
	Header(shell []byte)
	// Commit takes each transaction the trial puts in the block, in their order
	Commit(t *transaction.Transaction)
}

// shell is the header of the block without what its trial sets, its signature, its hash and its
// rollbacks hash, and the rollbacks hash of the block before it as the block carries it
func (b *Block) shell() ([]byte, error) {
	h := *b.Header
	h.Sign, h.BlockHash, h.RollbacksHash = nil, nil, nil
	return proto.Marshal(&types.BlockData{Header: &h, PrevHeader: &types.BlockHeader{RollbacksHash: b.PrevRollbacksHash}})
}

// ErrSpeculation is a speculation that does not decide the proposal: the trial of the proposal
// runs from the start
var ErrSpeculation = errors.New("the speculation does not decide the proposal")

// Speculation runs the batches of a streaming proposal of a height ahead of the proposal. It holds
// writes from Speculate to Finish or Abort, which the node takes before it writes the database.
type Speculation struct {
	mu     sync.Mutex
	writes sync.Locker
	shell  []byte
	b      *Block
	dbTx   *sqldb.DbTransaction
	mem    memState
	run    *blockRun
	checks *txChecks
	// failed is the error of a transaction of the batches the block is not valid with, and
	// failedAt the number of the transactions it needs at the start of the block
	failed   error
	failedAt int
	// broken is a speculation that cannot go on and decides nothing
	broken bool
	done   bool
}

// Speculate begins the speculation of a streaming proposal at the height whose block has the
// shell. It locks writes; it fails, and leaves writes unlocked, for a header that is not of the
// height or not of the chain.
func Speculate(shell []byte, height int64, writes sync.Locker) (*Speculation, error) {
	data := &types.BlockData{}
	if err := proto.Unmarshal(shell, data); err != nil {
		return nil, fmt.Errorf("parsing the shell of a block: %w", err)
	}
	if data.Header == nil || data.PrevHeader == nil || len(data.TxFullData) > 0 {
		return nil, errors.New("a malformed shell of a block")
	}
	if data.Header.BlockId != height || height <= 1 {
		return nil, fmt.Errorf("the shell of a block %d at height %d", data.Header.BlockId, height)
	}
	if err := checkNetwork(data.Header); err != nil {
		return nil, err
	}
	b := &Block{BlockData: data, PrevRollbacksHash: data.PrevHeader.RollbacksHash}
	writes.Lock()
	s, err := b.speculate(writes)
	if err != nil {
		writes.Unlock()
		return nil, err
	}
	s.shell = shell
	return s, nil
}

func (b *Block) speculate(writes sync.Locker) (*Speculation, error) {
	var err error
	if b.PrevHeader, err = GetBlockHeaderFromBlockChain(b.Header.BlockId - 1); err != nil {
		return nil, err
	}
	if b.PrevHeader == nil {
		return nil, errors.New("block previous header nil")
	}
	if err := b.checkHeader(); err != nil {
		return nil, err
	}
	dbTx, mem, err := b.beginTrial()
	if err != nil {
		return nil, err
	}
	return &Speculation{writes: writes, b: b, dbTx: dbTx, mem: mem, run: b.beginRun(dbTx), checks: newTxChecks()}, nil
}

// ParseTransactions parses the bodies of transactions, in parallel
func ParseTransactions(bodies [][]byte) ([]*transaction.Transaction, error) {
	txs := make([]*transaction.Transaction, len(bodies))
	errs := make([]error, len(bodies))
	workers := min(runtime.GOMAXPROCS(0), len(bodies))
	next := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range next {
				txs[i], errs[i] = transaction.UnmarshallTransaction(bytes.NewBuffer(bodies[i]), true)
			}
		})
	}
	for i := range bodies {
		next <- i
	}
	close(next)
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return txs, nil
}

// Ran is the number of the transactions the speculation took
func (s *Speculation) Ran() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.b.Transactions)
}

// Add runs the transactions of a batch, which follow those it ran
func (s *Speculation) Add(txs []*transaction.Transaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done || s.broken || s.failed != nil || len(txs) == 0 {
		return
	}
	s.add(txs)
}

func (s *Speculation) add(txs []*transaction.Transaction) {
	b := s.b
	transaction.VerifyAll(txs)
	if err := s.checks.check(b, txs); err != nil {
		b.Transactions = append(b.Transactions, txs...)
		s.failed, s.failedAt = err, len(b.Transactions)
		return
	}
	b.Transactions = append(b.Transactions, txs...)
	if err := s.run.execute(); err != nil {
		if s.run.failedAt > 0 {
			s.failed, s.failedAt = err, s.run.failedAt
		} else {
			s.broken = true
		}
	}
}

// Finish is valid(v) of the proposal of the block at the height of the hash, as Validate is,
// from what the speculation ran: the trial of the block it holds as Trial does. ErrSpeculation
// is a proposal the speculation does not decide, which the node validates by Validate; the
// speculation is over either way.
func (s *Speculation) Finish(binData []byte, height int64, hash bft.Hash) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return ErrSpeculation
	}
	full, err := parseProposal(binData, height, hash)
	if err != nil {
		s.abort()
		return err
	}
	b := s.b
	n := len(b.Transactions)
	if s.failed != nil {
		n = s.failedAt
	}
	if shell, err := full.shell(); err != nil || !bytes.Equal(shell, s.shell) || s.broken || !prefixOf(b.Transactions, full.Transactions, n) {
		s.abort()
		return ErrSpeculation
	}
	if s.failed != nil {
		s.abort()
		return s.failed
	}
	b.Header.Sign, b.Header.BlockHash, b.Header.RollbacksHash = full.Header.Sign, full.Header.BlockHash, full.Header.RollbacksHash
	b.MerkleRoot, b.BinData, b.TxFullData = full.MerkleRoot, full.BinData, full.TxFullData
	b.SysUpdate = b.SysUpdate || full.SysUpdate
	if rest := full.Transactions[n:]; len(rest) > 0 {
		s.add(rest)
		if s.broken {
			s.abort()
			return ErrSpeculation
		}
		if s.failed != nil {
			s.abort()
			return s.failed
		}
	}
	if len(b.Transactions) == 0 {
		// a proposal without a batch runs nothing ahead: execute runs it, empty
		if err := s.run.execute(); err != nil {
			s.abort()
			return err
		}
	}
	if err := b.CheckSign(); err != nil {
		s.abort()
		return err
	}
	s.done = true
	defer s.writes.Unlock()
	return b.endTrial(s.dbTx, s.mem, s.run.end(nil))
}

// prefixOf reports whether the first n transactions of full are ran, in their order
func prefixOf(ran, full []*transaction.Transaction, n int) bool {
	if n > len(full) || n > len(ran) {
		return false
	}
	for i := range n {
		if !bytes.Equal(ran[i].Hash(), full[i].Hash()) {
			return false
		}
	}
	return true
}

// Abort rolls the speculation back and unlocks writes
func (s *Speculation) Abort() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.done {
		s.abort()
	}
}

func (s *Speculation) abort() {
	s.done = true
	defer s.writes.Unlock()
	b := s.b
	if s.run.state != nil {
		s.run.state.stop()
	}
	close(s.run.bad)
	b.prof = nil
	b.trial = false
	b.Notifications = nil
	if err := s.dbTx.Rollback(); err != nil {
		b.GetLogger().WithError(err).Error("rolling back a speculation")
	}
	if err := s.mem.restore(); err != nil {
		b.GetLogger().WithError(err).Error("restoring the state of the node after a speculation")
	}
}
