/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/faults"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/network/bftnet"
	"github.com/IBAX-io/go-ibax/packages/service/bftmon"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

// The consensus driver of a chain of the BFT consensus (F18 design 4.2, 4.3): it runs the machine
// of package bft at the height after the last block, gives it the messages the pool admitted and
// the timeouts it scheduled, stores the sign state before it sends a message the machine signed,
// and commits the block of a decision with its certificate. On such a chain BlockGenerator starts
// the driver and generates no block itself. The driver orders the blocks and executes none (F18
// direction A): a proposal is valid by what its block carries, and the executor (bftExecutor)
// executes the committed blocks after, under DBLock, which the driver never takes.
// Proposals travel with their blocks in the compact form (package block): the driver fills in the
// block of a proposal it takes from its transaction queue and the proposer, apart from the
// goroutine of run, and gives the proposal to the pool and the machine once it has the block.

const (
	// bftTick is how often the driver checks its peers, its height and the transactions
	bftTick = 500 * time.Millisecond
	// bftResend is how often the driver resends the messages of its height and gives them to the
	// machine again: the messages a partition or a full queue lost reach the others after it
	bftResend = 2 * time.Second
	// bftEvents is the messages and timeouts waiting for the driver; past it a message is
	// dropped from the queue, the resends make up for it
	bftEvents = 4096
	// bftReceived is the frames of transactions of the other nodes waiting for the pool; past it
	// a frame is dropped, the proposers give the bodies of their blocks
	bftReceived = 64
)

// bftNext is the event of a height a commit moved to: the machine starts it at once if there is
// work, not at the next tick, which would hold a chain under load to a block a tick
type bftNext struct{}

// bftWork is the event of the transactions the node pulled at a height from a validator that said
// it had some (bftPending): the machine starts the height at once, its round 0 from then
type bftWork struct {
	height uint64
	at     time.Time
}

// bftExpired is the event of a timeout the machine scheduled, due when it was to expire: the
// driver takes it later if it was busy, trying a block under DBLock
type bftExpired struct {
	bft.Timeout
	due time.Time
}

// bftRoundStart is when the validators started a round of a height, as far as the node knows: the
// time a quorum of precommits of the height before reached it, for round 0 of a height it moved to
// with work, and the time the precommit timeout of the round before was due, for a later round.
// The proposer of the round makes its proposal within syspar.GetBftProposeBudget from then.
type bftRoundStart struct {
	height uint64
	round  uint32
	at     time.Time
}

var bftStart sync.Once

// startBftConsensus starts the driver once
func startBftConsensus(ctx context.Context, logger *log.Entry) {
	bftStart.Do(func() {
		if crypto.HashSize() != bft.HashSize {
			log.WithFields(log.Fields{"type": consts.CryptoError, "size": crypto.HashSize()}).
				Fatal("the BFT consensus needs a hash of 32 bytes")
		}
		startBftExecutor(ctx, logger)
		d := newBftDriver(ctx, logger)
		go d.run()
	})
}

type bftDriver struct {
	ctx       context.Context
	logger    *log.Entry
	networkID uint64
	svc       *bftnet.Service
	events    chan any // *bft.Proposal, *bft.Vote, bftExpired, bftNext, bftWork
	received  chan [][]byte
	// pending gets the node without transactions those of a validator that has some
	pending *bftPending

	mu   sync.Mutex // guards pool, which the handler of the service also uses
	pool *bft.Pool
	// bodies is the blocks of the proposals the node made or filled in
	bodies *bftBodies

	// m is used only by the goroutine of run, and set is the validators set of its height, which
	// the executor and the service read for the execution statuses
	m   *bft.Machine
	set atomic.Pointer[bft.Validators]
	// roundStart is used only by the goroutine of run
	roundStart bftRoundStart
	// held is the propose timeout of a round without work the node holds (idle), and work is
	// hasWork, the pool and the delayed contracts, unless a test sets it; used only by the
	// goroutine of run
	held *bft.Timeout
	work func() bool
	// unsaved is a sign state the node could not store, and unsent the messages signed with it:
	// the driver sends them once it stores the state
	unsaved *bft.SignState
	unsent  []bft.Output
	resent  time.Time

	// store stores a sign state and transmit sends a message the machine signed: saveUnsaved
	// calls transmit only after store stored the state that has the message
	store    func(*bft.SignState) error
	transmit func(bft.Output)
	// signFile is the file store writes the sign state to, which restore opens
	signFile *bft.SignFile

	// evidence is the evidence the driver took, and submit submits one in a transaction
	evidence bftEvidence
	submit   func([]byte) error

	// watch is the state of the alerts, and clockDrift measures the clock against a time source
	watch      bftWatch
	clockDrift func(server string) (time.Duration, error)
}

