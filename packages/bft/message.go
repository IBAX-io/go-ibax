/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

// HashSize is the length of a block hash the validators sign. A node of a chain of the BFT
// consensus refuses to start with a hash algorithm of another length (SHA384).
const HashSize = 32

// Hash is a block hash; the zero hash is nil, the vote for no block
type Hash [HashSize]byte

// Nil is the hash of nil, the value a validator votes for when it votes for no block
var Nil Hash

// ErrHashSize is the error of a block hash that is not HashSize bytes long
var ErrHashSize = errors.New("bft: the block hash is not 32 bytes long")

// HashOf is the hash of a block, of its header's BlockHash
func HashOf(blockHash []byte) (h Hash, err error) {
	if len(blockHash) != HashSize {
		return h, ErrHashSize
	}
	copy(h[:], blockHash)
	return h, nil
}

// MsgType is the type of a signed message
type MsgType uint8

const (
	// TypeProposal is a proposal of a block
	TypeProposal MsgType = 1
	// TypePrevote is a prevote
	TypePrevote MsgType = 2
	// TypePrecommit is a precommit
	TypePrecommit MsgType = 3
)

// NoRound is the validRound of a proposal of a block no round had a quorum of prevotes for
const NoRound int32 = -1

// MsgPrefix opens the bytes every validator signs, so that no signature of another kind of
// message of IBAX passes as a signature of the consensus
const MsgPrefix = "IBAX-F18-MSG-v1"

var (
	// ErrFormat is a message or an evidence that does not decode
	ErrFormat = errors.New("bft: malformed")
	// ErrSignature is a signature that does not verify
	ErrSignature = errors.New("bft: the signature does not verify")
)

func (t MsgType) valid() bool {
	return t >= TypeProposal && t <= TypePrecommit
}

func (t MsgType) String() string {
	switch t {
	case TypeProposal:
		return "proposal"
	case TypePrevote:
		return "prevote"
	case TypePrecommit:
		return "precommit"
	}
	return fmt.Sprintf("type %d", uint8(t))
}

// SignBytes is what a validator signs:
// MsgPrefix ‖ network_id u64 ‖ height u64 ‖ round u32 ‖ type u8 ‖ hash 32, and for a proposal
// ‖ validRound i32
func SignBytes(networkID, height uint64, round uint32, t MsgType, hash Hash, validRound int32) []byte {
	b := make([]byte, 0, len(MsgPrefix)+8+8+4+1+HashSize+4)
	b = append(b, MsgPrefix...)
	b = binary.BigEndian.AppendUint64(b, networkID)
	b = binary.BigEndian.AppendUint64(b, height)
	b = binary.BigEndian.AppendUint32(b, round)
	b = append(b, byte(t))
	b = append(b, hash[:]...)
	if t == TypeProposal {
		b = binary.BigEndian.AppendUint32(b, uint32(validRound))
	}
	return b
}

// Sign signs the bytes of a message with a node private key, as the node signs (crypto.NodeSign)
func Sign(privateKey []byte, networkID, height uint64, round uint32, t MsgType, hash Hash, validRound int32) ([]byte, error) {
	return crypto.NodeSign(privateKey, SignBytes(networkID, height, round, t, hash, validRound))
}

// Verify checks the signature of a message by a node public key
func Verify(publicKey []byte, networkID, height uint64, round uint32, t MsgType, hash Hash, validRound int32, signature []byte) error {
	if len(signature) == 0 || len(signature) > crypto.GetAsymProvider().SignatureSize() {
		return ErrSignature
	}
	ok, err := crypto.NodeVerify(publicKey, SignBytes(networkID, height, round, t, hash, validRound), signature)
	if err != nil || !ok {
		return ErrSignature
	}
	return nil
}

// Vote is a prevote or a precommit of a validator
type Vote struct {
	Type      MsgType
	Height    uint64
	Round     uint32
	Validator uint32
	Hash      Hash
	Signature []byte
}

// Sign signs the vote with the node private key of its validator
func (v *Vote) Sign(privateKey []byte, networkID uint64) (err error) {
	v.Signature, err = Sign(privateKey, networkID, v.Height, v.Round, v.Type, v.Hash, NoRound)
	return err
}

// Verify checks the signature of the vote by the node public key of its validator
func (v *Vote) Verify(publicKey []byte, networkID uint64) error {
	return Verify(publicKey, networkID, v.Height, v.Round, v.Type, v.Hash, NoRound, v.Signature)
}

// Marshal is the wire form of a vote:
// type u8 ‖ height u64 ‖ round u32 ‖ validator u32 ‖ hash 32 ‖ len u16 ‖ signature
func (v *Vote) Marshal() []byte {
	b := make([]byte, 0, 1+8+4+4+HashSize+2+len(v.Signature))
	b = append(b, byte(v.Type))
	b = binary.BigEndian.AppendUint64(b, v.Height)
	b = binary.BigEndian.AppendUint32(b, v.Round)
	b = binary.BigEndian.AppendUint32(b, v.Validator)
	b = append(b, v.Hash[:]...)
	return appendBytes16(b, v.Signature)
}

