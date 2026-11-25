/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/faults"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"

	log "github.com/sirupsen/logrus"
)

const (
	// maxTimeWait is how long a proposer waits at most for its clock to pass the time of the last
	// block
	maxTimeWait = syspar.BftProposerWait
	// blockFrame is what a block takes besides the bodies of its transactions: its header, the
	// header of the block before and their framing; txFrame is the framing of a body
	blockFrame = 2048
	txFrame    = 8
)

// bftApp is the node the machine asks for a block and for its validity at a height. Neither
// executes a block: a proposal is valid by what its block carries and the chain committed before
// it, and the executor executes the block once the chain commits it (F18 direction A).
type bftApp struct {
	ctx    context.Context
	logger *log.Entry
	bodies *bftBodies
	exec   *bftExecutor
	// set is the validators set of the height of the machine
	set bft.Validators
	// roundStart is when the validators started a round, zero if the node does not know
	roundStart func(height uint64, round uint32) time.Time
	// pull pulls before the deadline the transactions of the validators that said they had some
	// at the height, and reports whether the pool took any
	pull func(height uint64, deadline time.Time) bool
	// delayed reports whether a delayed contract is due at the height
	delayed func(height int64) bool
}

// Value makes the block of the proposer (F18 design 3.2): it carries the execution hash of the
// block BftExecDepth before it, which the node waits for its executor to have, and its
// transactions are those of the pool that are valid at its time, in the order of the pool, up to
// what the slowest executor of the validators runs in a second (bftPace), max_tx_block and
// max_block_size. Its time is the clock in seconds and above the time of the last block, so a
// chain has at most one block a second; a proposer whose clock is not yet past the time of the
// last block waits for it, up to maxTimeWait (design 4.4).
//
// The proposal reaches the validators before they time out: the proposer makes it within
// syspar.GetBftProposeBudget from when they started the round (roundStart), or from when it
// started it if that is earlier. A proposer that finds no transaction for its block pulls, within
// the same deadline, those of the validators that said they had some at the height
// (bftPending). A block without transactions is made only if a delayed contract is due at the
// height, which its execution runs.
func (a bftApp) Value(height uint64, round uint32) (_ []byte, _ bft.Hash, err error) {
	// a proposer without a block proposes nothing and the round times out (startRound): the log
	// tells why, so that each propose timeout of a proposer that runs is explained
	defer func() {
		if err == nil {
			return
		}
		fields := log.Fields{"height": height, "round": round, "pool": transaction.TxPool.Len(), "error": err}
		if errors.Is(err, block.ErrEmptyBlock) {
			a.logger.WithFields(fields).Info("the proposer has no transaction for its block")
			return
		}
		fields["type"] = consts.BlockError
		a.logger.WithFields(fields).Warn("the proposer made no block")
	}()
	start := time.Now()
	started := start
	if a.roundStart != nil {
		if at := a.roundStart(height, round); !at.IsZero() && at.Before(started) {
			started = at
		}
	}
	metrics.BlockStage.WithLabelValues(metrics.KindPropose, metrics.StageLag).Observe(start.Sub(started).Seconds())
	deadline := started.Add(syspar.GetBftProposeBudget(round))
	h := int64(height)
	if err := a.exec.wait(a.ctx, h-consts.BftExecDepth, deadline); err != nil {
		return nil, bft.Hash{}, err
	}
	prev := &sqldb.BlockChain{}
	found, err := prev.Get(h - 1)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	if !found {
		return nil, bft.Hash{}, fmt.Errorf("block %d before height %d is not committed", h-1, height)
	}
	if wait := faults.Until(time.Unix(prev.Time+1, 0)); wait > 0 {
		time.Sleep(min(wait, maxTimeWait))
	}
	metrics.BlockStage.WithLabelValues(metrics.KindPropose, metrics.StageWait).Observe(time.Since(start).Seconds())
	bin, hash, err := a.propose(h, round, prev)
	if errors.Is(err, block.ErrEmptyBlock) && a.pull != nil && a.pull(height, deadline) {
		// the transactions of a validator that said it had some, which the pool of the node lost
		a.logger.WithFields(log.Fields{"height": height, "round": round, "pool": transaction.TxPool.Len()}).
			Info("the proposer pulled transactions for its block")
		bin, hash, err = a.propose(h, round, prev)
	}
	if err != nil {
		return nil, bft.Hash{}, err
	}
	return bin, hash, nil
}