func newBftDriver(ctx context.Context, logger *log.Entry) *bftDriver {
	d := &bftDriver{
		ctx:       ctx,
		logger:    logger.WithFields(log.Fields{"consensus": "bft"}),
		networkID: uint64(conf.Config.LocalConf.NetworkID),
		events:    make(chan any, bftEvents),
		received:  make(chan [][]byte, bftReceived),
		bodies:    newBftBodies(),
	}
	cfg := bftnet.DefaultConfig
	cfg.Rate = syspar.GetBftPeerMsgRate()
	d.svc = bftnet.NewService(cfg, d)
	d.pending = &bftPending{
		poolLen: transaction.TxPool.Len,
		fetch:   fetchPending,
		receive: func(bodies [][]byte) int {
			taken := d.takeTxs(bodies)
			bftmon.Add(bftmon.PendingPulled, taken)
			return taken
		},
		work:    func(height uint64, at time.Time) { d.post(bftWork{height: height, at: at}) },
		say:     d.svc.SendPending,
		timeout: bftnet.DefaultConfig.Dial,
		now:     time.Now,
	}
	d.store = func(s *bft.SignState) error {
		return d.signFile.Write(s)
	}
	d.submit = submitEvidenceTx
	d.clockDrift = utils.ClockDrift
	d.transmit = func(o bft.Output) {
		if o.Proposal != nil {
			d.sendProposal(o.Proposal)
		} else {
			d.svc.SendVote(o.Vote)
		}
	}
	return d
}

func (d *bftDriver) run() {
	syspar.SetRunModel(consts.HonorNodeMode)
	bftExec.setNotify(func(height int64, rate float64) {
		d.post(bftNext{})
		d.sendExecuted(height, rate)
	})
	err := d.reset()
	for errors.Is(err, block.ErrNotExecuted) {
		// a node that committed blocks ahead of its execution starts once it executed them
		d.logger.WithFields(log.Fields{"error": err}).Info("the consensus waits for the executor")
		err = d.reset()
	}
	if err == nil {
		err = d.restore()
	}
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Fatal("starting the consensus")
	}
	go d.receiveTxs()
	bftnet.Register(d.svc)
	defer func() {
		bftnet.Register(nil)
		d.svc.Close()
		d.signFile.Close()
	}()
	d.logger.WithFields(log.Fields{"height": d.m.Height()}).Info("the consensus started")
	d.checkOperator()

	tick := time.NewTicker(bftTick)
	defer tick.Stop()
	for {
		select {
		case <-d.ctx.Done():
			return
		case e := <-d.events:
			d.event(e)
		case <-tick.C:
			d.tick()
		}
	}
}

