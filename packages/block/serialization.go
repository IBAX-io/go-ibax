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

func MarshallBlock(header, prev *types.BlockData, trData [][]byte) ([]byte, error) {
	block := &types.Block{
		Header:     *header,
		PrevHeader: prev,
		TxFullData: trData,
	}
	return block.MarshallBlock(syspar.GetNodePrivKey())
}

func UnmarshallBlock(blockBuffer *bytes.Buffer, fillData bool) (*Block, error) {
	block := &types.Block{}
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
		t.BlockData = &block.Header
		transactions = append(transactions, t)
	}

	return &Block{
		BinData:           block.BinData,
		Header:            block.Header,
		PrevRollbacksHash: block.PrevHeader.RollbacksHash,
		Transactions:      transactions,
		MrklRoot:          block.MerkleRoot,
	}, nil
}
