/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"fmt"
	"math"
	"slices"
	"time"
)

// The consensus state machine of one height: Algorithm 1 of arXiv:1807.04938, and nothing else.
// Each rule cites its lines of the pseudocode as "L22". The machine does no I/O and reads no
// clock: it takes verified messages and expired timeouts, and returns what the node does, the
// messages it broadcasts, the timeouts it schedules, the block it commits and the evidence it
// found. The paper's 2f+1 is Quorum(N) and f+1 is Faults(N)+1, which are 2f+1 and f+1 for
// N = 3f+1 and keep the properties the proofs use for any N: two quorums share an honest
// validator, and f+1 validators hold an honest one.
//
// Line 10 runs StartRound(0) "upon start". The machine starts a height when the node calls
// Start, as it has transactions, or when it takes a message of the height from a validator that
// started: a chain without transactions has no rounds. The start is only later; no rule changes.

// Step is the step of a round
type Step uint8

const (
	// StepPropose waits for the proposal of the round
	StepPropose Step = iota
	// StepPrevote has prevoted and waits for a quorum of prevotes
	StepPrevote
	// StepPrecommit has precommitted
	StepPrecommit
)

func (s Step) String() string {
	return [...]string{"propose", "prevote", "precommit"}[s]
}

// Config is the timeouts of the platform parameters bft_timeout_propose_ms,
// bft_timeout_prevote_ms and bft_timeout_precommit_ms, each longer by Delta per round, and
// bft_max_future_rounds, the rounds above the current round of which the machine keeps messages
type Config struct {
	Propose, Prevote, Precommit, Delta time.Duration
	MaxFutureRounds                    uint32
}

// DefaultConfig is the starting values of the design, section 4.3
var DefaultConfig = Config{
	Propose:         3000 * time.Millisecond,
	Prevote:         1000 * time.Millisecond,
	Precommit:       1000 * time.Millisecond,
	Delta:           500 * time.Millisecond,
	MaxFutureRounds: 10,
}

func (c Config) timeout(s Step, round uint32) time.Duration {
	base := map[Step]time.Duration{StepPropose: c.Propose, StepPrevote: c.Prevote, StepPrecommit: c.Precommit}[s]
	return base + time.Duration(round)*c.Delta
}

// App is what the machine asks of the node
type App interface {
	// Value is the block the node proposes at a height and a round, its bytes and its hash
	// (getValue, L18); an error proposes nothing
	Value(height uint64, round uint32) ([]byte, Hash, error)
	// Valid reports whether the block may be committed at the height (valid(v)): it decodes, its
	// hash is the hash, it follows the last block, its time is right and its transactions apply
	Valid(height uint64, hash Hash, block []byte) bool
	// Timely reports whether the time of the block of a new proposal, one of no valid round, is
	// within the bounds of the clock of the node when it took the proposal (design 4.4). Only
	// L23 asks it: a value of a valid round had the prevotes of a quorum, and the time of a
	// block takes no part in the safety.
	Timely(p *Proposal) bool
}

// Timeout is a timeout the node schedules, and gives back to OnTimeout when it expires
type Timeout struct {
	Height uint64
	Round  uint32
	Step   Step
	After  time.Duration
}

// Commit is the decision of a height: the block and the precommits of a quorum for it, its
// certificate
type Commit struct {
	Height     uint64
	Round      uint32
	Hash       Hash
	Block      []byte
	Precommits []*Vote
}

// Output is one thing the node does; one field is set
type Output struct {
	Proposal *Proposal
	Vote     *Vote
	Timeout  *Timeout
	Commit   *Commit
	Evidence *Evidence
}

type voteKey struct {
	round     uint32
	t         MsgType
	validator uint32
}

type tallyKey struct {
	round uint32
	t     MsgType
	hash  Hash
}

