/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/utils"
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
	nb, err = UnmarshallBlock(bytes.NewBuffer(newBlockData), true)
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

func UnmarshallBlock(blockBuffer *bytes.Buffer, fill bool) (*Block, error) {
	var (
		contractNames  []string
		classifyTxsMap = make(map[int][]*transaction.Transaction)
		block          = &types.BlockData{}
	)
	if err := block.UnmarshallBlock(blockBuffer.Bytes()); err != nil {
		return nil, err
	}

	if block.Header.BlockId != 1 {
		allDelayedContract, err := sqldb.GetAllDelayedContract()
		if err != nil {
			return nil, err
		}

		for _, contract := range allDelayedContract {
			contractNames = append(contractNames, contract.Contract)
		}
	}

	transactions := make([]*transaction.Transaction, 0)
	for i := 0; i < len(block.TxFullData); i++ {
		tx, err := transaction.UnmarshallTransaction(bytes.NewBuffer(block.TxFullData[i]), fill)
		if err != nil {
			return nil, err
		}
		if tx.Type() == types.StopNetworkTxType {
			classifyTxsMap[types.StopNetworkTxType] = append(classifyTxsMap[types.StopNetworkTxType], tx)
			transactions = append(transactions, tx)
			continue
		}
		if tx.IsSmartContract() {
			if tx.Type() == types.TransferSelfTxType {
				classifyTxsMap[types.TransferSelfTxType] = append(classifyTxsMap[types.TransferSelfTxType], tx)
				transactions = append(transactions, tx)
				continue
			}
			if tx.Type() == types.UtxoTxType {
				classifyTxsMap[types.UtxoTxType] = append(classifyTxsMap[types.UtxoTxType], tx)
				transactions = append(transactions, tx)
				continue
			}
			if utils.StringInSlice(contractNames, tx.SmartContract().TxContract.Name) {
				classifyTxsMap[types.DelayTxType] = append(classifyTxsMap[types.DelayTxType], tx)
				transactions = append(transactions, tx)
				continue
			}
			classifyTxsMap[types.SmartContractTxType] = append(classifyTxsMap[types.SmartContractTxType], tx)
		}
		transactions = append(transactions, tx)
	}

	return &Block{
		BlockData:         block,
		PrevRollbacksHash: block.PrevHeader.RollbacksHash,
		ClassifyTxsMap:    classifyTxsMap,
		Transactions:      transactions,
	}, nil
}
