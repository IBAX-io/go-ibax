/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"encoding/binary"
	"fmt"
	"math"
)

// The sign state of a validator (design 4.3): what it signed at its height, and its lock and its
// valid value. The node stores it before it sends a message it signed, and a machine that
// restores it after a restart signs nothing that conflicts with what the validator signed before.
//
// The machine signs only messages of its current round, and its round never goes back, so a
// machine restored at the round after the highest round of the state signs no message of a
// round it signed before. That is the change of round of L65-L67 as a crash looks to the other
// validators; the lock and the valid value are restored with it (L23, L29 and L15 read them), so
// the machine prevotes what the validator may prevote after its lock.

// SignState is what the validator of a machine signed at its height
type SignState struct {
	Height uint64
	// Round is the round of the machine when the state was taken: the validator signed nothing
	// above it
	Round       uint32
	LockedRound int32
	Locked      *Proposal // nil for nil
	ValidRound  int32
	Valid       *Proposal // nil for nil
	// Proposals and Votes are the messages the validator signed at the height
	Proposals []*Proposal
	Votes     []*Vote
}

// TakeSignState is the sign state of the machine if the machine signed a message, locked or
// took a valid value since it was last taken, and nil otherwise: the node stores a state it takes
// before it sends the messages the machine returned with it. A machine of a node whose key is no
// validator has none.
func (m *Machine) TakeSignState() *SignState {
	if !m.validator || !m.signChanged {
		return nil
	}
	m.signChanged = false
	s := &SignState{
		Height: m.height, Round: m.round,
		LockedRound: m.lockedRound, Locked: m.lockedValue,
		ValidRound: m.validRound, Valid: m.validValue,
	}
	for _, r := range m.rounds() {
		if p := m.proposals[r]; p.Validator == m.self {
			s.Proposals = append(s.Proposals, p)
		}
	}
	for r := uint32(0); r <= m.round; r++ {
		for _, t := range []MsgType{TypePrevote, TypePrecommit} {
			if v := m.votes[voteKey{r, t, m.self}]; v != nil {
				s.Votes = append(s.Votes, v)
			}
		}
	}
	return s
}

// Restore restarts the machine at its height from the sign state the node stored before it
// stopped: it takes the messages of the state as its own, restores the lock and the valid value,
// and starts the round after the round of the state. A state of another height changes nothing.
func (m *Machine) Restore(s *SignState) []Output {
	if s == nil || s.Height != m.height || m.decided || s.Round >= math.MaxInt32 {
		return nil
	}
	for _, p := range s.Proposals {
		if p.Height == m.height && p.Validator == m.self {
			m.recordProposal(p)
		}
	}
	for _, v := range s.Votes {
		if v.Height == m.height && v.Validator == m.self {
			m.recordVote(v)
		}
	}
	m.lockedRound, m.lockedValue = s.LockedRound, s.Locked
	m.validRound, m.validValue = s.ValidRound, s.Valid
	if m.lockedValue == nil {
		m.lockedRound = NoRound
	}
	if m.validValue == nil {
		m.validRound = NoRound
	}
	m.started = true
	if s.Round+1 > m.round {
		m.startRound(s.Round + 1)
	}
	m.signChanged = true
	m.run()
	return m.drain()
}

// Marshal is the stored form of a sign state: height u64 ‖ round u32 ‖ lockedRound i32 ‖
// locked ‖ validRound i32 ‖ valid ‖ proposals u16 ‖ proposals ‖ votes u16 ‖ votes, a proposal
// being len u32 ‖ the wire form of the proposal, len 0 for nil, and a vote len u16 ‖ the wire
// form of the vote
func (s *SignState) Marshal() []byte {
	b := binary.BigEndian.AppendUint64(nil, s.Height)
	b = binary.BigEndian.AppendUint32(b, s.Round)
	b = binary.BigEndian.AppendUint32(b, uint32(s.LockedRound))
	b = appendProposal(b, s.Locked)
	b = binary.BigEndian.AppendUint32(b, uint32(s.ValidRound))
	b = appendProposal(b, s.Valid)
	b = binary.BigEndian.AppendUint16(b, uint16(len(s.Proposals)))
	for _, p := range s.Proposals {
		b = appendProposal(b, p)
	}
	b = binary.BigEndian.AppendUint16(b, uint16(len(s.Votes)))
	for _, v := range s.Votes {
		b = appendBytes16(b, v.Marshal())
	}
	return b
}

func appendProposal(b []byte, p *Proposal) []byte {
	if p == nil {
		return binary.BigEndian.AppendUint32(b, 0)
	}
	m := p.Marshal()
	b = binary.BigEndian.AppendUint32(b, uint32(len(m)))
	return append(b, m...)
}

func (r *reader) proposal(maxBlock int64) *Proposal {
	n := r.u32()
	if r.err != nil || n == 0 {
		return nil
	}
	p, err := UnmarshalProposal(r.take(int(n)), maxBlock)
	if err != nil && r.err == nil {
		r.err = err
	}
	return p
}

// UnmarshalSignState decodes a sign state whose blocks are at most maxBlock bytes
func UnmarshalSignState(b []byte, maxBlock int64) (*SignState, error) {
	r := reader{b: b}
	s := &SignState{Height: r.u64(), Round: r.u32(), LockedRound: int32(r.u32())}
	s.Locked = r.proposal(maxBlock)
	s.ValidRound = int32(r.u32())
	s.Valid = r.proposal(maxBlock)
	for n := r.u16(); n > 0 && r.err == nil; n-- {
		if p := r.proposal(maxBlock); p != nil {
			s.Proposals = append(s.Proposals, p)
		}
	}
	for n := r.u16(); n > 0 && r.err == nil; n-- {
		raw := r.bytes16()
		if r.err != nil {
			break
		}
		v, err := UnmarshalVote(raw)
		if err != nil {
			return nil, err
		}
		s.Votes = append(s.Votes, v)
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	if s.Locked == nil && s.LockedRound != NoRound || s.Valid == nil && s.ValidRound != NoRound {
		return nil, fmt.Errorf("%w: a round of a lock or a valid value of nil", ErrFormat)
	}
	return s, nil
}
