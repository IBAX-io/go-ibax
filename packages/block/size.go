/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"encoding/binary"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// blockSize tracks the length of the block a node is generating while its transactions run, so
// that the block it publishes is never longer than max_block_size, the limit its peers check on
// the bytes they receive (ProcessBlockByBinData). It counts what each transaction adds to the
// encoded block: its compressed data and its after_txs entries, which include its rollback rows.
// The fields only known once every transaction ran (signature, block hash, rollbacks hash, Merkle
// root) are counted at their maximum length, so the count never falls short of the block.
type blockSize struct {
	limit    int64
	fixed    int64 // headers, Merkle root and sys_update
	txs      int64 // tx_full_data entries
	afterTxs int64 // content of after_txs
}

func newBlockSize(limit int64, header, prev *types.BlockHeader) *blockSize {
	h := *header
	h.Sign = make([]byte, crypto.SignatureSize())
	h.BlockHash = make([]byte, crypto.HashSize())
	h.RollbacksHash = make([]byte, crypto.HashSize())
	fixed := &types.BlockData{
		Header:     &h,
		PrevHeader: prev,
		MerkleRoot: make([]byte, 2*crypto.HashSize()), // hex
		SysUpdate:  true,
	}
	return &blockSize{limit: limit, fixed: int64(fixed.Size())}
}

// size is the length of the encoded block counted so far
func (s *blockSize) size() int64 {
	size := s.fixed + s.txs
	if s.afterTxs > 0 {
		size += fieldSize(s.afterTxs)
	}
	return size
}

// add counts a transaction that ran, with the entries it adds to after_txs. If the block would
// exceed the limit, it counts nothing and returns false.
func (s *blockSize) add(fullData []byte, after *types.AfterTx, rollbacks []*types.RollbackTx) bool {
	tx := fieldSize(int64(len(types.DoZlibCompress(fullData))))
	afterTxs := s.afterTxs + fieldSize(int64(after.Size()))
	for _, r := range rollbacks {
		afterTxs += fieldSize(int64(r.Size()))
	}
	next := blockSize{limit: s.limit, fixed: s.fixed, txs: s.txs + tx, afterTxs: afterTxs}
	if next.size() > s.limit {
		return false
	}
	*s = next
	return true
}

// fieldSize is the length of a length-delimited protobuf field of n bytes. Every such field of a
// block has a number below 16, so its tag is one byte.
func fieldSize(n int64) int64 {
	return 1 + int64(len(binary.AppendUvarint(nil, uint64(n)))) + n
}