// checkOperator logs an error if the KeyID of the node is not the key_id of its node in
// honor_nodes: the fees of the blocks it proposes go to its KeyID, and with no account there
// every transaction of its blocks fails
func (d *bftDriver) checkOperator() {
	operators, err := transaction.BftOperators(syspar.SysString(syspar.HonorNodes))
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.ConfigError, "error": err}).Error("reading the operators of the validators")
		return
	}
	for i, n := range syspar.GetNodes() {
		if bytes.Equal(n.PublicKey, syspar.GetNodePubKey()) && i < len(operators) && operators[i] != conf.Config.KeyID {
			d.logger.WithFields(log.Fields{"type": consts.ConfigError, "key_id": conf.Config.KeyID, "operator": operators[i]}).
				Error("the KeyID of this node is not the key_id of its validator: the fees of its blocks go to an account it does not operate")
		}
	}
}

// config is the machine configuration of the current platform parameters
func (d *bftDriver) config() bft.Config {
	propose, prevote, precommit := syspar.GetBftTimeouts()
	return bft.Config{
		Propose: propose, Prevote: prevote, Precommit: precommit,
		Delta:           syspar.BftTimeoutDelta,
		MaxFutureRounds: syspar.GetBftMaxFutureRounds(),
	}
}

// validatorsAt is the stored validators set of a height, as the executed blocks left it: of a
// height above the executed one and BftSetDelay, knownValidators
func validatorsAt(height uint64) (bft.Validators, error) {
	s := &sqldb.BftValidatorSet{}
	found, err := s.GetAt(nil, int64(height))
	if err != nil {
		return bft.Validators{}, err
	}
	if !found {
		return bft.Validators{}, fmt.Errorf("no validators set of height %d", height)
	}
	return bft.UnmarshalValidators(s.Data)
}

func (d *bftDriver) newMachine(height uint64, set bft.Validators) {
	app := bftApp{ctx: d.ctx, logger: d.logger, bodies: d.bodies, exec: bftExec, set: set, roundStart: d.roundStartOf,
		pull: d.pending.pullNow, delayed: func(height int64) bool { return delayedDue(d.logger, height) }}
	d.set.Store(&set)
	d.m = bft.NewMachine(d.config(), app, syspar.GetNodePrivKey(), syspar.GetNodePubKey(), d.networkID, height, set)
}

// reset starts the pool and the machine at the height after the last committed block, once the
// executor executed the block that stores its validators set
func (d *bftDriver) reset() error {
	last, err := sqldb.CommittedHeight()
	if err != nil {
		return err
	}
	if err := bftExec.wait(d.ctx, last+1-consts.BftSetDelay, time.Now().Add(bftSyncWait)); err != nil {
		return err
	}
	set, err := knownValidators(uint64(last) + 1)
	if err != nil {
		return err
	}
	next := uint64(last) + 1
	d.mu.Lock()
	d.pool = bft.NewPool(d.networkID, syspar.GetBftMaxFutureRounds(), next, set)
	d.mu.Unlock()
	d.newMachine(next, set)
	d.watch.moveTo(next, set, time.Now(), false)
	d.pending.moveTo(next, set, syspar.GetNodePubKey())
	return nil
}

// signStatePath is the file of the sign state of the node
func signStatePath() string {
	return filepath.Join(conf.Config.DirPathConf.DataDir, bft.SignStateFile)
}

// restore restores the sign state the node stored at its height before it stopped (design 4.3):
// the machine signs nothing that conflicts with it, and the pool resends its votes. The proposals
// of the state have no blocks and are not resent: the validators took them before the node
// stored the state, and their blocks are in the proposals of the hashes the network sends
func (d *bftDriver) restore() error {
	f, s, err := bft.OpenSignFile(signStatePath())
	if err != nil {
		return fmt.Errorf("the stored sign state: %w", err)
	}
	d.signFile = f
	if s == nil || s.Height != d.m.Height() {
		return nil
	}
	d.mu.Lock()
	for _, v := range s.Votes {
		d.pool.AddVote(v)
	}
	d.mu.Unlock()
	d.logger.WithFields(log.Fields{"height": s.Height, "round": s.Round}).Info("restored the sign state")
	d.apply(d.m.Restore(s))
	return nil
}

