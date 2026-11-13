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
	"path/filepath"
	"slices"
	"sync"
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
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

// The consensus driver of a chain of the BFT consensus (F18 design 4.2, 4.3): it runs the machine
// of package bft at the height after the last block, gives it the messages the pool admitted and
// the timeouts it scheduled, stores the sign state before it sends a message the machine signed,
// and commits the block of a decision with its certificate. On such a chain BlockGenerator starts
// the driver and generates no block itself. DBLock guards only the execution and the writing of
// a block (plan 5.9): the proposer's trial of its block (Value), a validator's trial of a proposal
// (Valid) and the commit of a decision. The machine, the messages and the verification of
// signatures run without it, so the synchronization writing a block holds up none of them.
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

// bftWritten is the event of the end of the writes of a block the node proposed, which its trial
// goes on with after the proposal (block.Written): the machine prevotes the block only once they
// succeed
type bftWritten struct {
	height uint64
	hash   bft.Hash
	err    error
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
		d := newBftDriver(ctx, logger)
		go d.run()
	})
}

type bftDriver struct {
	ctx       context.Context
	logger    *log.Entry
	networkID uint64
	svc       *bftnet.Service
	events    chan any // *bft.Proposal, *bft.Vote, bftExpired, bftNext, bftWork, bftWritten
	received  chan [][]byte
	// pending gets the node without transactions those of a validator that has some
	pending *bftPending

	mu   sync.Mutex // guards pool, which the handler of the service also uses
	pool *bft.Pool
	// bodies is the blocks of the proposals the node made or filled in
	bodies *bftBodies
	// spec runs the batches of the streaming proposals of the others
	spec *bftSpeculator

	// m is used only by the goroutine of run
	m *bft.Machine
	// roundStart is used only by the goroutine of run
	roundStart bftRoundStart
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
	d.spec = &bftSpeculator{
		logger: d.logger,
		begin:  speculate,
		parse:  block.ParseTransactions,
		raw:    transaction.TxPool.Raw,
		fetch:  fetchStreamed,
		idle: func() time.Duration {
			propose, _, _ := syspar.GetBftTimeouts()
			return propose
		},
	}
	speculations = d.spec
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
	// the machine starts at the height after the last block under DBLock; the restored sign state
	// runs without it, as the machine does, since it may try a block (Value, Valid), which takes it
	lockWrites()
	err := d.reset()
	DBUnlock()
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

// validatorsAt is the stored validators set of a height
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

// lastBlock is the height of the last block
func lastBlock() (uint64, error) {
	info := &sqldb.InfoBlock{}
	found, err := info.Get()
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, errors.New("info block not found")
	}
	return uint64(info.BlockID), nil
}

func (d *bftDriver) newMachine(height uint64, set bft.Validators) {
	app := bftApp{logger: d.logger, bodies: d.bodies, spec: d.spec, stream: func(height uint64, round uint32) *bftStream {
		return &bftStream{send: d.svc.SendBatch, key: syspar.GetNodePrivKey(), networkID: d.networkID,
			height: height, round: round, validator: set.Proposer(height, round), kept: d.bodies.stream(height, round)}
	}, roundStart: d.roundStartOf, pull: d.pending.pullNow, written: d.awaitWritten}
	d.m = bft.NewMachine(d.config(), app, syspar.GetNodePrivKey(), syspar.GetNodePubKey(), d.networkID, height, set)
}

