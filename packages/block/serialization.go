/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"

	"github.com/pkg/errors"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// repeatMarshallBlock encodes the block this node generated, once it has run: the transactions of
// the block are those that ran well, in the order they ran, so they are not parsed again
func (b *Block) repeatMarshallBlock() error {
	ran := make(map[string]*transaction.Transaction, len(b.Transactions))
	for _, t := range b.Transactions {
		ran[string(t.FullData)] = t
	}
	txs := make([]*transaction.Transaction, 0, len(b.TxFullData))
	compressed := make([][]byte, 0, len(b.TxFullData))
	for _, data := range b.TxFullData {
		t, ok := ran[string(data)]
		if !ok {
			return errors.New("a transaction of the block is not one that ran")
		}
		txs = append(txs, t)
		compressed = append(compressed, t.Compressed())
	}
	data := &types.BlockData{}
	if err := data.Apply(
		types.WithCurHeader(b.Header),
		types.WithPrevHeader(b.PrevHeader),
		types.WithAfterTxs(b.AfterTxs),
		types.WithSysUpdate(b.SysUpdate),
		types.WithTxFullData(compressed)); err != nil {
		return errors.Wrap(err, "marshalling repeat block")
	}
	newBlockData, err := data.MarshallCompressed(syspar.GetNodePrivKey())
	if err != nil {
		return errors.Wrap(err, "marshalling repeat block")
	}
	b.BinData = newBlockData
	b.Transactions = txs
	b.MerkleRoot = data.MerkleRoot
	return nil
}

func MarshallBlock(opts ...types.BlockDataOption) ([]byte, error) {
	block := &types.BlockData{}
	if err := block.Apply(opts...); err != nil {
		return nil, err
	}
	return block.MarshallBlock(syspar.GetNodePrivKey())
}

func UnmarshallBlock(blockBuffer *bytes.Buffer, fill bool) (*Block, error) {
	block := &types.BlockData{}
	if err := block.UnmarshallBlock(blockBuffer.Bytes()); err != nil {
		return nil, err
	}
	transactions := make([]*transaction.Transaction, 0, len(block.TxFullData))
	for i := 0; i < len(block.TxFullData); i++ {
		tx, err := transaction.UnmarshallTransaction(bytes.NewBuffer(block.TxFullData[i]), fill)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, tx)
	}

	return &Block{
		BlockData:         block,
		PrevRollbacksHash: block.PrevHeader.RollbacksHash,
		Transactions:      transactions,
	}, nil
}