// propose makes the block of Value after the committed block prev
func (a bftApp) propose(height int64, round uint32, prev *sqldb.BlockChain) ([]byte, bft.Hash, error) {
	position, ok := a.set.Index(syspar.GetNodePubKey())
	if !ok {
		return nil, bft.Hash{}, fmt.Errorf("the node is not a validator of height %d", height)
	}
	exec, err := block.ExecHashAt(height - consts.BftExecDepth)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	state, err := block.StateHashAt(height)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	collect := time.Now()
	ts := max(faults.Now().Unix(), prev.Time+1)
	bodies, txs := collectTxs(ts, a.exec.capacity(a.set, syspar.GetNodePubKey(), height-1), syspar.GetMaxBlockSize()-blockFrame)
	if len(txs) == 0 && (a.delayed == nil || !a.delayed(height)) {
		return nil, bft.Hash{}, block.ErrEmptyBlock
	}
	metrics.BlockStage.WithLabelValues(metrics.KindPropose, metrics.StageCollect).Observe(metrics.Since(collect))
	header := &types.BlockHeader{
		BlockId:       height,
		Timestamp:     ts,
		KeyId:         conf.Config.KeyID,
		NetworkId:     conf.Config.LocalConf.NetworkID,
		NodePosition:  int64(position),
		Version:       consts.BftBlockVersion,
		ConsensusMode: consts.HonorNodeMode,
		ExecHash:      faults.ExecHash(exec),
		StateHash:     state,
	}
	prevHeader := &types.BlockHeader{BlockId: prev.ID, BlockHash: prev.Hash}
	bin, err := sealBlock(header, prevHeader, bodies)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	// the framing of the bodies is an estimate: the bodies past max_block_size leave the block
	for limit := syspar.GetMaxBlockSize(); int64(len(bin)) > limit && len(bodies) > 0; {
		over, cut := int64(len(bin))-limit, len(bodies)
		for over > 0 && cut > 0 {
			cut--
			over -= int64(len(bodies[cut]) + txFrame)
		}
		bodies, txs = bodies[:cut], txs[:cut]
		if bin, err = sealBlock(header, prevHeader, bodies); err != nil {
			return nil, bft.Hash{}, err
		}
	}
	hash, err := bft.HashOf(header.BlockHash)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	compact, err := block.Compact(bin, txs)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	hashes := make([][]byte, len(txs))
	for i, t := range txs {
		hashes[i] = t.Hash()
	}
	a.bodies.put(uint64(height), hash, bin, compact, hashes, bodies)
	total, behind := time.Since(collect), time.Since(time.Unix(ts, 0))
	metrics.BlockStage.WithLabelValues(metrics.KindPropose, metrics.StageTotal).Observe(total.Seconds())
	metrics.BlockStage.WithLabelValues(metrics.KindPropose, metrics.StageBehind).Observe(behind.Seconds())
	a.logger.WithFields(log.Fields{"height": height, "round": round, "txs": len(txs), "bytes": len(bin), "took": total}).Debug("proposing")
	return bin, hash, nil
}

// sealBlock signs and marshals the block of the header after the header of the block before,
// of the compressed bodies of its transactions
func sealBlock(header, prev *types.BlockHeader, bodies [][]byte) ([]byte, error) {
	b := &types.BlockData{Header: header, PrevHeader: prev, TxFullData: slices.Clone(bodies)}
	return b.MarshallCompressed(syspar.GetNodePrivKey())
}