// syncHeight moves the machine to the height after the last committed block if a block was
// committed without it: by the synchronization, or by a commit of this driver. A height whose
// validators set the executor has not stored yet waits for it (errSetUnknown): the executor
// posts bftNext after each block.
func (d *bftDriver) syncHeight() error {
	committed, err := sqldb.CommittedHeight()
	if err != nil {
		return err
	}
	last := uint64(committed)
	if last+1 == d.m.Height() {
		return nil
	}
	if last+1 < d.m.Height() {
		return fmt.Errorf("the last block %d is below the height %d of the consensus", last, d.m.Height())
	}
	set, err := knownValidators(last + 1)
	if err != nil {
		return err
	}
	d.unsaved, d.unsent = nil, nil
	// the watch moves first: the handler records the precommits the pool admits for the new height
	from, now := d.m.Height(), time.Now()
	d.watch.moveTo(last+1, set, now, d.m.Decided())
	d.mu.Lock()
	var kept []any
	if d.pool.Height() == last {
		cert := &sqldb.BftCertificate{}
		if _, err := cert.Get(nil, int64(last)); err != nil {
			d.mu.Unlock()
			return err
		}
		kept = d.pool.Advance(uint32(cert.Round), set)
	} else {
		d.pool = bft.NewPool(d.networkID, syspar.GetBftMaxFutureRounds(), last+1, set)
	}
	d.mu.Unlock()
	d.bodies.prune(last)
	d.newMachine(last+1, set)
	d.pending.moveTo(last+1, set, syspar.GetNodePubKey())
	d.roundStart = bftRoundStart{}
	if decided := d.watch.decided(last); !decided.IsZero() && d.hasWork() {
		// the validators of the quorum that reached the node then start the height at once, as
		// they have work too under load: an idle height starts later, from when it has work
		d.roundStart = bftRoundStart{height: last + 1, at: decided}
	}
	d.logger.WithFields(log.Fields{"height": last + 1, "validators": set.N()}).Debug("moved to the height")
	d.onHeight(from, last+1, set, now)
	for _, m := range kept {
		if v, ok := m.(*bft.Vote); ok {
			d.watch.precommit(v, now)
		}
		d.give(m)
	}
	return nil
}

// bftStopped reports whether the network is stopped (StopNetwork): the node then signs nothing.
// The pause of a node behind is no stop: it takes the blocks of the others by their certificates,
// and its messages of a height the others left change nothing; a node that reaches none of the
// others is not paused (node.unreachableRelevant).
func bftStopped() bool {
	return node.NodePauseType() == node.PauseTypeStopingNetwork
}

// give gives an admitted message to the machine
func (d *bftDriver) give(m any) {
	switch m := m.(type) {
	case *bft.Proposal:
		d.apply(d.m.OnProposal(m))
	case *bft.Vote:
		d.apply(d.m.OnVote(m))
	}
}

func (d *bftDriver) event(e any) {
	if bftStopped() {
		return
	}
	if !d.moveHeight() {
		return
	}
	switch e := e.(type) {
	case bftExpired:
		d.expired(e)
	case bftNext:
		d.startWork()
	case bftWork:
		if e.height == d.m.Height() && d.roundStart.height != e.height {
			// the validator that had the transactions started the round then
			d.roundStart = bftRoundStart{height: e.height, at: e.at}
		}
		d.startWork()
	default:
		d.give(e)
	}
}

// moveHeight is syncHeight, and reports whether the machine is at the height after the last
// committed block
func (d *bftDriver) moveHeight() bool {
	err := d.syncHeight()
	switch {
	case err == nil:
		return true
	case errors.Is(err, errSetUnknown):
		d.logger.WithFields(log.Fields{"error": err}).Debug("the next height waits for the executor")
	default:
		d.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("moving to the height")
	}
	return false
}

