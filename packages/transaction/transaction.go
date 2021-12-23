/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"math/rand"

	"github.com/shopspring/decimal"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/IBAX-io/go-ibax/packages/types"
)

// Transaction is a structure for parsing transactions
type Transaction struct {
	Notifications  types.Notifications
	GenBlock       bool
	SysUpdate      bool
	RollBackTx     []*sqldb.RollbackTx
	BlockData      *types.BlockData
	PreBlockData   *types.BlockData
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
	TransactionInfoer
}

type TransactionInfoer interface {
	txType() byte
	txHash() []byte
	txPayload() []byte
	txTime() int64
	txKeyID() int64
	txExpedite() decimal.Decimal
}

func (t *Transaction) TxType() byte                { return t.Inner.txType() }
func (t *Transaction) TxHash() []byte              { return t.Inner.txHash() }
func (t *Transaction) TxPayload() []byte           { return t.Inner.txPayload() }
func (t *Transaction) TxTime() int64               { return t.Inner.txTime() }
func (t *Transaction) TxKeyID() int64              { return t.Inner.txKeyID() }
func (t *Transaction) TxExpedite() decimal.Decimal { return t.Inner.txExpedite() }

func (t *Transaction) IsSmartContract() bool {
	_, ok := t.Inner.(*SmartContractTransaction)
	return ok
}

func (t *Transaction) SmartContract() *SmartContractTransaction {
	return t.Inner.(*SmartContractTransaction)
}

// UnmarshallTransaction is unmarshalling transaction
func UnmarshallTransaction(buffer *bytes.Buffer, fillData bool) (*Transaction, error) {
	tx := &Transaction{}
	if err := tx.Unmarshall(buffer); err != nil {
		return nil, err
	}
	txCache.Set(tx)

	return tx, nil
}