// reset starts the pool and the machine at the height after the last block
func (d *bftDriver) reset() error {
	last, err := lastBlock()
	if err != nil {
		return err
	}
	set, err := validatorsAt(last + 1)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.pool = bft.NewPool(d.networkID, syspar.GetBftMaxFutureRounds(), last+1, set)
	d.mu.Unlock()
	d.newMachine(last+1, set)
	d.watch.moveTo(last+1, set, time.Now(), false)
	d.pending.moveTo(last+1, set, syspar.GetNodePubKey())
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

// syncHeight moves the machine to the height after the last block if a block was committed
// without it: by the synchronization, or by a commit of this driver
func (d *bftDriver) syncHeight() error {
	last, err := lastBlock()
	if err != nil {
		return err
	}
	if last+1 == d.m.Height() {
		return nil
	}
	if last+1 < d.m.Height() {
		return fmt.Errorf("the last block %d is below the height %d of the consensus", last, d.m.Height())
	}
	set, err := validatorsAt(last + 1)
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
	if err := d.syncHeight(); err != nil {
		d.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("moving to the height")
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
	case bftWritten:
		d.apply(d.m.OnValid(e.height, e.hash, writtenValidity(d.logger, e)))
	default:
		d.give(e)
	}
}

func (d *bftDriver) tick() {
	d.svc.SetPeers(syspar.GetRemoteHosts())
	d.saveUnsaved()
	if bftStopped() {
		return
	}
	d.submitEvidence()
	now := time.Now()
	d.checkOverdue(now)
	d.checkClock(now)
	if err := d.syncHeight(); err != nil {
		d.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("moving to the height")
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
	return true
}

// hasWork reports whether the height has a transaction to put in a block: a transaction of the
// pool, or a delayed contract due at the height. A chain without one has no round.
func (d *bftDriver) hasWork() bool {
	if transaction.TxPool.Len() > 0 {
		return true
	}
	delayed, err := sqldb.GetAllDelayedContractsForBlockID(int64(d.m.Height()))
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting delayed contracts")
		return false
	}
	return len(delayed) > 0
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
			t := bftExpired{Timeout: *o.Timeout, due: time.Now().Add(o.Timeout.After)}
			time.AfterFunc(t.After, func() {
				select {
				case d.events <- t:
				case <-d.ctx.Done():
				}
			})
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

// commitDecision commits the block of the decision under DBLock if the last block is below its
// height, and reports whether the node has the height committed
func commitDecision(logger *log.Entry, c *bft.Commit) bool {
	lockWrites()
	defer DBUnlock()
	last, err := lastBlock()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting the last block")
		return false
	}
	if last >= c.Height {
		return true
	}
	if _, err := block.Commit(c.Block, bft.CertificateOf(c)); err != nil {
		logger.WithFields(log.Fields{"type": consts.BlockError, "error": err, "height": c.Height}).Error("committing the decision")
		return false
	}
	logger.WithFields(log.Fields{"height": c.Height, "round": c.Round, "hash": fmt.Sprintf("%x", c.Hash[:8])}).Info("committed")
	return true
}

// awaitWritten waits, apart from the driver, for the writes of the block the node proposed at the
// height, and gives the driver their end; the event is not dropped, the machine waits for it
func (d *bftDriver) awaitWritten(height uint64, hash bft.Hash, written func() error) {
	go func() {
		e := bftWritten{height: height, hash: hash, err: written()}
		select {
		case d.events <- e:
		case <-d.ctx.Done():
		}
	}()
}

// writtenValidity is the validity of a block the node proposed by the end of its writes: unknown
// for a trial the node dropped, which a validator then tries as any block if it needs to know
func writtenValidity(logger *log.Entry, e bftWritten) bft.Validity {
	switch {
	case e.err == nil:
		return bft.ValidityValid
	case errors.Is(e.err, block.ErrTrialDropped):
		return bft.ValidityUnknown
	}
	logger.WithFields(log.Fields{"type": consts.BlockError, "error": e.err, "height": e.height}).
		Warn("writing the block of the proposal of the node failed, it is not valid")
	return bft.ValidityInvalid
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
				return bftnet.FetchTransactions(addr, p.Height, p.Round, p.Hash, hashes, syspar.GetMaxBlockSize(), bftnet.DefaultConfig.Dial)
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
// in, those it streamed for the zero hash, and those of its queue
func (d *bftDriver) Transactions(height uint64, round uint32, hash bft.Hash, hashes [][]byte) [][]byte {
	if hash == (bft.Hash{}) {
		return d.bodies.streamedBodies(height, round, hashes, transaction.TxPool.Stored)
	}
	return d.bodies.transactions(hash, hashes, transaction.TxPool.Stored)
}

// Batch is bftnet.Handler.Batch: the speculation runs a batch the pool would admit
func (d *bftDriver) Batch(b *bft.Batch) {
	if bftStopped() {
		return
	}
	d.mu.Lock()
	key, ok := d.pool.AdmissibleBatch(b)
	d.mu.Unlock()
	if !ok {
		bftmon.Count(bftmon.MessagesDropped)
		return
	}
	d.spec.take(b, key)
}

// MaxBlockTxs is bftnet.Handler.MaxBlockTxs
func (d *bftDriver) MaxBlockTxs() int { return syspar.GetMaxTxCount() }

// fetchStreamed takes the bodies of the transactions of a batch from its proposer
func fetchStreamed(b *bft.Batch, key []byte, hashes [][]byte) ([][]byte, error) {
	addr, ok := syspar.GetNodeTCPAddress(key)
	if !ok {
		return nil, errNoProposer
	}
	bodies, err := bftnet.FetchTransactions(addr, b.Height, b.Round, bft.Hash{}, hashes, syspar.GetMaxBlockSize(), bftnet.DefaultConfig.Dial)
	if err != nil {
		return nil, err
	}
	// the proposer sends the bodies as a block stores them, compressed
	for i, body := range bodies {
		bodies[i] = types.DoZlibUnCompress(body)
	}
	return bodies, nil
}

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

// maxTimeWait is how long a proposer waits at most for its clock to pass the time of the last
// block
const maxTimeWait = syspar.BftProposerWait

// bftApp is the node the machine asks for a block and for its validity
type bftApp struct {
	logger *log.Entry
	bodies *bftBodies
	spec   *bftSpeculator
	// stream is the stream of the trial of the block of the node at a height and a round
	stream func(height uint64, round uint32) *bftStream
	// roundStart is when the validators started a round, zero if the node does not know
	roundStart func(height uint64, round uint32) time.Time
	// pull pulls before the deadline the transactions of the validators that said they had some
	// at the height, and reports whether the pool took any
	pull func(height uint64, deadline time.Time) bool
	// written gives the machine the end of the writes of the block of the node at a height, which
	// written waits for, as the event bftWritten
	written func(height uint64, hash bft.Hash, written func() error)
}

// Value generates the block of the proposer: its time is the clock in seconds and above the time
// of the last block, so a chain has at most one block a second, and the proposer makes it within
// max_block_generation_time from when it reads its clock (syspar.GetBftTimeliness). A proposer
// whose clock is not yet past the time of the last block waits for it, up to maxTimeWait, before
// it reads its clock: else a chain that commits faster than a block a second takes its blocks ever
// further ahead of the clocks, until the validators refuse them as untimely (design 4.4)
//
// The proposal reaches the validators before they time out: the proposer makes it within
// syspar.GetBftProposeBudget from when they started the round (roundStart), or from when it
// started it if that is earlier. A proposer that starts the round late, as it was still trying
// the block of the height before, makes a block of the time left, at least 1/proposeFloor of
// max_block_generation_time. The limit is the proposer's choice of the transactions of its
// block: the validators try the block without one, and its time only gets closer to their clocks.
//
// A proposer that finds no transaction for its block pulls, within the same deadline, those of the
// validators that said they had some at the height (bftPending), and makes its block of them.
//
// The block is valid once the writes its trial goes on with after the proposal succeed: Value
// returns it pending, and the machine prevotes it once written tells their end.
func (a bftApp) Value(height uint64, round uint32) (_ []byte, _ bft.Hash, pending bool, err error) {
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
	prev, err := lastBefore(height)
	if err != nil {
		return nil, bft.Hash{}, false, err
	}
	start := time.Now()
	started := start
	if a.roundStart != nil {
		if at := a.roundStart(height, round); !at.IsZero() && at.Before(started) {
			started = at
		}
	}
	deadline := started.Add(syspar.GetBftProposeBudget(round))
	if wait := faults.Until(time.Unix(prev.Time+1, 0)); wait > 0 {
		time.Sleep(min(wait, maxTimeWait))
	}
	waited := time.Since(start)
	metrics.BlockStage.WithLabelValues(metrics.KindPropose, metrics.StageWait).Observe(waited.Seconds())
	b, hash, err := a.propose(height, round, start, started, deadline, waited)
	if errors.Is(err, block.ErrEmptyBlock) && a.pull != nil && a.pull(height, deadline) {
		// the transactions of a validator that said it had some, which the pool of the node lost
		a.logger.WithFields(log.Fields{"height": height, "round": round, "pool": transaction.TxPool.Len()}).
			Info("the proposer pulled transactions for its block")
		b, hash, err = a.propose(height, round, start, started, deadline, waited)
	}
	if err != nil {
		return nil, bft.Hash{}, false, err
	}
	if a.written == nil {
		return b.BinData, hash, false, b.Written()
	}
	a.written(height, hash, b.Written)
	return b.BinData, hash, true, nil
}

// propose makes the block of Value from the transactions of the pool, which the proposer started
// at start, the validators at started, and waited for its clock: the block is made and tried under
// DBLock, on the last block as it is then
func (a bftApp) propose(height uint64, round uint32, start, started, deadline time.Time,
	waited time.Duration) (*block.Block, bft.Hash, error) {
	lockWrites()
	defer DBUnlock()
	prev, err := lastBefore(height)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	ts := max(faults.Now().Unix(), prev.Time+1)
	collect := time.Now()
	// the lag is how much later than the validators the proposer started the round
	lag := start.Sub(started)
	metrics.BlockStage.WithLabelValues(metrics.KindPropose, metrics.StageLag).Observe(lag.Seconds())
	limit, late := proposeLimit(collect, deadline, maxBlockGenerationTime())
	if late {
		a.logger.WithFields(log.Fields{"type": consts.BlockError, "height": height, "round": round, "lag": lag,
			"wait": waited, "limit": limit}).Warn("the proposer starts its proposal past its time")
	}
	privateKey, publicKey := utils.GetNodeKeys()
	dtx := DelayedTx{privateKey: privateKey, publicKey: publicKey, logger: a.logger, time: ts}
	txs, err := dtx.RunForDelayBlockID(int64(height))
	if err != nil {
		return nil, bft.Hash{}, err
	}
	trs, parsed, err := processTransactionsNew(a.logger, txs, time.Unix(ts, 0), limit)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	if len(trs) == 0 {
		return nil, bft.Hash{}, block.ErrEmptyBlock
	}
	collected := time.Since(collect)
	metrics.BlockStage.WithLabelValues(metrics.KindPropose, metrics.StageCollect).Observe(collected.Seconds())
	position, err := syspar.GetThisNodePosition()
	if err != nil {
		return nil, bft.Hash{}, err
	}
	header := &types.BlockHeader{
		BlockId:       int64(height),
		Timestamp:     ts,
		EcosystemId:   0,
		KeyId:         conf.Config.KeyID,
		NetworkId:     conf.Config.LocalConf.NetworkID,
		NodePosition:  position,
		Version:       consts.BlockVersion,
		ConsensusMode: consts.HonorNodeMode,
	}
	prevHeader := &types.BlockHeader{
		BlockId:       prev.BlockID,
		BlockHash:     prev.Hash,
		RollbacksHash: prev.RollbacksHash,
	}
	bin, err := generateNextBlock(header, prevHeader, trs)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	trial := time.Now()
	var stream *bftStream
	var s block.Stream
	if a.stream != nil {
		stream = a.stream(height, round)
		s = stream
	}
	b, err := block.Propose(bin, parsed, collect, limit, s)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	if stream != nil {
		stream.flush()
	}
	tried := time.Since(trial)
	metrics.BlockStage.WithLabelValues(metrics.KindPropose, metrics.StageTrial).Observe(tried.Seconds())
	hash, err := bft.HashOf(b.Header.BlockHash)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	total, behind := time.Since(collect), time.Since(time.Unix(ts, 0))
	metrics.BlockStage.WithLabelValues(metrics.KindPropose, metrics.StageTotal).Observe(total.Seconds())
	metrics.BlockStage.WithLabelValues(metrics.KindPropose, metrics.StageBehind).Observe(behind.Seconds())
	if total > limit {
		a.logger.WithFields(log.Fields{"type": consts.BlockError, "height": height, "round": round, "txs": len(b.TxFullData),
			"lag": lag, "wait": waited, "limit": limit, "collect": collected, "trial": tried, "total": total,
			"behind": behind}).Warn("a proposal past its time")
	}
	compact, err := block.Compact(b.BinData, b.Transactions)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	hashes, bodies := make([][]byte, len(b.Transactions)), make([][]byte, len(b.Transactions))
	for i, t := range b.Transactions {
		hashes[i], bodies[i] = t.Hash(), t.Compressed()
	}
	a.bodies.put(height, hash, b.BinData, compact, hashes, bodies)
	a.logger.WithFields(log.Fields{"height": height, "round": round, "txs": len(b.TxFullData)}).Debug("proposing")
	return b, hash, nil
}

// expired takes a timeout of the machine that expired. The precommit timeout of the current round
// starts the next round (L65) at the validators as it expires, which the driver keeps as the start of
// that round.
func (d *bftDriver) expired(e bftExpired) {
	d.countTimeout(e.Timeout)
	if e.Step == bft.StepPrecommit && e.Height == d.m.Height() && e.Round == d.m.Round() {
		d.roundStart = bftRoundStart{height: e.Height, round: e.Round + 1, at: e.due}
	}
	d.apply(d.m.OnTimeout(e.Timeout))
}

// roundStartOf is roundStart if it is of the height and the round, else zero
func (d *bftDriver) roundStartOf(height uint64, round uint32) time.Time {
	if s := d.roundStart; s.height == height && s.round == round {
		return s.at
	}
	return time.Time{}
}

// proposeFloor is the part of max_block_generation_time a proposer past the time of its proposal
// still gives its block: a validator that did not start the round, as it had no work, takes the
// proposal as it comes, and the others time out whatever the size of the block
const proposeFloor = 10

// proposeLimit is the time the block of a proposal whose collection starts at collect has:
// max_block_generation_time, cut to the deadline by which the proposal reaches the validators
// before they time out, and at least 1/proposeFloor of it; late reports that the deadline left
// less than that
func proposeLimit(collect, deadline time.Time, block time.Duration) (limit time.Duration, late bool) {
	limit = min(block, deadline.Sub(collect))
	if floor := block / proposeFloor; limit < floor {
		return floor, true
	}
	return limit, false
}

// lastBefore is the last block, which is the block before the height
func lastBefore(height uint64) (*sqldb.InfoBlock, error) {
	prev := &sqldb.InfoBlock{}
	if _, err := prev.Get(); err != nil {
		return nil, err
	}
	if uint64(prev.BlockID)+1 != height {
		return nil, fmt.Errorf("the last block %d is not before height %d", prev.BlockID, height)
	}
	return prev, nil
}

// dbWrites is DBLock as a sync.Locker
type dbWrites struct{}

func (dbWrites) Lock()   { DBLock() }
func (dbWrites) Unlock() { DBUnlock() }

// Valid is valid(v): the speculation of the streaming proposal goes on with the block, or else
// block.Validate tries it under DBLock
func (a bftApp) Valid(height uint64, hash bft.Hash, bin []byte) bool {
	start := time.Now()
	defer func() {
		metrics.BlockStage.WithLabelValues(metrics.KindTrial, metrics.StageValid).Observe(time.Since(start).Seconds())
	}()
	err := a.spec.finish(height, hash, bin)
	if errors.Is(err, block.ErrSpeculation) {
		err = block.Validate(bin, int64(height), hash, dbWrites{})
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