func (d *bftDriver) tick() {
	d.svc.SetPeers(bftPeers(d.logger))
	d.saveUnsaved()
	if bftStopped() {
		return
	}
	d.submitEvidence()
	now := time.Now()
	d.checkOverdue(now)
	d.checkClock(now)
	if !d.moveHeight() {
		return
	}
	busy := d.startWork()
	d.checkLiveness(now, busy, d.m.Round(), d.silent)
	if time.Since(d.resent) < bftResend {
		return
	}
	d.resent = time.Now()
	d.mu.Lock()
	msgs := d.pool.Messages(d.m.Height())
	d.mu.Unlock()
	for _, m := range msgs {
		d.send(m)
		d.give(m)
	}
}

// startWork starts the height if it is undecided and has work, says so to the others if the
// pool has transactions, and reports whether it has work
func (d *bftDriver) startWork() bool {
	if d.m.Decided() || !d.hasWork() {
		return false
	}
	if transaction.TxPool.Len() > 0 {
		d.pending.announce(d.m.Height())
	}
	d.apply(d.m.Start())
	d.resume()
	return true
}

// hasWork reports whether the height has a transaction to put in a block: a transaction of the
// pool a block may take, or a delayed contract due at the height. A chain without one has no
// round.
func (d *bftDriver) hasWork() bool {
	if d.work != nil {
		return d.work()
	}
	return transaction.TxPool.Live(faults.Now().Unix()) || delayedDue(d.logger, int64(d.m.Height()))
}

func (d *bftDriver) send(m any) {
	switch m := m.(type) {
	case *bft.Proposal:
		d.sendProposal(m)
	case *bft.Vote:
		d.svc.SendVote(m)
	}
}

func (d *bftDriver) sendProposal(p *bft.Proposal) {
	if err := d.svc.SendProposal(p); err != nil {
		d.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err, "height": p.Height, "round": p.Round}).
			Error("the compact form of a proposal")
	}
}

// apply does what the machine returned: it stores the sign state, then puts the messages the
// machine signed in the pool and sends them, schedules the timeouts, takes and sends the evidence and
// commits a decision. A message is sent only after the state that has it is stored.
func (d *bftDriver) apply(out []bft.Output) {
	defer func() {
		d.mu.Lock()
		d.pool.SetRound(d.m.Round())
		d.mu.Unlock()
	}()
	if s := d.m.TakeSignState(); s != nil {
		d.unsaved = s
	}
	var commit *bft.Commit
	for _, o := range out {
		switch {
		case o.Proposal != nil, o.Vote != nil:
			d.unsent = append(d.unsent, o)
		case o.Timeout != nil:
			d.schedule(*o.Timeout)
		case o.Evidence != nil:
			if body := o.Evidence.Marshal(); d.takeEvidence(body) {
				d.svc.SendEvidence(body)
			}
		case o.Commit != nil:
			commit = o.Commit
		}
	}
	d.saveUnsaved()
	if commit != nil {
		d.commit(commit)
	}
}

// schedule posts the timeout as an event once it expires
func (d *bftDriver) schedule(timeout bft.Timeout) {
	t := bftExpired{Timeout: timeout, due: time.Now().Add(timeout.After)}
	time.AfterFunc(t.After, func() {
		select {
		case d.events <- t:
		case <-d.ctx.Done():
		}
	})
}

// slowSignState is a store of the sign state the node warns of: the messages it signs wait for it
const slowSignState = 200 * time.Millisecond

// saveUnsaved stores the sign state not stored yet and sends the messages signed with it
func (d *bftDriver) saveUnsaved() {
	if d.unsaved != nil {
		s, start := d.unsaved, time.Now()
		if err := d.store(s); err != nil {
			d.logger.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("storing the sign state, the messages wait")
			return
		}
		if took := time.Since(start); took > slowSignState {
			d.logger.WithFields(log.Fields{"height": s.Height, "round": s.Round, "took": took}).Warn("storing the sign state held the messages")
		}
		d.unsaved = nil
	}
	for _, o := range d.unsent {
		d.mu.Lock()
		if o.Proposal != nil {
			d.pool.AddProposal(o.Proposal)
		} else {
			d.pool.AddVote(o.Vote)
		}
		d.mu.Unlock()
		if o.Vote != nil {
			d.watch.precommit(o.Vote, time.Now())
		}
		d.transmit(o)
	}
	d.unsent = nil
}