// Machine is the consensus of a node at its current height
type Machine struct {
	cfg        Config
	app        App
	privateKey []byte
	publicKey  []byte
	networkID  uint64

	set       Validators
	self      uint32
	validator bool

	height  uint64 // h_p
	round   uint32 // round_p
	step    Step   // step_p
	started bool
	decided bool // decision_p[h_p] != nil

	lockedValue *Proposal // lockedValue_p, nil for nil
	lockedRound int32     // lockedRound_p
	validValue  *Proposal // validValue_p, nil for nil
	validRound  int32     // validRound_p

	proposals map[uint32]*Proposal // the first proposal of each round from its proposer
	votes     map[voteKey]*Vote    // the first vote of each validator, round and type
	tally     map[tallyKey]int
	any       map[voteKey]int            // votes of any hash of a round and type, validator 0
	senders   map[uint32]map[uint32]bool // the validators with a message of a round
	reported  map[voteKey]bool           // evidence found, by round, type and validator
	valid     map[Hash]bool

	prevoteTimeout   map[uint32]bool // L34 ran for the round
	locked           map[uint32]bool // L36 ran for the round
	precommitTimeout map[uint32]bool // L47 ran for the round

	signChanged bool // the machine signed, locked or took a valid value since TakeSignState

	out []Output
}

// NewMachine is the machine of a node signing by its node keys on the chain networkID, at height
// height with the validators set. A node whose key is no validator follows the consensus and
// signs nothing.
func NewMachine(cfg Config, app App, privateKey, publicKey []byte, networkID, height uint64, set Validators) *Machine {
	m := &Machine{cfg: cfg, app: app, privateKey: privateKey, publicKey: publicKey, networkID: networkID}
	m.NewHeight(height, set)
	return m
}

// NewHeight moves the machine to a height and its validators, after the node committed the height
// before. L1-L9 and L53: the round, the step, the lock and the valid value are reset.
func (m *Machine) NewHeight(height uint64, set Validators) {
	m.height, m.set = height, set
	m.self, m.validator = set.Index(m.publicKey)
	m.round, m.step = 0, StepPropose
	m.started, m.decided = false, false
	m.lockedValue, m.lockedRound = nil, NoRound
	m.validValue, m.validRound = nil, NoRound
	m.proposals = map[uint32]*Proposal{}
	m.votes = map[voteKey]*Vote{}
	m.tally = map[tallyKey]int{}
	m.any = map[voteKey]int{}
	m.senders = map[uint32]map[uint32]bool{}
	m.reported = map[voteKey]bool{}
	m.valid = map[Hash]bool{}
	m.prevoteTimeout = map[uint32]bool{}
	m.locked = map[uint32]bool{}
	m.precommitTimeout = map[uint32]bool{}
	m.signChanged = false
}

// Height is the height of the machine, Round its round and Step its step
func (m *Machine) Height() uint64 { return m.height }

// Round is the round of the machine
func (m *Machine) Round() uint32 { return m.round }

// Step is the step of the machine
func (m *Machine) Step() Step { return m.step }

// Decided reports whether the machine committed its height
func (m *Machine) Decided() bool { return m.decided }

// Locked is the round of the lock of the machine, NoRound for none, and the hash it locked
func (m *Machine) Locked() (int32, Hash) {
	if m.lockedValue == nil {
		return m.lockedRound, Nil
	}
	return m.lockedRound, m.lockedValue.Hash
}

// ValidValue is validRound_p and the hash of validValue_p
func (m *Machine) ValidValue() (int32, Hash) {
	if m.validValue == nil {
		return m.validRound, Nil
	}
	return m.validRound, m.validValue.Hash
}

// Start starts the height (L10) if it has not started
func (m *Machine) Start() []Output {
	if !m.started && !m.decided {
		m.started = true
		m.startRound(0)
		m.run()
	}
	return m.drain()
}

// OnProposal takes a proposal whose signature the node verified by the key of its validator
func (m *Machine) OnProposal(p *Proposal) []Output {
	if !m.accept(p.Height, p.Round, p.Validator) || p.Validator != m.set.Proposer(p.Height, p.Round) {
		return nil
	}
	m.recordProposal(p)
	m.begin()
	return m.drain()
}

