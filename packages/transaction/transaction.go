/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"fmt"
	"math/rand"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
)

// Transaction is a structure for parsing transactions
type Transaction struct {
	FullData []byte // full transaction, with type and data
	*InToCxt
	*OutCtx
	Inner TransactionCaller
}

// TransactionCaller is parsing transactions
type TransactionCaller interface {
	Init(*InToCxt) error
	Validate() error
	Action(*InToCxt, *OutCtx) error
	TxRollback() error
	txType() byte
	txHash() []byte
	txPayload() []byte
	txTime() int64
	txKeyID() int64
	txExpedite() decimal.Decimal
	// txSize is the length of the transaction as a block stores it (Transaction.FullData).
	// Size limits and size-based fees count it.
	txSize() int64
	setTxSize(int64)
}

// storedSize keeps the length of a transaction as a block stores it. Parsers embed it; its field
// is unexported, so it is not part of their msgpack encoding.
type storedSize struct{ size int64 }

func (s *storedSize) txSize() int64        { return s.size }
func (s *storedSize) setTxSize(size int64) { s.size = size }

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

// Verify checks a transaction when it enters the node, before it reaches the queue: the signature
// holds and the key it signs with is the one the header names. The transfer transactions run no
// contract, so before this their signature was checked nowhere on the way in; this is where the
// node first refuses a forged TransferSelf or UTXO. The check is off the chain and uses the key in
// the header; the account's registered key decides at execution. Genesis and network-stop
// transactions are authorized by other means.
func (t *Transaction) Verify() error {
	sc, ok := t.Inner.(*SmartTransactionParser)
	if !ok {
		return nil
	}
	if err := sc.Validate(); err != nil {
		return err
	}
	// A signer whose own key signs must name itself: the header key must be the key of KeyID.
	// A transaction signed by another key (a node for a delayed contract) is checked at execution.
	if sc.TxSmart.SignedBy != 0 || len(sc.TxSmart.PublicKey) == 0 {
		return nil
	}
	key, err := crypto.ParseAccountKey(sc.TxSmart.PublicKey)
	if err != nil {
		return err
	}
	if key.Address() != sc.TxSmart.KeyID {
		return fmt.Errorf("the key in the header is not that of key %d", sc.TxSmart.KeyID)
	}
	return nil
}

// UnmarshallTransaction is unmarshalling transaction
func UnmarshallTransaction(buffer *bytes.Buffer, fill bool) (*Transaction, error) {
	tx := &Transaction{}
	var err error
	defer func() {
		if err != nil && tx != nil {
			if tx.Inner == nil {
				return
			}
			_ = MarkTransactionBad(tx.Hash(), err.Error())
		}
	}()
	err = tx.Unmarshall(buffer, fill)
	if err != nil {
		return nil, fmt.Errorf("parse transaction error: %w", err)
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
	sqlDbSavePoint string,
	outputsMap map[sqldb.KeyUTXO][]sqldb.SpentInfo,
	prevSysPar map[string]string,
	ecoParams []sqldb.EcoParam,
	opts ...TransactionOption) error {
	in := &InToCxt{
		SqlDbSavePoint: sqlDbSavePoint,
		TxCheckLimits:  txCheckLimits,
		Rand:           rand,
		DbTransaction:  dbTransaction,
		PreBlockHeader: preBlockHeader,
		BlockHeader:    blockHeader,
		GenBlock:       genBlock,
		Notifications:  notifications,
		OutputsMap:     outputsMap,
		PrevSysPar:     prevSysPar,
		EcoParams:      ecoParams,
	}
	in.DbTransaction.BinLogSql = nil
	tr.InToCxt = in
	tr.OutCtx = &OutCtx{
		TxResult: &pbgo.TxResult{Hash: tr.Hash()},
	}
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