// UnmarshalVote decodes the wire form of a vote; it refuses a type other than a prevote or a
// precommit and bytes left over
func UnmarshalVote(b []byte) (*Vote, error) {
	r := reader{b: b}
	v := &Vote{Type: MsgType(r.u8())}
	v.Height = r.u64()
	v.Round = r.u32()
	v.Validator = r.u32()
	v.Hash = r.hash()
	v.Signature = r.bytes16()
	if err := r.done(); err != nil {
		return nil, err
	}
	if v.Type != TypePrevote && v.Type != TypePrecommit {
		return nil, fmt.Errorf("%w: a vote of %s", ErrFormat, v.Type)
	}
	return v, nil
}

// Proposal is the proposal of a block by the proposer of a height and a round. Hash is the hash
// of Block, which the receiver checks when it parses the block, after it verified the signature.
type Proposal struct {
	Height     uint64
	Round      uint32
	ValidRound int32
	Validator  uint32
	Hash       Hash
	Block      []byte
	Signature  []byte
}

// Sign signs the proposal with the node private key of its proposer
func (p *Proposal) Sign(privateKey []byte, networkID uint64) (err error) {
	p.Signature, err = Sign(privateKey, networkID, p.Height, p.Round, TypeProposal, p.Hash, p.ValidRound)
	return err
}

// Verify checks the signature of the proposal by the node public key of its proposer
func (p *Proposal) Verify(publicKey []byte, networkID uint64) error {
	return Verify(publicKey, networkID, p.Height, p.Round, TypeProposal, p.Hash, p.ValidRound, p.Signature)
}

// Marshal is the wire form of a proposal: height u64 ‖ round u32 ‖ validRound i32 ‖
// validator u32 ‖ hash 32 ‖ len u32 ‖ block ‖ len u16 ‖ signature
func (p *Proposal) Marshal() []byte {
	b := make([]byte, 0, 8+4+4+4+HashSize+4+len(p.Block)+2+len(p.Signature))
	b = binary.BigEndian.AppendUint64(b, p.Height)
	b = binary.BigEndian.AppendUint32(b, p.Round)
	b = binary.BigEndian.AppendUint32(b, uint32(p.ValidRound))
	b = binary.BigEndian.AppendUint32(b, p.Validator)
	b = append(b, p.Hash[:]...)
	b = binary.BigEndian.AppendUint32(b, uint32(len(p.Block)))
	b = append(b, p.Block...)
	return appendBytes16(b, p.Signature)
}

// UnmarshalProposal decodes the wire form of a proposal whose block is at most maxBlock bytes
// (max_block_size); it refuses a validRound below NoRound or not below the round, and bytes left
// over
func UnmarshalProposal(b []byte, maxBlock int64) (*Proposal, error) {
	r := reader{b: b}
	p := &Proposal{Height: r.u64(), Round: r.u32(), ValidRound: int32(r.u32()), Validator: r.u32(), Hash: r.hash()}
	size := r.u32()
	if r.err == nil && int64(size) > maxBlock {
		return nil, fmt.Errorf("%w: a block of %d bytes, more than %d", ErrFormat, size, maxBlock)
	}
	p.Block = r.take(int(size))
	p.Signature = r.bytes16()
	if err := r.done(); err != nil {
		return nil, err
	}
	if p.ValidRound < NoRound || int64(p.ValidRound) >= int64(p.Round) {
		return nil, fmt.Errorf("%w: validRound %d at round %d", ErrFormat, p.ValidRound, p.Round)
	}
	if p.Hash == Nil {
		return nil, fmt.Errorf("%w: a proposal of nil", ErrFormat)
	}
	return p, nil
}

func appendBytes16(b, s []byte) []byte {
	if len(s) > math.MaxUint16 {
		panic("bft: a byte string longer than a uint16 length")
	}
	b = binary.BigEndian.AppendUint16(b, uint16(len(s)))
	return append(b, s...)
}

// reader decodes fixed-length big-endian fields; past the first short read every field is zero
// and err tells
type reader struct {
	b   []byte
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.b) {
		r.err = fmt.Errorf("%w: short of %d bytes", ErrFormat, n-len(r.b))
		return nil
	}
	v := r.b[:n:n]
	r.b = r.b[n:]
	return v
}

func (r *reader) u8() uint8 {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *reader) u16() uint16 {
	if b := r.take(2); b != nil {
		return binary.BigEndian.Uint16(b)
	}
	return 0
}

func (r *reader) u32() uint32 {
	if b := r.take(4); b != nil {
		return binary.BigEndian.Uint32(b)
	}
	return 0
}

func (r *reader) u64() uint64 {
	if b := r.take(8); b != nil {
		return binary.BigEndian.Uint64(b)
	}
	return 0
}

func (r *reader) hash() (h Hash) {
	copy(h[:], r.take(HashSize))
	return h
}

func (r *reader) bytes16() []byte {
	n := r.u16()
	if r.err != nil {
		return nil
	}
	return append([]byte(nil), r.take(int(n))...)
}

func (r *reader) done() error {
	if r.err == nil && len(r.b) > 0 {
		r.err = fmt.Errorf("%w: %d bytes left over", ErrFormat, len(r.b))
	}
	return r.err
}
