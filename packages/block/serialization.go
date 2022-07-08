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

func (b *Block) repeatMarshallBlock() error {
	newBlockData, err := MarshallBlock(
		types.WithCurHeader(b.Header),
		types.WithPrevHeader(b.PrevHeader),
		types.WithAfterTxs(b.AfterTxs),
		types.WithSysUpdate(b.SysUpdate),
		types.WithTxFullData(b.TxFullData))
	if err != nil {
		return errors.Wrap(err, "marshalling repeat block")
	}

	var nb = new(Block)
	nb, err = UnmarshallBlock(bytes.NewBuffer(newBlockData))
	if err != nil {
		return errors.Wrap(err, "parsing repeat block")
	}
	b.BinData = newBlockData
	b.Transactions = nb.Transactions
	b.MerkleRoot = nb.MerkleRoot
	return nil
}

func MarshallBlock(opts ...types.BlockDataOption) ([]byte, error) {
	block := &types.BlockData{}
	if err := block.Apply(opts...); err != nil {
		return nil, err
	}
	return block.MarshallBlock(syspar.GetNodePrivKey())
}

func UnmarshallBlock(blockBuffer *bytes.Buffer) (*Block, error) {
	block := &types.BlockData{}
	if err := block.UnmarshallBlock(blockBuffer.Bytes()); err != nil {
		return nil, err
	}
	transactions := make([]*transaction.Transaction, 0)
	for i := 0; i < len(block.TxFullData); i++ {
		t, err := transaction.UnmarshallTransaction(bytes.NewBuffer(block.TxFullData[i]))
		if err != nil {
			if t != nil && t.Hash() != nil {
				transaction.MarkTransactionBad(t.Hash(), err.Error())
			}
			return nil, err
		}
		transactions = append(transactions, t)
	}

	return &Block{
		BlockData:         block,
		PrevRollbacksHash: block.PrevHeader.RollbacksHash,
		Transactions:      transactions,
	}, nil
}
