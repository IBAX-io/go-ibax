/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"container/heap"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// A deterministic simulation of validators on a network: a virtual clock, delays of a seeded
// random source, stopped nodes, partitions and validators that sign two messages where one is
// due. The network delivers every message in the end (partial synchrony: a message held by a
// partition or for a stopped node arrives when it heals), as the gossip of the node layer resends
// them, and an honest node relays each message the first time it takes it, so that two messages a
// validator signed for two halves of the network both reach honest nodes. Each honest node runs a
// Machine; a node holds messages of later heights until it commits the heights before them, as
// the node layer does.

type simEvent struct {
	at       time.Duration
	seq      int
	node     int
	proposal *Proposal
	vote     *Vote
	timeout  *Timeout
	start    uint64 // the node moves to this height
}

type simQueue []*simEvent

func (q simQueue) Len() int { return len(q) }
func (q simQueue) Less(i, j int) bool {
	return q[i].at < q[j].at || q[i].at == q[j].at && q[i].seq < q[j].seq
}
func (q simQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *simQueue) Push(x any)   { *q = append(*q, x.(*simEvent)) }
func (q *simQueue) Pop() any {
	old := *q
	e := old[len(old)-1]
	*q = old[:len(old)-1]
	return e
}

type simWindow struct {
	from, to time.Duration
}

func (w simWindow) holds(at time.Duration) bool {
	return at >= w.from && at < w.to
}

type simApp struct {
	node int
}

func simBlock(height uint64, round uint32, node int, tag string) ([]byte, Hash) {
	b := []byte(fmt.Sprintf("block %d/%d of %d%s", height, round, node, tag))
	return b, sha256.Sum256(b)
}

func (a *simApp) Value(height uint64, round uint32) ([]byte, Hash, error) {
	b, h := simBlock(height, round, a.node, "")
	return b, h, nil
}

// Valid refuses a block whose hash is not its hash and a block of a forged time
func (a *simApp) Valid(height uint64, hash Hash, block []byte) bool {
	return sha256.Sum256(block) == hash && !containsTag(block, " forged-time")
}

// Timely takes every proposal: Valid refuses the blocks of a forged time
func (a *simApp) Timely(p *Proposal) bool {
	return true
}

func containsTag(b []byte, tag string) bool {
	return len(b) >= len(tag) && string(b[len(b)-len(tag):]) == tag
}

// simByzantine is a validator that signs two messages where one is due: two proposals in its
// rounds, one to each half of the validators, two prevotes and two precommits in every round it
// sees, a proposal in the rounds of others, and blocks of a forged time
type simByzantine struct {
	acted map[[2]uint64]bool
}

type sim struct {
	t       *testing.T
	rnd     *rand.Rand
	n       int
	vs      []testValidator
	set     Validators
	cfg     Config
	nodes   []*Machine
	byz     map[int]*simByzantine
	down    map[int]simWindow
	parts   []int // the side of each node in the partition
	part    simWindow
	delay   time.Duration
	jitter  time.Duration
	now     time.Duration
	seq     int
	q       simQueue
	held    []map[uint64][]*simEvent // messages of later heights, by node and height
	seen    []map[string]bool        // the messages each node relayed
	commits []map[uint64]Hash
	certs   []map[uint64]*Commit
	ev      []*Evidence
	msgs    int
}

func newSim(t *testing.T, seed int64, n int) *sim {
	s := &sim{t: t, rnd: rand.New(rand.NewSource(seed)), n: n, cfg: DefaultConfig, byz: map[int]*simByzantine{},
		down: map[int]simWindow{}, delay: 5 * time.Millisecond, jitter: 45 * time.Millisecond}
	s.vs, s.set = testSet(t, n)
	return s
}

func (s *sim) push(e *simEvent) {
	s.seq++
	e.seq = s.seq
	heap.Push(&s.q, e)
}

// send delivers a message from a node to every other node after the delay of the network
func (s *sim) send(from int, p *Proposal, v *Vote, only func(to int) bool) {
	for to := 0; to < s.n; to++ {
		if to == from || only != nil && !only(to) {
			continue
		}
		at := s.now + s.delay + time.Duration(s.rnd.Int63n(int64(s.jitter)+1))
		if s.parts != nil && s.parts[from] != s.parts[to] && s.part.holds(at) {
			at = s.part.to + s.delay + time.Duration(s.rnd.Int63n(int64(s.jitter)+1))
		}
		s.msgs++
		s.push(&simEvent{at: at, node: to, proposal: p, vote: v})
	}
}

