/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"math/rand"

	"github.com/vmihailenco/msgpack/v5"

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

func (t Transaction) MarshallTransaction() ([]byte, error) {
	return msgpack.Marshal(t)
}

// UnmarshallTransaction is unmarshalling transaction
func UnmarshallTransaction(buffer *bytes.Buffer, fillData bool) (*Transaction, error) {
	tx := &Transaction{}
	if err := tx.Unmarshall(buffer); err != nil {
		return nil, err
	}
	return tx, nil
}
