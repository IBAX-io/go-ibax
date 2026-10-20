/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// EvidencePrefix opens the body of an evidence transaction
const EvidencePrefix = "IBAX-F18-EVD-v1"

// The kinds of evidence: a validator signed two messages of one height, round and type for two
// hashes
const (
	KindDoubleProposal  uint8 = 1
	KindDoublePrevote   uint8 = 2
	KindDoublePrecommit uint8 = 3
)

// The checks of an evidence, in order (design section 4.5); an evidence that fails one is invalid
var (
	// ErrEvidenceFormat fails check 1: the body decodes with no bytes left over, its prefix, its
	// network and its kind are right, and only a double proposal has validRounds
	ErrEvidenceFormat = errors.New("bft: evidence check 1, the format")
	// ErrEvidenceAge fails check 2: the height is not above the chain and not older than
	// bft_evidence_max_age
	ErrEvidenceAge = errors.New("bft: evidence check 2, the height")
	// ErrEvidenceValidator fails check 3: the validator is in the set of the height
	ErrEvidenceValidator = errors.New("bft: evidence check 3, the validator")
	// ErrEvidenceSignature fails check 4: both signatures verify by the key of the validator
	ErrEvidenceSignature = errors.New("bft: evidence check 4, the signatures")
	// ErrEvidenceSameHash fails check 5: the two hashes differ
	ErrEvidenceSameHash = errors.New("bft: evidence check 5, two hashes")
	// ErrEvidenceDuplicate fails check 6: no evidence of the same key is on the chain
	ErrEvidenceDuplicate = errors.New("bft: evidence check 6, a duplicate")
)

// Evidence is the body of an evidence transaction: two messages a validator signed for one
// height, round and type and two hashes
type Evidence struct {
	Kind        uint8
	NetworkID   uint64
	Height      uint64
	Round       uint32
	Validator   uint32
	HashA       Hash
	HashB       Hash
	ValidRoundA int32
	ValidRoundB int32
	SigA        []byte
	SigB        []byte
}

// EvidenceKey is the unique key of an evidence: a validator is stopped once per height, round and
// kind
type EvidenceKey struct {
	PublicKey string
	Height    uint64
	Round     uint32
	Kind      uint8
}

// MsgType is the type of the two messages of the evidence
func (e *Evidence) MsgType() MsgType {
	return MsgType(e.Kind)
}

// Marshal is the body of the transaction: EvidencePrefix ‖ kind u8 ‖ network_id u64 ‖
// height u64 ‖ round u32 ‖ validator u32 ‖ hash_a 32 ‖ hash_b 32 ‖ valid_round_a i32 ‖
// valid_round_b i32 ‖ len u16 ‖ sig_a ‖ len u16 ‖ sig_b
func (e *Evidence) Marshal() []byte {
	b := make([]byte, 0, len(EvidencePrefix)+1+8+8+4+4+2*HashSize+4+4+2+len(e.SigA)+2+len(e.SigB))
	b = append(b, EvidencePrefix...)
	b = append(b, e.Kind)
	b = binary.BigEndian.AppendUint64(b, e.NetworkID)
	b = binary.BigEndian.AppendUint64(b, e.Height)
	b = binary.BigEndian.AppendUint32(b, e.Round)
	b = binary.BigEndian.AppendUint32(b, e.Validator)
	b = append(b, e.HashA[:]...)
	b = append(b, e.HashB[:]...)
	b = binary.BigEndian.AppendUint32(b, uint32(e.ValidRoundA))
	b = binary.BigEndian.AppendUint32(b, uint32(e.ValidRoundB))
	b = appendBytes16(b, e.SigA)
	return appendBytes16(b, e.SigB)
}