func (s *sim) outputs(node int, out []Output) {
	m := s.nodes[node]
	for _, o := range out {
		switch {
		case o.Proposal != nil:
			s.send(node, o.Proposal, nil, nil)
		case o.Vote != nil:
			s.send(node, nil, o.Vote, nil)
		case o.Timeout != nil:
			t := *o.Timeout
			s.push(&simEvent{at: s.now + t.After, node: node, timeout: &t})
		case o.Evidence != nil:
			s.ev = append(s.ev, o.Evidence)
		case o.Commit != nil:
			c := o.Commit
			if prev, ok := s.commits[node][c.Height]; ok && prev != c.Hash {
				s.t.Fatalf("node %d committed height %d twice", node, c.Height)
			}
			s.commits[node][c.Height] = c.Hash
			s.certs[node][c.Height] = c
			// the node applies the block and moves to the next height
			s.push(&simEvent{at: s.now + 2*time.Millisecond, node: node, start: m.Height() + 1})
		}
	}
}

func (s *sim) byzantine(node int, h uint64, r uint32) {
	b := s.byz[node]
	k := [2]uint64{h, uint64(r)}
	if b.acted[k] {
		return
	}
	b.acted[k] = true
	half := func(to int) bool { return to%2 == 0 }
	other := func(to int) bool { return to%2 == 1 }
	priv := s.vs[node].private
	blockA, a := simBlock(h, r, node, " a")
	blockB, bb := simBlock(h, r, node, " b")
	blockF, f := simBlock(h, r, node, " forged-time")
	prop := func(block []byte, hash Hash, to func(int) bool, validator uint32) {
		p := &Proposal{Height: h, Round: r, ValidRound: NoRound, Validator: validator, Hash: hash, Block: block}
		if err := p.Sign(priv, testNetwork); err != nil {
			s.t.Fatal(err)
		}
		s.send(node, p, nil, to)
	}
	vote := func(t MsgType, hash Hash, to func(int) bool) {
		v := &Vote{Type: t, Height: h, Round: r, Validator: uint32(node), Hash: hash}
		if err := v.Sign(priv, testNetwork); err != nil {
			s.t.Fatal(err)
		}
		s.send(node, nil, v, to)
	}
	if s.set.Proposer(h, r) == uint32(node) {
		prop(blockA, a, half, uint32(node))
		prop(blockB, bb, other, uint32(node))
	} else {
		// a proposal in the round of another validator, under its own index: dropped
		prop(blockF, f, nil, uint32(node))
	}
	vote(TypePrevote, a, half)
	vote(TypePrevote, bb, other)
	vote(TypePrecommit, a, half)
	vote(TypePrecommit, bb, other)
}

// run runs the simulation until every honest node committed heights, or until the virtual time
// limit
func (s *sim) run(heights uint64, limit time.Duration) {
	s.nodes = make([]*Machine, s.n)
	s.held = make([]map[uint64][]*simEvent, s.n)
	s.seen = make([]map[string]bool, s.n)
	s.commits = make([]map[uint64]Hash, s.n)
	s.certs = make([]map[uint64]*Commit, s.n)
	for i := range s.nodes {
		s.held[i] = map[uint64][]*simEvent{}
		s.seen[i] = map[string]bool{}
		s.commits[i] = map[uint64]Hash{}
		s.certs[i] = map[uint64]*Commit{}
		if b, ok := s.byz[i]; ok {
			b.acted = map[[2]uint64]bool{}
			continue
		}
		s.nodes[i] = NewMachine(s.cfg, &simApp{node: i}, s.vs[i].private, s.vs[i].public, testNetwork, 1, s.set)
		s.push(&simEvent{at: 0, node: i, start: 1})
	}
	for s.q.Len() > 0 {
		e := heap.Pop(&s.q).(*simEvent)
		if e.at > limit {
			break
		}
		if w, ok := s.down[e.node]; ok && w.holds(e.at) {
			e.at = w.to
			s.push(e)
			continue
		}
		s.now = e.at
		if _, ok := s.byz[e.node]; ok {
			if e.proposal != nil {
				s.byzantine(e.node, e.proposal.Height, e.proposal.Round)
			} else if e.vote != nil {
				s.byzantine(e.node, e.vote.Height, e.vote.Round)
			}
			continue
		}
		m := s.nodes[e.node]
		switch {
		case e.start > 0:
			if e.start > 1 {
				m.NewHeight(e.start, s.set)
			}
			s.outputs(e.node, m.Start())
			held := s.held[e.node][e.start]
			delete(s.held[e.node], e.start)
			for _, h := range held {
				h.at = s.now
				s.push(h)
			}
		case e.timeout != nil:
			s.outputs(e.node, m.OnTimeout(*e.timeout))
		default:
			s.relay(e)
			height := uint64(0)
			if e.proposal != nil {
				height = e.proposal.Height
			} else {
				height = e.vote.Height
			}
			if height > m.Height() || height == m.Height() && m.Decided() {
				s.held[e.node][height] = append(s.held[e.node][height], e)
				continue
			}
			s.verify(e)
			if e.proposal != nil {
				s.outputs(e.node, m.OnProposal(e.proposal))
			} else {
				s.outputs(e.node, m.OnVote(e.vote))
			}
		}
		if s.done(heights) {
			return
		}
	}
}