// OnVote takes a vote whose signature the node verified by the key of its validator
func (m *Machine) OnVote(v *Vote) []Output {
	if !m.accept(v.Height, v.Round, v.Validator) || (v.Type != TypePrevote && v.Type != TypePrecommit) {
		return nil
	}
	m.recordVote(v)
	m.begin()
	return m.drain()
}

// OnTimeout takes a timeout that expired: L57-L67
func (m *Machine) OnTimeout(t Timeout) []Output {
	if m.decided || t.Height != m.height || t.Round != m.round {
		return nil
	}
	switch t.Step {
	case StepPropose:
		// L57-L60
		if m.step == StepPropose {
			m.broadcastVote(TypePrevote, Nil)
			m.step = StepPrevote
		}
	case StepPrevote:
		// L61-L64
		if m.step == StepPrevote {
			m.broadcastVote(TypePrecommit, Nil)
			m.step = StepPrecommit
		}
	case StepPrecommit:
		// L65-L67
		m.startRound(m.round + 1)
	}
	m.run()
	return m.drain()
}

// accept keeps the messages of the height, of a validator of its set and of a round the window
// keeps
func (m *Machine) accept(height uint64, round, validator uint32) bool {
	if m.decided || height != m.height || round > math.MaxInt32 {
		return false
	}
	if _, ok := m.set.Key(validator); !ok {
		return false
	}
	return uint64(round) <= uint64(m.round)+uint64(m.cfg.MaxFutureRounds)
}

// begin starts the height on a message of a validator that started it, and runs the rules
func (m *Machine) begin() {
	if !m.started {
		m.started = true
		m.startRound(0)
	}
	m.run()
}

func (m *Machine) sender(round, validator uint32) {
	s := m.senders[round]
	if s == nil {
		s = map[uint32]bool{}
		m.senders[round] = s
	}
	s[validator] = true
}

func (m *Machine) recordProposal(p *Proposal) {
	m.sender(p.Round, p.Validator)
	first := m.proposals[p.Round]
	if first == nil {
		m.proposals[p.Round] = p
		return
	}
	if first.Hash != p.Hash {
		m.evidence(p.Round, TypeProposal, p.Validator, first.Hash, p.Hash, first.ValidRound, p.ValidRound, first.Signature, p.Signature)
	}
}

func (m *Machine) recordVote(v *Vote) {
	m.sender(v.Round, v.Validator)
	k := voteKey{v.Round, v.Type, v.Validator}
	first := m.votes[k]
	if first == nil {
		m.votes[k] = v
		m.tally[tallyKey{v.Round, v.Type, v.Hash}]++
		m.any[voteKey{v.Round, v.Type, 0}]++
		return
	}
	if first.Hash != v.Hash {
		m.evidence(v.Round, v.Type, v.Validator, first.Hash, v.Hash, NoRound, NoRound, first.Signature, v.Signature)
	}
}

func (m *Machine) evidence(round uint32, t MsgType, validator uint32, a, b Hash, vrA, vrB int32, sigA, sigB []byte) {
	k := voteKey{round, t, validator}
	if m.reported[k] {
		return
	}
	m.reported[k] = true
	if e, err := NewEvidence(m.networkID, m.height, round, validator, t, a, b, vrA, vrB, sigA, sigB); err == nil {
		m.out = append(m.out, Output{Evidence: e})
	}
}

func (m *Machine) count(round uint32, t MsgType, hash Hash) int {
	return m.tally[tallyKey{round, t, hash}]
}

func (m *Machine) countAny(round uint32, t MsgType) int {
	return m.any[voteKey{round, t, 0}]
}

func (m *Machine) isValid(p *Proposal) bool {
	v, ok := m.valid[p.Hash]
	if !ok {
		v = m.app.Valid(m.height, p.Hash, p.Block)
		m.valid[p.Hash] = v
	}
	return v
}

