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

// repeatMarshallBlock encodes the block this node generated, once it has run: the transactions of
// the block are those that ran well, in the order they ran, so they are not parsed again
func (b *Block) repeatMarshallBlock() error {
	ran := make(map[string]*transaction.Transaction, len(b.Transactions))
	for _, t := range b.Transactions {
		ran[string(t.FullData)] = t
	}
	txs := make([]*transaction.Transaction, 0, len(b.TxFullData))
	for _, data := range b.TxFullData {
		t, ok := ran[string(data)]
		if !ok {
			return errors.New("a transaction of the block is not one that ran")
		}
		txs = append(txs, t)
	}
	data := &types.BlockData{}
	if err := data.Apply(
		types.WithCurHeader(b.Header),
		types.WithPrevHeader(b.PrevHeader),
		types.WithAfterTxs(b.AfterTxs),
		types.WithSysUpdate(b.SysUpdate),
		types.WithTxFullData(b.TxFullData)); err != nil {
		return errors.Wrap(err, "marshalling repeat block")
	}
	newBlockData, err := data.MarshallBlock(syspar.GetNodePrivKey())
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