// relay sends a message a node takes for the first time on to the other nodes
func (s *sim) relay(e *simEvent) {
	var key string
	if e.proposal != nil {
		key = "p" + string(e.proposal.Marshal())
	} else {
		key = "v" + string(e.vote.Marshal())
	}
	if s.seen[e.node][key] {
		return
	}
	s.seen[e.node][key] = true
	s.send(e.node, e.proposal, e.vote, nil)
}

// verify checks the signature of a message, as the node layer does before the machine takes it
func (s *sim) verify(e *simEvent) {
	if e.proposal != nil {
		if e.proposal.Verify(s.set.Keys[e.proposal.Validator], testNetwork) != nil {
			s.t.Fatal("a proposal of a bad signature")
		}
		return
	}
	if e.vote.Verify(s.set.Keys[e.vote.Validator], testNetwork) != nil {
		s.t.Fatal("a vote of a bad signature")
	}
}

func (s *sim) honest() []int {
	var hs []int
	for i := 0; i < s.n; i++ {
		if _, ok := s.byz[i]; !ok {
			hs = append(hs, i)
		}
	}
	return hs
}

func (s *sim) done(heights uint64) bool {
	for _, i := range s.honest() {
		if _, ok := s.commits[i][heights]; !ok {
			return false
		}
	}
	return true
}

// check fails on two hashes committed at one height, and on a certificate without a quorum of
// valid precommits; it returns the heights each of the nodes (default the honest ones) committed,
// and the most any honest node did
func (s *sim) check(nodes ...int) (all, most uint64) {
	counted := map[int]bool{}
	for _, i := range nodes {
		counted[i] = true
	}
	agreed := map[uint64]Hash{}
	all = ^uint64(0)
	for _, i := range s.honest() {
		var top uint64
		for h, hash := range s.commits[i] {
			if prev, ok := agreed[h]; ok && prev != hash {
				s.t.Fatalf("conflicting commits at height %d", h)
			}
			agreed[h] = hash
			c := s.certs[i][h]
			signers := map[uint32]bool{}
			for _, v := range c.Precommits {
				if v.Type != TypePrecommit || v.Height != h || v.Round != c.Round || v.Hash != hash ||
					v.Verify(s.set.Keys[v.Validator], testNetwork) != nil {
					s.t.Fatalf("a bad precommit in the certificate of height %d", h)
				}
				signers[v.Validator] = true
			}
			if len(signers) < s.set.Quorum() {
				s.t.Fatalf("the certificate of height %d has %d signers", h, len(signers))
			}
			cert, err := UnmarshalCertificate(CertificateOf(c).Marshal())
			if err != nil {
				s.t.Fatalf("the certificate of height %d: %v", h, err)
			}
			if err := cert.Verify(s.set, testNetwork, h, hash); err != nil {
				s.t.Fatalf("the certificate of height %d: %v", h, err)
			}
			if h > top {
				top = h
			}
		}
		for h := uint64(1); h <= top; h++ {
			if _, ok := s.commits[i][h]; !ok {
				s.t.Fatalf("node %d skipped height %d", i, h)
			}
		}
		if len(nodes) == 0 || counted[i] {
			all = min(all, top)
		}
		most = max(most, top)
	}
	return all, most
}

// The tests that look at the chain at a virtual time and after it run the simulation twice with
// the same seed, to that time and past it.

func TestSimNormal(t *testing.T) {
	for seed := int64(1); seed <= 20; seed++ {
		s := newSim(t, seed, 4)
		s.run(30, 10*time.Minute)
		if all, _ := s.check(); all < 30 {
			t.Fatalf("seed %d: %d heights, want 30", seed, all)
		}
		if len(s.ev) != 0 {
			t.Fatalf("seed %d: evidence of honest validators", seed)
		}
	}
}

func TestSimOneStopped(t *testing.T) {
	// N = 4 bears f = 1 validator off: the 3 commit
	s := newSim(t, 2, 4)
	s.down[3] = simWindow{0, 24 * time.Hour}
	s.run(0, 5*time.Minute)
	all, _ := s.check(0, 1, 2)
	if all < 20 {
		t.Fatalf("with 1 of 4 off: %d heights in 5 minutes", all)
	}
}

