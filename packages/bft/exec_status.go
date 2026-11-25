/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"encoding/binary"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

// ExecPrefix opens the bytes a validator signs for its execution status: no signature of a message
// of the consensus (MsgPrefix) passes as one of an execution status, nor the other way
const ExecPrefix = "IBAX-F18-EXEC-v1"

// ExecStatus is what a validator says of its executor after it executed a block: the height of
// that block and the transactions a second its executor ran at. It is not part of the consensus:
// the proposers size their blocks by the execution of the validators (direction A, proposer policy).
type ExecStatus struct {
	Validator uint32
	Height    uint64
	// Rate is the transactions a second of the last block of at least the measured size, zero
	// before one
	Rate      uint32
	Signature []byte
}

// ExecSignBytes is what a validator signs for its execution status:
// ExecPrefix ‖ network_id u64 ‖ height u64 ‖ rate u32
func ExecSignBytes(networkID, height uint64, rate uint32) []byte {
	b := make([]byte, 0, len(ExecPrefix)+8+8+4)
	b = append(b, ExecPrefix...)
	b = binary.BigEndian.AppendUint64(b, networkID)
	b = binary.BigEndian.AppendUint64(b, height)
	return binary.BigEndian.AppendUint32(b, rate)
}

// Sign signs the execution status with the node private key of its validator
func (s *ExecStatus) Sign(privateKey []byte, networkID uint64) (err error) {
	s.Signature, err = crypto.NodeSign(privateKey, ExecSignBytes(networkID, s.Height, s.Rate))
	return err
}

// Verify checks the signature of the execution status by the node public key of its validator
func (s *ExecStatus) Verify(publicKey []byte, networkID uint64) error {
	if len(s.Signature) == 0 || len(s.Signature) > crypto.GetAsymProvider().SignatureSize() {
		return ErrSignature
	}
	ok, err := crypto.NodeVerify(publicKey, ExecSignBytes(networkID, s.Height, s.Rate), s.Signature)
	if err != nil || !ok {
		return ErrSignature
	}
	return nil
}

// ExecStatusFixed is the length of the wire form of an execution status without its signature
const ExecStatusFixed = 4 + 8 + 4 + 2

// Marshal is the wire form of an execution status:
// validator u32 ‖ height u64 ‖ rate u32 ‖ len u16 ‖ signature
func (s *ExecStatus) Marshal() []byte {
	b := make([]byte, 0, ExecStatusFixed+len(s.Signature))
	b = binary.BigEndian.AppendUint32(b, s.Validator)
	b = binary.BigEndian.AppendUint64(b, s.Height)
	b = binary.BigEndian.AppendUint32(b, s.Rate)
	return appendBytes16(b, s.Signature)
}

// UnmarshalExecStatus decodes the wire form of an execution status; it refuses bytes left over
// and an execution status of height 0
func UnmarshalExecStatus(b []byte) (*ExecStatus, error) {
	r := reader{b: b}
	s := &ExecStatus{Validator: r.u32()}
	s.Height = r.u64()
	s.Rate = r.u32()
	s.Signature = r.bytes16()
	if err := r.done(); err != nil {
		return nil, err
	}
	if s.Height == 0 {
		return nil, fmt.Errorf("%w: an execution status of height 0", ErrFormat)
	}
	return s, nil
}
