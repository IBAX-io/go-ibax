/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// A node parses each block of the consensus once (direction A). It uses a block three times: it
// checks the proposal (CheckProposal), it commits the block a certificate commits (Commit), and its
// executor executes the block (Committed). Each use parsed the block from its bytes, and the
// executor read the bytes from block_chain first. The node now keeps a block it parsed, by its
// hash, with its bytes: what the bytes give alone, the header and the transactions decoded
// without their contracts, of which it checked the hash and the merkle root. Each use binds the
// transactions to the contracts as the node has them at the time: the blocks the node executes
// in between change the contracts.

// decodedKeep is how many parsed blocks the node keeps: those from the height it executes to the
// height it checks the proposals of, BftExecDepth+1 heights, and the proposals of other rounds
const decodedKeep = 8

// Uses of a block, the label of metrics.BlockDecodes
const (
	useProposal = "proposal"
	useCommit   = "commit"
	useExecute  = "execute"
)

// decodedBlock is a block of the consensus as its bytes give it
type decodedBlock struct {
	hash bft.Hash
	bin  []byte
	// data is the block with its transactions uncompressed, and the header of the block before
	// as the block carries it
	data *types.BlockData
	// txs is the transactions decoded, not bound to their contracts (transaction.Decode)
	txs []*transaction.Transaction
	// committed is that the node committed the block
	committed bool
}

func (e *decodedBlock) height() int64 { return e.data.Header.BlockId }

// decodedBlocks is the blocks the node parsed, at most decodedKeep, the lowest heights left out first
type decodedBlocks struct {
	mu     sync.Mutex
	blocks []*decodedBlock
}

var decoded decodedBlocks

// get is the block of the hash the node parsed from the same bytes
func (d *decodedBlocks) get(hash bft.Hash, bin []byte) *decodedBlock {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.blocks {
		if e.hash == hash && bytes.Equal(e.bin, bin) {
			return e
		}
	}
	return nil
}

// put keeps the block the node parsed
func (d *decodedBlocks) put(e *decodedBlock) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, k := range d.blocks {
		if k.hash == e.hash {
			return
		}
	}
	d.blocks = append(d.blocks, e)
	for len(d.blocks) > decodedKeep {
		low := 0
		for i, k := range d.blocks {
			if k.height() < d.blocks[low].height() {
				low = i
			}
		}
		d.blocks = slices.Delete(d.blocks, low, low+1)
	}
}

// commit marks the block of the hash committed
func (d *decodedBlocks) commit(hash bft.Hash) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.blocks {
		if e.hash == hash {
			e.committed = true
		}
	}
}

// take is the committed block of the height, for its execution, and leaves out the blocks of the
// height and below: the node uses none of them again
func (d *decodedBlocks) take(height int64) *decodedBlock {
	d.mu.Lock()
	defer d.mu.Unlock()
	var got *decodedBlock
	d.blocks = slices.DeleteFunc(d.blocks, func(e *decodedBlock) bool {
		if e.height() == height && e.committed {
			got = e
		}
		return e.height() <= height
	})
	return got
}

// parseBft is the block of the bytes bin whose hash is hash, for the use: the block the node
// parsed before from the same bytes, or the block of at most max_block_size parsed now and kept,
// its transactions bound
func parseBft(bin []byte, hash bft.Hash, use string) (*Block, error) {
	e := decoded.get(hash, bin)
	if e != nil {
		metrics.BlockDecodes.WithLabelValues(use, "cached").Inc()
	} else {
		metrics.BlockDecodes.WithLabelValues(use, "parsed").Inc()
		if int64(len(bin)) > syspar.GetMaxBlockSize() {
			return nil, types.ErrMaxBlockSize(syspar.GetMaxBlockSize(), len(bin))
		}
		var err error
		if e, err = decodeBlock(bin, hash); err != nil {
			return nil, err
		}
		decoded.put(e)
	}
	return e.block()
}

// decodeBlock parses the block of the bytes bin and checks that its hash is hash and the hash of its content: its header, the header of the block before it, and the
// merkle root of its transactions as the block carries them, compressed. A parsed block takes
// its hash and its merkle root as they are written, so a certificate, which commits the hash,
// commits the content only by this check.
func decodeBlock(bin []byte, hash bft.Hash) (*decodedBlock, error) {
	start := time.Now()
	data := &types.BlockData{}
	if err := data.UnmarshallRaw(bin); err != nil {
		return nil, fmt.Errorf("%w: %w", types.ErrUnmarshallBlock, err)
	}
	h := data.Header
	if h == nil || data.PrevHeader == nil {
		return nil, errors.New("a block without its header or the header of the block before")
	}
	got, err := bft.HashOf(h.BlockHash)
	if err != nil {
		return nil, err
	}
	if got != hash {
		return nil, fmt.Errorf("block %d: the hash %x is not %x", h.BlockId, h.BlockHash, hash[:])
	}
	merkle := data.GenMerkleRoot()
	if !bytes.Equal(merkle, data.MerkleRoot) {
		return nil, fmt.Errorf("block %d: the merkle root is not that of its transactions", h.BlockId)
	}
	if !bytes.Equal(h.GenHash(data.PrevHeader, merkle), h.BlockHash) {
		return nil, fmt.Errorf("block %d: the hash is not that of its content", h.BlockId)
	}
	data.Uncompress()
	txs, err := decodeTxs(data.TxFullData)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", types.ErrUnmarshallBlock, err)
	}
	metrics.BlockStage.WithLabelValues(metrics.KindReceived, metrics.StageParse).Observe(metrics.Since(start))
	return &decodedBlock{hash: hash, bin: bin, data: data, txs: txs}, nil
}

// decodeTxs decodes the transactions in parallel; the error is that of the first that does not
func decodeTxs(full [][]byte) ([]*transaction.Transaction, error) {
	txs := make([]*transaction.Transaction, len(full))
	errs := make([]error, len(full))
	workers := min(runtime.GOMAXPROCS(0), len(full)/64+1)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := w; i < len(full); i += workers {
				txs[i], errs[i] = transaction.Decode(full[i])
			}
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return txs, nil
}

// block is the parsed block for a use: a Block of headers of its own, its transactions bound to
// their contracts as the node has them now, and the header of the block before it from the chain
func (e *decodedBlock) block() (*Block, error) {
	data := *e.data
	header, prev := *e.data.Header, *e.data.PrevHeader
	data.Header, data.PrevHeader = &header, &prev
	txs := make([]*transaction.Transaction, len(e.txs))
	for i, t := range e.txs {
		var err error
		if txs[i], err = t.Bind(); err != nil {
			return nil, fmt.Errorf("%w: %w", types.ErrUnmarshallBlock, err)
		}
	}
	b := &Block{BlockData: &data, PrevRollbacksHash: prev.RollbacksHash, Transactions: txs}
	var err error
	if b.PrevHeader, err = GetBlockHeaderFromBlockChain(header.BlockId - 1); err != nil {
		return nil, err
	}
	if b.PrevHeader == nil {
		return nil, errors.New("block previous header nil")
	}
	return b, nil
}