// isTimely is Timely of the app for the proposal of another validator: the proposal of the node
// has the time Value gave its block
func (m *Machine) isTimely(p *Proposal) bool {
	return m.validator && p.Validator == m.self || m.app.Timely(p)
}

func (m *Machine) sign(t MsgType, hash Hash, validRound int32) []byte {
	sig, err := Sign(m.privateKey, m.networkID, m.height, m.round, t, hash, validRound)
	if err != nil {
		panic(fmt.Errorf("bft: signing by the node key: %w", err))
	}
	m.signChanged = true
	return sig
}

// broadcastVote signs and broadcasts a vote of the round, and takes it as a message of its own
func (m *Machine) broadcastVote(t MsgType, hash Hash) {
	if !m.validator {
		return
	}
	v := &Vote{Type: t, Height: m.height, Round: m.round, Validator: m.self, Hash: hash, Signature: m.sign(t, hash, NoRound)}
	m.out = append(m.out, Output{Vote: v})
	m.recordVote(v)
}

func (m *Machine) schedule(s Step) {
	m.out = append(m.out, Output{Timeout: &Timeout{Height: m.height, Round: m.round, Step: s, After: m.cfg.timeout(s, m.round)}})
}

// startRound is StartRound, L11-L21
func (m *Machine) startRound(round uint32) {
	m.round = round                                                 // L12
	m.step = StepPropose                                            // L13
	if m.validator && m.set.Proposer(m.height, m.round) == m.self { // L14
		var p *Proposal
		if m.validValue != nil { // L15
			p = &Proposal{Block: m.validValue.Block, Hash: m.validValue.Hash} // L16
		} else if block, hash, err := m.app.Value(m.height, m.round); err == nil && hash != Nil { // L18
			p = &Proposal{Block: block, Hash: hash}
			// the block of the proposer is valid as Value made it: the node tries it once
			m.valid[hash] = true
		}
		if p != nil {
			// L19: broadcast <PROPOSAL, h_p, round_p, proposal, validRound_p>
			p.Height, p.Round, p.ValidRound, p.Validator = m.height, m.round, m.validRound, m.self
			p.Signature = m.sign(TypeProposal, p.Hash, p.ValidRound)
			m.out = append(m.out, Output{Proposal: p})
			m.recordProposal(p)
			return
		}
		// a proposer without a block waits for the proposal as a validator that is not the
		// proposer does, and times out
	}
	m.schedule(StepPropose) // L21
}

// run applies the rules whose conditions hold until none does
func (m *Machine) run() {
	for !m.decided && m.started && m.rule() {
	}
}