// commit commits the block of a decision with its certificate, unless the synchronization
// committed the height first; a commit that fails leaves the height to the synchronization
func (d *bftDriver) commit(c *bft.Commit) {
	if !commitDecision(d.logger, c) {
		return
	}
	if err := d.syncHeight(); err != nil {
		d.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("moving to the next height")
		return
	}
	// posted, not started here: a machine of one validator decides in Start, and its commit would
	// start the next height within this one
	d.post(bftNext{})
}

// commitDecision commits the block of the decision if the last committed block is below its
// height, and reports whether the node has the height committed
func commitDecision(logger *log.Entry, c *bft.Commit) bool {
	committed, err := commitCertified(c.Block, bft.CertificateOf(c))
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.BlockError, "error": err, "height": c.Height}).Error("committing the decision")
		return false
	}
	if !committed {
		return true
	}
	logger.WithFields(log.Fields{"height": c.Height, "round": c.Round, "hash": fmt.Sprintf("%x", c.Hash[:8])}).Info("committed")
	return true
}

// post queues a message for the driver, and drops it if the queue is full
func (d *bftDriver) post(m any) {
	select {
	case d.events <- m:
	default:
	}
}

// countAdmission counts a message the pool dropped as dropped or duplicate, and reports whether
// it kept it
func countAdmission(a bft.Admission) bool {
	switch {
	case a == bft.Duplicate:
		bftmon.Count(bftmon.MessagesDuplicate)
	case !a.Kept():
		bftmon.Count(bftmon.MessagesDropped)
	}
	return a.Kept()
}

// Proposal is bftnet.Handler.Proposal: a proposal the pool would admit is new, and the driver
// fills in its block apart; it gives the proposal to the pool and the machine once it has it
func (d *bftDriver) Proposal(p *bft.Proposal) bool {
	p.Received = faults.Now()
	d.mu.Lock()
	a, key := d.pool.Admissible(p)
	d.mu.Unlock()
	if !countAdmission(a) {
		return false
	}
	k := bftProposalKey{p.Height, p.Round, p.Validator, p.Hash}
	if !d.bodies.start(k) {
		bftmon.Count(bftmon.MessagesDuplicate)
		return false
	}
	go d.expand(p, key, k)
	return true
}

// expand fills in the block of a proposal from the transaction queue and the proposer, and gives
// the proposal to the pool and the machine
func (d *bftDriver) expand(p *bft.Proposal, key []byte, k bftProposalKey) {
	defer d.bodies.done(k)
	compact := p.Block
	start := time.Now()
	if b := d.bodies.get(p.Hash); b != nil && bytes.Equal(b.compact, compact) {
		p.Block = b.block
	} else {
		bin, bodies, err := block.Expand(compact, syspar.GetMaxBlockSize(), transaction.TxPool.Stored,
			func(hashes [][]byte) ([][]byte, error) {
				addr, ok := syspar.GetNodeTCPAddress(key)
				if !ok {
					return nil, fmt.Errorf("no address of validator %d", p.Validator)
				}
				return bftnet.FetchTransactions(addr, p.Hash, hashes, syspar.GetMaxBlockSize(), bftnet.DefaultConfig.Dial)
			})
		if err != nil {
			bftmon.Count(bftmon.MessagesDropped)
			d.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err, "height": p.Height, "round": p.Round}).
				Warn("filling in the block of a proposal")
			return
		}
		hashes, _ := block.CompactHashes(compact)
		d.bodies.put(p.Height, p.Hash, bin, compact, hashes, bodies)
		p.Block = bin
	}
	metrics.BlockStage.WithLabelValues(metrics.KindTrial, metrics.StageExpand).Observe(metrics.Since(start))
	d.mu.Lock()
	a := d.pool.AddProposal(p)
	d.mu.Unlock()
	if !countAdmission(a) {
		return
	}
	bftmon.Count(bftmon.MessagesAdmitted)
	d.post(p)
}