// UnmarshalEvidence decodes the body of an evidence transaction and makes check 1 of it on the
// chain of networkID
func UnmarshalEvidence(b []byte, networkID uint64) (*Evidence, error) {
	r := reader{b: b}
	if string(r.take(len(EvidencePrefix))) != EvidencePrefix {
		return nil, fmt.Errorf("%w: no prefix %s", ErrEvidenceFormat, EvidencePrefix)
	}
	e := &Evidence{Kind: r.u8(), NetworkID: r.u64(), Height: r.u64(), Round: r.u32(), Validator: r.u32(),
		HashA: r.hash(), HashB: r.hash(), ValidRoundA: int32(r.u32()), ValidRoundB: int32(r.u32())}
	e.SigA = r.bytes16()
	e.SigB = r.bytes16()
	if err := r.done(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrEvidenceFormat, err)
	}
	if e.NetworkID != networkID {
		return nil, fmt.Errorf("%w: network %d, the chain is %d", ErrEvidenceFormat, e.NetworkID, networkID)
	}
	switch e.Kind {
	case KindDoubleProposal:
		if e.ValidRoundA < NoRound || e.ValidRoundB < NoRound {
			return nil, fmt.Errorf("%w: validRounds %d, %d", ErrEvidenceFormat, e.ValidRoundA, e.ValidRoundB)
		}
	case KindDoublePrevote, KindDoublePrecommit:
		if e.ValidRoundA != NoRound || e.ValidRoundB != NoRound {
			return nil, fmt.Errorf("%w: validRounds of a vote", ErrEvidenceFormat)
		}
	default:
		return nil, fmt.Errorf("%w: kind %d", ErrEvidenceFormat, e.Kind)
	}
	return e, nil
}

// EvidenceChain is what the checks 2, 3 and 6 of an evidence read of the chain
type EvidenceChain interface {
	// Height is the height of the last committed block
	Height() uint64
	// MaxAge is bft_evidence_max_age, in heights
	MaxAge() uint64
	// Validators is the set of validators of a height not above Height
	Validators(height uint64) (Validators, error)
	// HasEvidence reports whether an evidence of the key is on the chain
	HasEvidence(key EvidenceKey) (bool, error)
}

// CheckEvidence decodes the body of an evidence transaction and makes the six checks of it; it
// returns the evidence and its key, or the error of the first check that fails
func CheckEvidence(body []byte, networkID uint64, chain EvidenceChain) (*Evidence, EvidenceKey, error) {
	e, err := UnmarshalEvidence(body, networkID)
	if err != nil {
		return nil, EvidenceKey{}, err
	}
	top := chain.Height()
	if e.Height == 0 || e.Height > top || top-e.Height > chain.MaxAge() {
		return nil, EvidenceKey{}, fmt.Errorf("%w: height %d, the chain is at %d and keeps evidence of %d heights", ErrEvidenceAge, e.Height, top, chain.MaxAge())
	}
	set, err := chain.Validators(e.Height)
	if err != nil {
		return nil, EvidenceKey{}, err
	}
	key, ok := set.Key(e.Validator)
	if !ok {
		return nil, EvidenceKey{}, fmt.Errorf("%w: no validator %d of %d at height %d", ErrEvidenceValidator, e.Validator, set.N(), e.Height)
	}
	t := e.MsgType()
	if Verify(key, networkID, e.Height, e.Round, t, e.HashA, e.ValidRoundA, e.SigA) != nil ||
		Verify(key, networkID, e.Height, e.Round, t, e.HashB, e.ValidRoundB, e.SigB) != nil {
		return nil, EvidenceKey{}, ErrEvidenceSignature
	}
	if e.HashA == e.HashB {
		return nil, EvidenceKey{}, ErrEvidenceSameHash
	}
	k := EvidenceKey{PublicKey: string(key), Height: e.Height, Round: e.Round, Kind: e.Kind}
	dup, err := chain.HasEvidence(k)
	if err != nil {
		return nil, EvidenceKey{}, err
	}
	if dup {
		return nil, EvidenceKey{}, ErrEvidenceDuplicate
	}
	return e, k, nil
}

// NewEvidence is the evidence of two messages of one validator, height, round and type for two
// hashes; validRounds are those of two proposals, NoRound for votes
func NewEvidence(networkID, height uint64, round, validator uint32, t MsgType, hashA, hashB Hash, validRoundA, validRoundB int32, sigA, sigB []byte) (*Evidence, error) {
	if !t.valid() || hashA == hashB {
		return nil, fmt.Errorf("%w: no evidence of %s for %x and %x", ErrFormat, t, hashA, hashB)
	}
	if t != TypeProposal {
		validRoundA, validRoundB = NoRound, NoRound
	}
	return &Evidence{Kind: uint8(t), NetworkID: networkID, Height: height, Round: round, Validator: validator,
		HashA: hashA, HashB: hashB, ValidRoundA: validRoundA, ValidRoundB: validRoundB,
		SigA: append([]byte(nil), sigA...), SigB: append([]byte(nil), sigB...)}, nil
}
