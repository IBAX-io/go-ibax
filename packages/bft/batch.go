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

// A batch is a part of a block the proposer of a height and a round sends while it tries the
// block (design 4.2 "streaming proposals"): the hashes of the transactions its trial put in the
// block since the batch before, in their order, the first batch with the header of the block. A
// validator runs the batches ahead of the proposal; the proposal decides. A batch is no message of
// Algorithm 1: no vote counts it, no evidence comes of it, and no node relays it.

// BatchPrefix opens the bytes the proposer signs for a batch
const BatchPrefix = "IBAX-F18-BAT-v1"

// MaxBatchHeader bounds the header of the first batch
const MaxBatchHeader = 4096

// Batch is a batch of the proposal of a height and a round
type Batch struct {
	Height    uint64
	Round     uint32
	Validator uint32
	Seq       uint32
	// Header is the block without its transactions, its merkle root, its signature and its hash,
	// in the first batch only
	Header []byte
	Hashes [][]byte
	// Signature signs BatchPrefix ‖ network_id u64 ‖ the digest of the wire form up to it
	Signature []byte
}

// unsigned is the wire form of the batch up to its signature
func (b *Batch) unsigned() []byte {
	out := make([]byte, 0, 8+4+4+4+4+len(b.Header)+4+len(b.Hashes)*HashSize)
	out = binary.BigEndian.AppendUint64(out, b.Height)
	out = binary.BigEndian.AppendUint32(out, b.Round)
	out = binary.BigEndian.AppendUint32(out, b.Validator)
	out = binary.BigEndian.AppendUint32(out, b.Seq)
	out = binary.BigEndian.AppendUint32(out, uint32(len(b.Header)))
	out = append(out, b.Header...)
	out = binary.BigEndian.AppendUint32(out, uint32(len(b.Hashes)))
	for _, h := range b.Hashes {
		out = append(out, h...)
	}
	return out
}

func batchSignBytes(networkID uint64, unsigned []byte) []byte {
	b := make([]byte, 0, len(BatchPrefix)+8+HashSize)
	b = append(b, BatchPrefix...)
	b = binary.BigEndian.AppendUint64(b, networkID)
	return append(b, crypto.Hash(unsigned)...)
}

// Sign signs the batch with the node private key of its proposer
func (b *Batch) Sign(privateKey []byte, networkID uint64) (err error) {
	b.Signature, err = crypto.NodeSign(privateKey, batchSignBytes(networkID, b.unsigned()))
	return err
}

// Verify checks the signature of the batch by the node public key of its proposer
func (b *Batch) Verify(publicKey []byte, networkID uint64) error {
	if len(b.Signature) == 0 || len(b.Signature) > crypto.GetAsymProvider().SignatureSize() {
		return ErrSignature
	}
	ok, err := crypto.NodeVerify(publicKey, batchSignBytes(networkID, b.unsigned()), b.Signature)
	if err != nil || !ok {
		return ErrSignature
	}
	return nil
}

// Marshal is the wire form of a batch: height u64 ‖ round u32 ‖ validator u32 ‖ seq u32 ‖
// len u32 ‖ header ‖ count u32 ‖ count × hash ‖ len u16 ‖ signature
func (b *Batch) Marshal() []byte {
	return appendBytes16(b.unsigned(), b.Signature)
}

// UnmarshalBatch decodes the wire form of a batch of at most maxTxs hashes. It refuses a first
// batch without a header, a later one with a header or without a hash, a header longer than
// MaxBatchHeader, and bytes left over.
func UnmarshalBatch(data []byte, maxTxs int) (*Batch, error) {
	r := reader{b: data}
	b := &Batch{Height: r.u64(), Round: r.u32(), Validator: r.u32(), Seq: r.u32()}
	size := r.u32()
	if r.err == nil && size > MaxBatchHeader {
		return nil, fmt.Errorf("%w: a batch header of %d bytes", ErrFormat, size)
	}
	if header := r.take(int(size)); len(header) > 0 {
		b.Header = append([]byte(nil), header...)
	}
	count := r.u32()
	if r.err == nil && int64(count) > int64(maxTxs) {
		return nil, fmt.Errorf("%w: a batch of %d transactions, more than %d", ErrFormat, count, maxTxs)
	}
	all := r.take(int(count) * HashSize)
	b.Signature = r.bytes16()
	if err := r.done(); err != nil {
		return nil, err
	}
	if (b.Seq == 0) != (len(b.Header) > 0) {
		return nil, fmt.Errorf("%w: batch %d with a header of %d bytes", ErrFormat, b.Seq, len(b.Header))
	}
	if b.Seq > 0 && count == 0 {
		return nil, fmt.Errorf("%w: batch %d without a transaction", ErrFormat, b.Seq)
	}
	b.Hashes = make([][]byte, count)
	for i := range b.Hashes {
		b.Hashes[i] = append([]byte(nil), all[i*HashSize:(i+1)*HashSize]...)
	}
	return b, nil
}