// Compact is bftnet.Handler.Compact
func (d *bftDriver) Compact(p *bft.Proposal) ([]byte, error) {
	return d.bodies.compact(p.Height, p.Hash, p.Block)
}

// Transactions is bftnet.Handler.Transactions: the bodies of the blocks the node made or filled
// in, and those of its queue
func (d *bftDriver) Transactions(hash bft.Hash, hashes [][]byte) [][]byte {
	return d.bodies.transactions(hash, hashes, transaction.TxPool.Stored)
}

// MaxBlockTxs is bftnet.Handler.MaxBlockTxs
func (d *bftDriver) MaxBlockTxs() int { return syspar.GetMaxTxCount() }

// Vote is bftnet.Handler.Vote
func (d *bftDriver) Vote(v *bft.Vote) bool {
	d.mu.Lock()
	a := d.pool.AddVote(v)
	d.mu.Unlock()
	if !countAdmission(a) {
		return false
	}
	bftmon.Count(bftmon.MessagesAdmitted)
	d.watch.precommit(v, time.Now())
	d.post(v)
	return true
}

// Txs is bftnet.Handler.Txs: the pool takes the transactions apart from the stream, which carries
// the votes too
func (d *bftDriver) Txs(bodies [][]byte) {
	select {
	case d.received <- bodies:
	default:
		bftmon.Count(bftmon.TxFramesDropped)
	}
}

// receiveTxs puts the transactions of the other nodes in the pool as sent: each node sends its
// own to all the others
func (d *bftDriver) receiveTxs() {
	for {
		select {
		case <-d.ctx.Done():
			return
		case bodies := <-d.received:
			d.takeTxs(bodies)
		}
	}
}

// takeTxs puts transactions of another node in the pool as sent, and is the count it put
func (d *bftDriver) takeTxs(bodies [][]byte) int {
	maxSize := syspar.GetMaxTxSize()
	bodies = slices.DeleteFunc(bodies, func(b []byte) bool { return int64(len(b)) > maxSize })
	taken, full, err := transaction.TxPool.Receive(bodies, true)
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.ProtocolError, "error": err}).Warn("taking the transactions of a node")
	}
	bftmon.Add(bftmon.PoolFullTxs, full)
	return taken
}

// Pending is bftnet.Handler.Pending
func (d *bftDriver) Pending(height uint64, validator uint32) {
	if !bftStopped() {
		d.pending.hint(height, validator)
	}
}

// sendExecuted sends the execution status of the node, a validator of the set of the machine,
// after it executed the block of the height (bftPace)
func (d *bftDriver) sendExecuted(height int64, rate float64) {
	set := d.set.Load()
	if set == nil || bftStopped() {
		return
	}
	index, ok := set.Index(syspar.GetNodePubKey())
	if !ok {
		return
	}
	s := &bft.ExecStatus{Validator: index, Height: uint64(height), Rate: uint32(min(rate, math.MaxUint32))}
	if err := s.Sign(syspar.GetNodePrivKey(), d.networkID); err != nil {
		d.logger.WithFields(log.Fields{"type": consts.CryptoError, "height": height, "error": err}).Error("signing the execution status")
		return
	}
	d.svc.SendExecuted(s)
}