func TestSimTwoStopped(t *testing.T) {
	// 2 of 4 off, more than f: no height commits while they are off, and the chain goes on once
	// they are back, with no conflict
	s := newSim(t, 3, 4)
	s.down[2] = simWindow{0, 2 * time.Minute}
	s.down[3] = simWindow{0, 2 * time.Minute}
	s.run(0, 2*time.Minute-time.Millisecond)
	if _, most := s.check(); most != 0 {
		t.Fatalf("with 2 of 4 off: %d heights", most)
	}
	s = newSim(t, 3, 4)
	s.down[2] = simWindow{0, 2 * time.Minute}
	s.down[3] = simWindow{0, 2 * time.Minute}
	s.run(10, 2*time.Minute+5*time.Minute)
	if all, _ := s.check(); all < 10 {
		t.Fatalf("after 2 of 4 are back: %d heights", all)
	}
}

func TestSimPartition(t *testing.T) {
	// 2+2: neither side commits; once the network heals the chain goes on
	s := newSim(t, 4, 4)
	s.parts, s.part = []int{0, 0, 1, 1}, simWindow{0, time.Minute}
	s.run(0, time.Minute-time.Millisecond)
	if _, most := s.check(); most != 0 {
		t.Fatalf("a partition 2+2 commits %d heights", most)
	}
	s = newSim(t, 4, 4)
	s.parts, s.part = []int{0, 0, 1, 1}, simWindow{0, time.Minute}
	s.run(10, 5*time.Minute)
	if all, _ := s.check(); all < 10 {
		t.Fatalf("after a partition 2+2: %d heights", all)
	}

	// 3+1: the 3 commit; the 1 catches up after the network heals and commits the same blocks
	s = newSim(t, 5, 4)
	s.parts, s.part = []int{0, 0, 0, 1}, simWindow{0, 2 * time.Minute}
	s.run(0, 2*time.Minute-time.Millisecond)
	alone := len(s.commits[3])
	if all := min(len(s.commits[0]), len(s.commits[1]), len(s.commits[2])); all < 10 || alone != 0 {
		t.Fatalf("a partition 3+1: the 3 commit %d heights, the 1 %d", all, alone)
	}
	s.check()
	s = newSim(t, 5, 4)
	s.parts, s.part = []int{0, 0, 0, 1}, simWindow{0, 2 * time.Minute}
	s.run(60, 10*time.Minute)
	if all, _ := s.check(); all < 60 {
		t.Fatalf("after a partition 3+1: %d heights", all)
	}
}

func TestSimByzantine(t *testing.T) {
	// one validator of 4 signs two proposals, two prevotes and two precommits everywhere, proposes
	// in the rounds of others and blocks of a forged time: no conflicting commit, the chain goes
	// on, and the honest validators find evidence the chain accepts
	for seed := int64(1); seed <= 20; seed++ {
		s := newSim(t, seed, 4)
		s.byz[int(seed%4)] = &simByzantine{}
		s.run(20, 30*time.Minute)
		if all, _ := s.check(); all < 20 {
			t.Fatalf("seed %d: %d heights with a byzantine validator", seed, all)
		}
		if len(s.ev) == 0 {
			t.Fatalf("seed %d: no evidence", seed)
		}
		chain := &testChain{height: 1 << 20, maxAge: 1 << 20, set: s.set, seen: map[EvidenceKey]bool{}}
		for _, e := range s.ev {
			_, key, err := CheckEvidence(e.Marshal(), testNetwork, chain)
			if err != nil {
				t.Fatalf("seed %d: evidence of the chain refused: %v", seed, err)
			}
			if key.PublicKey != string(s.vs[seed%4].public) {
				t.Fatalf("seed %d: evidence against an honest validator", seed)
			}
		}
	}
	// N = 7 bears f = 2 of them
	for seed := int64(1); seed <= 10; seed++ {
		s := newSim(t, seed, 7)
		s.byz[1] = &simByzantine{}
		s.byz[4] = &simByzantine{}
		s.run(15, 30*time.Minute)
		if all, _ := s.check(); all < 15 {
			t.Fatalf("seed %d: %d heights with 2 byzantine validators of 7", seed, all)
		}
	}
}

func TestSimByzantineAndPartition(t *testing.T) {
	// a byzantine validator and a partition that cuts an honest one off: still no conflict
	for seed := int64(1); seed <= 10; seed++ {
		s := newSim(t, seed, 4)
		s.byz[0] = &simByzantine{}
		s.parts, s.part = []int{0, 0, 0, 1}, simWindow{30 * time.Second, 3 * time.Minute}
		s.run(10, 30*time.Minute)
		if all, _ := s.check(); all < 10 {
			t.Fatalf("seed %d: %d heights", seed, all)
		}
	}
}