// collectTxs takes the transactions of a block of the time from the pool, in its order: at most
// count of them, of compressed bodies of at most maxBytes with their framing, each valid at the
// time, of a key and of a key in an ecosystem at most max_block_user_transactions, and a
// StopNetwork transaction alone. It returns the compressed bodies and the transactions. A
// transaction a committed block has is done and leaves the pool; one not valid is marked bad and
// its key counted for a ban.
func collectTxs(ts int64, count int, maxBytes int64) ([][]byte, []*transaction.Transaction) {
	pending := transaction.TxPool.Take(count)
	parsed := make([]*transaction.Transaction, len(pending))
	for i, p := range pending {
		parsed[i] = p.Tx
	}
	checked := transaction.CheckBatch(parsed, ts)
	limits := transaction.NewLimits(transaction.GetLetPreprocess())
	bodies := make([][]byte, 0, len(pending))
	txs := make([]*transaction.Transaction, 0, len(pending))
	var size int64
	for i, p := range pending {
		t := p.Tx
		if err := checked[i]; errors.Is(err, transaction.ErrDuplicatedTx) {
			transaction.TxPool.Remove([][]byte{t.Hash()})
			continue
		} else if err != nil {
			transaction.BadTxForBan(t.KeyID())
			transaction.MarkTransactionBad(t.Hash(), err.Error())
			continue
		}
		if p.Rate == transaction.RateStopNetwork {
			if len(txs) == 0 {
				return [][]byte{t.Compressed()}, []*transaction.Transaction{t}
			}
			continue
		}
		if err := limits.CheckLimit(t.Inner); errors.Is(err, transaction.ErrLimitStop) {
			break
		} else if errors.Is(err, transaction.ErrLimitSkip) {
			continue
		} else if err != nil {
			transaction.BadTxForBan(t.KeyID())
			transaction.MarkTransactionBad(t.Hash(), err.Error())
			continue
		}
		body := t.Compressed()
		if size += int64(len(body) + txFrame); size > maxBytes {
			break
		}
		bodies = append(bodies, body)
		txs = append(txs, t)
	}
	return bodies, txs
}

// Valid is valid(v) (F18 design 3.2): once the node executed the block whose execution hash the
// block carries, it checks the block by what it carries and the chain committed before it
func (a bftApp) Valid(height uint64, hash bft.Hash, bin []byte) bool {
	start := time.Now()
	defer func() {
		metrics.BlockStage.WithLabelValues(metrics.KindTrial, metrics.StageValid).Observe(time.Since(start).Seconds())
	}()
	propose, _, _ := syspar.GetBftTimeouts()
	err := a.exec.wait(a.ctx, int64(height)-consts.BftExecDepth, start.Add(propose))
	if err == nil {
		err = block.CheckProposal(bin, int64(height), hash)
	}
	switch {
	case errors.Is(err, block.ErrStale):
		a.logger.WithFields(log.Fields{"error": err, "height": height}).Debug("a proposal of a height decided")
	case err != nil:
		a.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err, "height": height}).Warn("an invalid proposal")
	}
	return err == nil
}

// Timely is block.Timely at the time the node took the proposal
func (a bftApp) Timely(p *bft.Proposal) bool {
	// the bounds are of what takes the system clock, the making and the way of the proposal; the
	// times of the block and of its receipt are of the clock of the node
	ahead, behind := syspar.GetBftTimeliness(p.Round)
	ahead, behind = faults.Span(ahead), faults.Span(behind)
	err := block.Timely(p.Block, p.Received, ahead, behind)
	if err != nil {
		a.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err, "height": p.Height, "round": p.Round}).Warn("an untimely proposal")
	}
	return err == nil
}

// delayedDue reports whether a delayed contract is due at the height
func delayedDue(logger *log.Entry, height int64) bool {
	delayed, err := sqldb.GetAllDelayedContractsForBlockID(nil, height)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting delayed contracts")
		return false
	}
	return len(delayed) > 0
}
