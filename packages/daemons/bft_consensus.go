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
)

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
	events    chan any // *bft.Proposal, *bft.Vote, bft.Timeout

	mu   sync.Mutex // guards pool, which the handler of the service also uses
	pool *bft.Pool
	// bodies is the blocks of the proposals the node made or filled in
	bodies *bftBodies

	// m is used only by the goroutine of run
	m *bft.Machine
	// unsaved is a sign state the node could not store, and unsent the messages signed with it:
	// the driver sends them once it stores the state
	unsaved *bft.SignState
	unsent  []bft.Output
	resent  time.Time

	// store stores a sign state and transmit sends a message the machine signed: saveUnsaved
	// calls transmit only after store stored the state that has the message
	store    func(*bft.SignState) error
	transmit func(bft.Output)

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
		bodies:    newBftBodies(),
	}
	cfg := bftnet.DefaultConfig
	cfg.Rate = syspar.GetBftPeerMsgRate()
	d.svc = bftnet.NewService(cfg, d)
	d.store = func(s *bft.SignState) error {
		return (&sqldb.BftSignState{Height: int64(s.Height), Data: s.Marshal()}).Save()
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
	DBLock()
	err := d.reset()
	if err == nil {
		err = d.restore()
	}
	DBUnlock()
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Fatal("starting the consensus")
	}
	bftnet.Register(d.svc)
	defer func() {
		bftnet.Register(nil)
		d.svc.Close()
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
	d.m = bft.NewMachine(d.config(), bftApp{d.logger, d.bodies}, syspar.GetNodePrivKey(), syspar.GetNodePubKey(), d.networkID, height, set)
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
	d.watch.moveTo(last+1, set, time.Now())
	return nil
}

// restore restores the sign state the node stored at its height before it stopped (design 4.3):
// the machine signs nothing that conflicts with it, and the pool resends its messages
func (d *bftDriver) restore() error {
	stored := &sqldb.BftSignState{}
	found, err := stored.GetLast()
	if err != nil || !found || uint64(stored.Height) != d.m.Height() {
		return err
	}
	s, err := bft.UnmarshalSignState(stored.Data, syspar.GetMaxBlockSize())
	if err != nil {
		return fmt.Errorf("the stored sign state of height %d: %w", stored.Height, err)
	}
	d.mu.Lock()
	for _, p := range s.Proposals {
		d.pool.AddProposal(p)
	}
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
	d.watch.moveTo(last+1, set, now)
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
	d.bodies.prune(last + 1)
	d.newMachine(last+1, set)
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
	if t, ok := e.(bft.Timeout); ok {
		d.countTimeout(t)
		d.apply(d.m.OnTimeout(t))
		return
	}
	d.give(e)
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
	busy := !d.m.Decided() && d.hasWork()
	if busy {
		d.apply(d.m.Start())
	}
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
			t := *o.Timeout
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

// saveUnsaved stores the sign state not stored yet and sends the messages signed with it
func (d *bftDriver) saveUnsaved() {
	if d.unsaved != nil {
		s := d.unsaved
		if err := d.store(s); err != nil {
			d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("storing the sign state, the messages wait")
			return
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
	}
}

// commitDecision commits the block of the decision under DBLock if the last block is below its
// height, and reports whether the node has the height committed
func commitDecision(logger *log.Entry, c *bft.Commit) bool {
	DBLock()
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
				return bftnet.FetchTransactions(addr, p.Height, p.Hash, hashes, syspar.GetMaxBlockSize(), bftnet.DefaultConfig.Dial)
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
	return d.bodies.compact(p.Hash, p.Block)
}

// Transactions is bftnet.Handler.Transactions: the bodies of the blocks the node made or filled
// in, and those of its queue
func (d *bftDriver) Transactions(_ uint64, hash bft.Hash, hashes [][]byte) [][]byte {
	return d.bodies.transactions(hash, hashes, transaction.TxPool.Stored)
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
}

// Value generates the block of the proposer: its time is the clock in seconds and above the time
// of the last block, so a chain has at most one block a second, and the proposer makes it within
// max_block_generation_time from when it reads its clock (syspar.GetBftTimeliness). A proposer
// whose clock is not yet past the time of the last block waits for it, up to maxTimeWait, before
// it reads its clock: else a chain that commits faster than a block a second takes its blocks ever
// further ahead of the clocks, until the validators refuse them as untimely (design 4.4)
func (a bftApp) Value(height uint64, round uint32) ([]byte, bft.Hash, error) {
	prev, err := lastBefore(height)
	if err != nil {
		return nil, bft.Hash{}, err
	}
	start := time.Now()
	if wait := time.Unix(prev.Time+1, 0).Sub(faults.Now()); wait > 0 {
		time.Sleep(min(wait, maxTimeWait))
	}
	waited := time.Since(start)
	metrics.BlockStage.WithLabelValues(metrics.KindPropose, metrics.StageWait).Observe(waited.Seconds())
	// the block is made and tried under DBLock, on the last block as it is then
	DBLock()
	defer DBUnlock()
	if prev, err = lastBefore(height); err != nil {
		return nil, bft.Hash{}, err
	}
	ts := max(faults.Now().Unix(), prev.Time+1)
	collect := time.Now()
	privateKey, publicKey := utils.GetNodeKeys()
	dtx := DelayedTx{privateKey: privateKey, publicKey: publicKey, logger: a.logger, time: ts}
	txs, err := dtx.RunForDelayBlockID(int64(height))
	if err != nil {
		return nil, bft.Hash{}, err
	}
	trs, parsed, err := processTransactionsNew(a.logger, txs, time.Unix(ts, 0))
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
	b, err := block.Propose(bin, parsed, collect)
	if err != nil {
		return nil, bft.Hash{}, err
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
	if limit := time.Duration(syspar.GetMaxBlockGenerationTime()) * time.Millisecond; total > limit {
		a.logger.WithFields(log.Fields{"type": consts.BlockError, "height": height, "round": round, "txs": len(b.TxFullData),
			"wait": waited, "collect": collected, "trial": tried, "total": total, "behind": behind}).Warn("a proposal past max_block_generation_time")
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
	return b.BinData, hash, nil
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

// Valid is valid(v): block.Validate, which tries the block under DBLock
func (a bftApp) Valid(height uint64, hash bft.Hash, bin []byte) bool {
	err := block.Validate(bin, int64(height), hash, dbWrites{})
	if err != nil {
		a.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err, "height": height}).Warn("an invalid proposal")
	}
	return err == nil
}

// Timely is block.Timely at the time the node took the proposal
func (a bftApp) Timely(p *bft.Proposal) bool {
	ahead, behind := syspar.GetBftTimeliness(p.Round)
	err := block.Timely(p.Block, p.Received, ahead, behind)
	if err != nil {
		a.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err, "height": p.Height, "round": p.Round}).Warn("an untimely proposal")
	}
	return err == nil
}
