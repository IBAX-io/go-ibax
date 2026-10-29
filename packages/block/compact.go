/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/gogo/protobuf/proto"
)

// A proposal carries its block in the compact form (F18 design 4.5, plan 5.9): the block without
// the bodies of its transactions, and the hashes of the transactions in their order. A validator
// takes the bodies from its queue and asks the proposer for those it does not have; the block it
// has then is the block the proposer made, byte for byte, its hash and its merkle root those of
// the whole block.
//
// The form: len u32 ‖ the block without its transactions ‖ count u32 ‖ count × hash

// ErrCompact is a compact form that does not decode, or whose transactions are not those of
// its block
var ErrCompact = errors.New("a malformed compact block")

// Compact is the compact form of the block bin, whose transactions are txs in their order
func Compact(bin []byte, txs []*transaction.Transaction) ([]byte, error) {
	hashes := make([][]byte, len(txs))
	for i, t := range txs {
		hashes[i] = t.Hash()
	}
	return compactOf(bin, hashes)
}

// CompactBlock is the compact form of the block bin, its transactions parsed for their hashes
func CompactBlock(bin []byte) ([]byte, error) {
	b, err := UnmarshallBlock(bytes.NewBuffer(bin), false)
	if err != nil {
		return nil, err
	}
	return Compact(bin, b.Transactions)
}

func compactOf(bin []byte, hashes [][]byte) ([]byte, error) {
	raw := &types.BlockData{}
	if err := proto.Unmarshal(bin, raw); err != nil {
		return nil, fmt.Errorf("parsing the block: %w", err)
	}
	if len(raw.TxFullData) != len(hashes) {
		return nil, fmt.Errorf("%w: a block of %d transactions with %d hashes", ErrCompact, len(raw.TxFullData), len(hashes))
	}
	raw.TxFullData = nil
	shell, err := proto.Marshal(raw)
	if err != nil {
		return nil, err
	}
	size := crypto.HashSize()
	b := make([]byte, 0, 4+len(shell)+4+len(hashes)*size)
	b = binary.BigEndian.AppendUint32(b, uint32(len(shell)))
	b = append(b, shell...)
	b = binary.BigEndian.AppendUint32(b, uint32(len(hashes)))
	for _, h := range hashes {
		if len(h) != size {
			return nil, fmt.Errorf("%w: a hash of %d bytes", ErrCompact, len(h))
		}
		b = append(b, h...)
	}
	return b, nil
}

// CompactHashes is the hashes of the transactions of a compact form, in their order
func CompactHashes(compact []byte) ([][]byte, error) {
	_, hashes, err := parseCompact(compact)
	return hashes, err
}

func parseCompact(compact []byte) (*types.BlockData, [][]byte, error) {
	if len(compact) < 4 {
		return nil, nil, ErrCompact
	}
	n := int64(binary.BigEndian.Uint32(compact))
	if n > int64(len(compact)-4-4) {
		return nil, nil, ErrCompact
	}
	shell := &types.BlockData{}
	if err := proto.Unmarshal(compact[4:4+n], shell); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrCompact, err)
	}
	if len(shell.TxFullData) != 0 || shell.Header == nil || shell.PrevHeader == nil {
		return nil, nil, ErrCompact
	}
	rest := compact[4+n:]
	count, size := int64(binary.BigEndian.Uint32(rest)), int64(crypto.HashSize())
	rest = rest[4:]
	if count*size != int64(len(rest)) {
		return nil, nil, fmt.Errorf("%w: %d hashes in %d bytes", ErrCompact, count, len(rest))
	}
	hashes := make([][]byte, count)
	for i := range hashes {
		hashes[i] = rest[int64(i)*size : int64(i+1)*size : int64(i+1)*size]
	}
	return shell, hashes, nil
}

// Expand is the block of the compact form and the bodies of its transactions: stored is the transactions of the hashes in the form
// the block holds them, nil for one the node does not have, and fetch fetches such transactions
// from the proposer. The bodies the node has that do not make the merkle root of the block are
// fetched too, all of them, so a body stored apart from the proposer's makes no proposal invalid.
// The block is at most maxBlock bytes.
func Expand(compact []byte, maxBlock int64, stored func(hashes [][]byte) [][]byte,
	fetch func(hashes [][]byte) ([][]byte, error)) ([]byte, [][]byte, error) {
	shell, hashes, err := parseCompact(compact)
	if err != nil {
		return nil, nil, err
	}
	bodies := stored(hashes)
	if len(bodies) != len(hashes) {
		return nil, nil, fmt.Errorf("%d transactions stored for %d hashes", len(bodies), len(hashes))
	}
	var missing [][]byte
	var at []int
	for i, body := range bodies {
		if body == nil {
			missing = append(missing, hashes[i])
			at = append(at, i)
		}
	}
	if len(missing) > 0 {
		got, err := fetchAll(fetch, missing)
		if err != nil {
			return nil, nil, err
		}
		for j, i := range at {
			bodies[i] = got[j]
		}
	}
	shell.TxFullData = bodies
	if !bytes.Equal(shell.GenMerkleRoot(), shell.MerkleRoot) {
		if len(missing) == len(hashes) {
			return nil, nil, fmt.Errorf("%w: the transactions do not make the merkle root", ErrCompact)
		}
		if shell.TxFullData, err = fetchAll(fetch, hashes); err != nil {
			return nil, nil, err
		}
		if !bytes.Equal(shell.GenMerkleRoot(), shell.MerkleRoot) {
			return nil, nil, fmt.Errorf("%w: the transactions do not make the merkle root", ErrCompact)
		}
	}
	bin, err := proto.Marshal(shell)
	if err != nil {
		return nil, nil, err
	}
	if int64(len(bin)) > maxBlock {
		return nil, nil, types.ErrMaxBlockSize(maxBlock, len(bin))
	}
	return bin, shell.TxFullData, nil
}

func fetchAll(fetch func(hashes [][]byte) ([][]byte, error), hashes [][]byte) ([][]byte, error) {
	got, err := fetch(hashes)
	if err != nil {
		return nil, err
	}
	if len(got) != len(hashes) {
		return nil, fmt.Errorf("%w: %d transactions fetched for %d asked", ErrCompact, len(got), len(hashes))
	}
	for i, body := range got {
		if len(body) == 0 {
			return nil, fmt.Errorf("the proposer has no transaction %x", hashes[i])
		}
	}
	return got, nil
}