// rule applies one rule whose condition holds, and reports whether it applied one
func (m *Machine) rule() bool {
	q := m.set.Quorum()
	r := m.round
	p := m.proposals[r]

	// L22-L27: upon <PROPOSAL, h_p, round_p, v, -1> from proposer(h_p, round_p) while
	// step_p = propose
	if p != nil && m.step == StepPropose && p.ValidRound == NoRound {
		if m.isTimely(p) && m.isValid(p) && (m.lockedRound == NoRound || m.lockedValue.Hash == p.Hash) { // L23
			m.broadcastVote(TypePrevote, p.Hash) // L24
		} else {
			m.broadcastVote(TypePrevote, Nil) // L26
		}
		m.step = StepPrevote // L27
		return true
	}

	// L28-L33: upon <PROPOSAL, h_p, round_p, v, vr> from proposer(h_p, round_p) AND 2f+1
	// <PREVOTE, h_p, vr, id(v)> while step_p = propose ∧ (vr >= 0 ∧ vr < round_p)
	if p != nil && m.step == StepPropose && p.ValidRound >= 0 && int64(p.ValidRound) < int64(r) &&
		m.count(uint32(p.ValidRound), TypePrevote, p.Hash) >= q {
		if m.isValid(p) && (m.lockedRound <= p.ValidRound || m.lockedValue.Hash == p.Hash) { // L29
			m.broadcastVote(TypePrevote, p.Hash) // L30
		} else {
			m.broadcastVote(TypePrevote, Nil) // L32
		}
		m.step = StepPrevote // L33
		return true
	}

	// L34-L35: upon 2f+1 <PREVOTE, h_p, round_p, *> while step_p = prevote for the first time
	if m.step == StepPrevote && m.countAny(r, TypePrevote) >= q && !m.prevoteTimeout[r] {
		m.prevoteTimeout[r] = true
		m.schedule(StepPrevote) // L35
		return true
	}

	// L36-L43: upon <PROPOSAL, h_p, round_p, v, *> from proposer(h_p, round_p) AND 2f+1
	// <PREVOTE, h_p, round_p, id(v)> while valid(v) ∧ step_p >= prevote for the first time
	if p != nil && m.step >= StepPrevote && !m.locked[r] && m.count(r, TypePrevote, p.Hash) >= q && m.isValid(p) {
		m.locked[r] = true
		if m.step == StepPrevote { // L37
			m.lockedValue, m.lockedRound = p, int32(r) // L38, L39
			m.broadcastVote(TypePrecommit, p.Hash)     // L40
			m.step = StepPrecommit                     // L41
		}
		m.validValue, m.validRound = p, int32(r) // L42, L43
		m.signChanged = true
		return true
	}

	// L44-L46: upon 2f+1 <PREVOTE, h_p, round_p, nil> while step_p = prevote
	if m.step == StepPrevote && m.count(r, TypePrevote, Nil) >= q {
		m.broadcastVote(TypePrecommit, Nil) // L45
		m.step = StepPrecommit              // L46
		return true
	}

	// L47-L48: upon 2f+1 <PRECOMMIT, h_p, round_p, *> for the first time
	if m.countAny(r, TypePrecommit) >= q && !m.precommitTimeout[r] {
		m.precommitTimeout[r] = true
		m.schedule(StepPrecommit) // L48
		return true
	}

	// L49-L54: upon <PROPOSAL, h_p, r, v, *> from proposer(h_p, r) AND 2f+1
	// <PRECOMMIT, h_p, r, id(v)> while decision_p[h_p] = nil
	for _, rr := range m.rounds() {
		if pr := m.proposals[rr]; pr != nil && m.count(rr, TypePrecommit, pr.Hash) >= q && m.isValid(pr) { // L50
			m.decide(rr, pr) // L51-L54
			return true
		}
	}

	// L55-L56: upon f+1 <*, h_p, round, *, *> with round > round_p
	next, found := uint32(0), false
	for rr, s := range m.senders {
		if rr > r && len(s) >= m.set.Faults()+1 && (!found || rr < next) {
			next, found = rr, true
		}
	}
	if found {
		m.startRound(next) // L56
		return true
	}
	return false
}

// decide is L51-L54: the machine commits the block with the precommits of its round; the node
// applies it and calls NewHeight, which is L52-L54
func (m *Machine) decide(round uint32, p *Proposal) {
	m.decided = true
	c := &Commit{Height: m.height, Round: round, Hash: p.Hash, Block: p.Block}
	for i := range m.set.Keys {
		if v := m.votes[voteKey{round, TypePrecommit, uint32(i)}]; v != nil && v.Hash == p.Hash {
			c.Precommits = append(c.Precommits, v)
		}
	}
	m.out = append(m.out, Output{Commit: c})
}

// rounds is the rounds with a proposal, in order
func (m *Machine) rounds() []uint32 {
	rs := make([]uint32, 0, len(m.proposals))
	for r := range m.proposals {
		rs = append(rs, r)
	}
	slices.Sort(rs)
	return rs
}

func (m *Machine) drain() []Output {
	out := m.out
	m.out = nil
	return out
}
