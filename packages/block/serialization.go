/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
)

func MarshallBlock(opts ...types.BlockDataOption) ([]byte, error) {
	block := &types.BlockData{}
	if err := block.Apply(opts...); err != nil {
		return nil, err
	}
	return block.MarshallBlock(syspar.GetNodePrivKey())
}

func UnmarshallBlock(blockBuffer *bytes.Buffer, fillData bool) (*Block, error) {
	block := &types.BlockData{}
	if err := block.UnmarshallBlock(blockBuffer.Bytes()); err != nil {
		return nil, err
	}
	transactions := make([]*transaction.Transaction, 0)
	for i := 0; i < len(block.TxFullData); i++ {
		t, err := transaction.UnmarshallTransaction(bytes.NewBuffer(block.TxFullData[i]), fillData)
		if err != nil {
			if t != nil && t.Hash() != nil {
				transaction.MarkTransactionBad(t.DbTransaction, t.Hash(), err.Error())
			}
			return nil, fmt.Errorf("parse transaction error(%s)", err)
		}
		transactions = append(transactions, t)
	}

	return &Block{
		BlockData:         *block,
		PrevRollbacksHash: block.PrevHeader.RollbacksHash,
		Transactions:      transactions,
	}, nil
}