// Executed is bftnet.Handler.Executed: the pace takes the execution status of another validator
// of the set of the machine that is newer than the one it has and verifies
func (d *bftDriver) Executed(s *bft.ExecStatus) {
	set := d.set.Load()
	if set == nil || bftStopped() {
		return
	}
	key, ok := set.Key(s.Validator)
	if !ok || bytes.Equal(key, syspar.GetNodePubKey()) || !bftExec.pace.newer(key, int64(s.Height)) {
		return
	}
	if s.Verify(key, d.networkID) != nil {
		bftmon.Count(bftmon.MessagesDropped)
		return
	}
	bftExec.pace.take(key, int64(s.Height), float64(s.Rate), time.Now())
}

// PendingTxs is bftnet.Handler.PendingTxs: the first transactions of the pool
func (d *bftDriver) PendingTxs(count int, maxBytes int64) [][]byte {
	return transaction.TxPool.Head(count, maxBytes)
}

// fetchPending takes the first transactions of the pool of the validator of the key
func fetchPending(key []byte, timeout time.Duration) ([][]byte, error) {
	addr, ok := syspar.GetNodeTCPAddress(key)
	if !ok {
		return nil, errNoValidator
	}
	return bftnet.FetchPending(addr, syspar.GetMaxTxCount(), syspar.GetMaxBlockSize(), timeout)
}

// Evidence is bftnet.Handler.Evidence
func (d *bftDriver) Evidence(body []byte) bool { return d.takeEvidence(body) }

// Certificates is bftnet.Handler.Certificates
func (d *bftDriver) Certificates(from uint64, count int) ([][]byte, error) {
	var certs [][]byte
	for h := from; len(certs) < count; h++ {
		c := &sqldb.BftCertificate{}
		found, err := c.GetArchived(int64(h))
		if err != nil {
			return nil, err
		}
		if !found {
			break
		}
		certs = append(certs, c.Data)
	}
	return certs, nil
}

// MaxBlockSize is bftnet.Handler.MaxBlockSize
func (d *bftDriver) MaxBlockSize() int64 { return syspar.GetMaxBlockSize() }

// expired takes a timeout of the machine that expired. The precommit timeout of the current round
// starts the next round (L65) at the validators as it expires, which the driver keeps as the start of
// that round.
func (d *bftDriver) expired(e bftExpired) {
	if d.idle(e.Timeout) {
		t := e.Timeout
		d.held = &t
		return
	}
	d.countTimeout(e.Timeout)
	if e.Step == bft.StepPrecommit && e.Height == d.m.Height() && e.Round == d.m.Round() {
		d.roundStart = bftRoundStart{height: e.Height, round: e.Round + 1, at: e.due}
	}
	d.apply(d.m.OnTimeout(e.Timeout))
}

// idle reports whether the propose timeout finds the round of the machine with nothing to wait
// for: no proposal admitted and no transaction to put in one,
// as a validator with work started the height and the node has none, or a block took the work.
// A round without work runs no timer and counts no timeout: the node holds the timeout, without
// its nil prevote, until work comes (resume), or a proposal or the votes of the others move the
// machine on. It takes back no vote, so the safety is that of the machine.
func (d *bftDriver) idle(t bft.Timeout) bool {
	return t.Step == bft.StepPropose && !d.m.Decided() && t.Height == d.m.Height() && t.Round == d.m.Round() &&
		d.m.Step() == bft.StepPropose && !d.hasProposal(t.Height, t.Round) && !d.hasWork()
}

// resume runs again the propose timeout the node held, the full timeout from now, if the machine
// is still at its round: the proposer may yet propose with the work that came
func (d *bftDriver) resume() {
	t := d.held
	d.held = nil
	if t != nil && !d.m.Decided() && t.Height == d.m.Height() && t.Round == d.m.Round() && d.m.Step() == bft.StepPropose {
		d.schedule(*t)
	}
}

// roundStartOf is roundStart if it is of the height and the round, else zero
func (d *bftDriver) roundStartOf(height uint64, round uint32) time.Time {
	if s := d.roundStart; s.height == height && s.round == round {
		return s.at
	}
	return time.Time{}
}
