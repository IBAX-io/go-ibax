/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"math/rand"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
)

// Transaction is a structure for parsing transactions
type Transaction struct {
	Notifications  types.Notifications
	GenBlock       bool
	SysUpdate      bool
	RollBackTx     []*types.RollbackTx
	BlockHeader    *types.BlockHeader
	PreBlockHeader *types.BlockHeader
	DbTransaction  *sqldb.DbTransaction
	Rand           *rand.Rand
	TxCheckLimits  *Limits
	TxResult       string
	SqlDbSavePoint int
	FullData       []byte // full transaction, with type and data
	Inner          TransactionCaller
}

// TransactionCaller is parsing transactions
type TransactionCaller interface {
	Init(*Transaction) error
	Validate() error
	Action(*Transaction) error
	TxRollback() error
	txType() byte
	txHash() []byte
	txPayload() []byte
	txTime() int64
	txKeyID() int64
	txExpedite() decimal.Decimal
}

func (t *Transaction) Type() byte                { return t.Inner.txType() }
func (t *Transaction) Hash() []byte              { return t.Inner.txHash() }
func (t *Transaction) Payload() []byte           { return t.Inner.txPayload() }
func (t *Transaction) Timestamp() int64          { return t.Inner.txTime() }
func (t *Transaction) KeyID() int64              { return t.Inner.txKeyID() }
func (t *Transaction) Expedite() decimal.Decimal { return t.Inner.txExpedite() }

func (t *Transaction) IsSmartContract() bool {
	_, ok := t.Inner.(*SmartTransactionParser)
	return ok
}

func (t *Transaction) SmartContract() *SmartTransactionParser {
	return t.Inner.(*SmartTransactionParser)
}

// UnmarshallTransaction is unmarshalling transaction
func UnmarshallTransaction(buffer *bytes.Buffer, fillData bool) (*Transaction, error) {
	tx := &Transaction{}
	if err := tx.Unmarshall(buffer); err != nil {
		return nil, err
	}
	return tx, nil
}

func (t *Transaction) BuildAfterTxs() *types.AfterTxs {
	after := &types.AfterTxs{
		Rts:         make([]*types.RollbackTx, 0),
		Lts:         make([]*types.LogTransaction, 0),
		UpdTxStatus: make([]*types.UpdateBlockMsg, 0),
	}
	after.UsedTx = append(after.UsedTx, t.Hash())
	after.TxExecutionSql = append(after.TxExecutionSql, t.DbTransaction.ExecutionSql...)
	var (
		eco      int64
		contract string
	)
	if t.IsSmartContract() {
		eco = t.SmartContract().TxSmart.EcosystemID
		contract = t.SmartContract().TxContract.Name
	}
	after.Lts = append(after.Lts, &types.LogTransaction{
		Block:        t.BlockHeader.BlockID,
		Hash:         t.Hash(),
		TxData:       t.FullData,
		Timestamp:    t.Timestamp(),
		Address:      t.KeyID(),
		EcosystemID:  eco,
		ContractName: contract,
	})
	after.UpdTxStatus = append(after.UpdTxStatus, &types.UpdateBlockMsg{
		Hash: t.Hash(),
		Msg:  t.TxResult,
	})
	after.Rts = append(after.Rts, t.RollBackTx...)
	return after
}
