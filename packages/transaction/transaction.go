/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"math/rand"

	"github.com/IBAX-io/go-ibax/packages/pbgo"
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
	TxResult       *pbgo.TxResult
	SqlDbSavePoint int
	FullData       []byte // full transaction, with type and data
	Inner          TransactionCaller
	TxInputs       []sqldb.SpentInfo
	TxOutputs      []sqldb.SpentInfo
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
func UnmarshallTransaction(buffer *bytes.Buffer) (*Transaction, error) {
	tx := &Transaction{}
	if err := tx.Unmarshall(buffer); err != nil {
		return nil, err
	}
	return tx, nil
}

func (tr *Transaction) WithOption(
	notifications types.Notifications,
	genBlock bool,
	blockHeader, preBlockHeader *types.BlockHeader,
	dbTransaction *sqldb.DbTransaction,
	rand *rand.Rand,
	txCheckLimits *Limits,
	sqlDbSavePoint int,
	txInputs []sqldb.SpentInfo,
	opts ...TransactionOption) error {
	tr.Notifications = notifications
	tr.GenBlock = genBlock
	tr.BlockHeader = blockHeader
	tr.PreBlockHeader = preBlockHeader
	tr.DbTransaction = dbTransaction
	tr.DbTransaction.ExecutionSql = nil
	tr.Rand = rand
	tr.TxCheckLimits = txCheckLimits
	tr.SqlDbSavePoint = sqlDbSavePoint
	tr.TxResult = &pbgo.TxResult{Hash: tr.Hash()}
	tr.TxInputs = txInputs
	return tr.Apply(opts...)
}

type TransactionOption func(b *Transaction) error

func (tr *Transaction) Apply(opts ...TransactionOption) error {
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(tr); err != nil {
			return err
		}
	}
	return nil
}
