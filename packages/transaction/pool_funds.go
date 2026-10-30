/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

// ErrPoolNoFunds refuses a transaction of a client or another node whose account has less than a
// block takes of it, beyond what its transactions in the pool take
var ErrPoolNoFunds = errors.New("the account has not the funds of the transaction, beyond those of its transactions in the pool")

// errPoolLedger is an error of the node reading a balance, not of the transaction
var errPoolLedger = errors.New("reading the balance of the transaction")

func ledgerError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", errPoolLedger, err)
}

// fundsBucket is a balance the transactions in the pool draw on: of an account, or of its unspent
// outputs, in an ecosystem
type fundsBucket struct {
	eco, key int64
	outputs  bool
}

// funds is what a transaction of a client or another node takes of a balance, which the pool
// checks before it takes the transaction: a block runs the transaction only when the balance has
// as much. The least fee of a contract, the value a transfer to oneself moves; a transfer of
// unspent outputs takes its fee of what they hold, so it needs some and holds none.
type funds struct {
	bucket  fundsBucket
	amount  decimal.Decimal // what the transaction holds of the balance while it is in the pool
	limit   decimal.Decimal // the most the account pays in a transaction
	balance decimal.Decimal
}

// ledger reads the balances the pool checks
type ledger interface {
	// account is the balance of the account and the most it pays in a transaction
	account(eco, key int64) (balance, limit decimal.Decimal, err error)
	// outputs is what the unspent outputs of the account hold, and how many they are
	outputs(eco, key int64) (decimal.Decimal, int64, error)
	digits(eco int64) (int32, error)
}

type dbLedger struct{}

func (dbLedger) account(eco, key int64) (decimal.Decimal, decimal.Decimal, error) {
	k := &sqldb.Key{}
	found, err := k.SetTablePrefix(eco).Get(nil, key)
	if err != nil || !found {
		return decimal.Zero, decimal.Zero, err
	}
	balance, _ := decimal.NewFromString(k.Amount)
	return balance, k.CapableAmount(), nil
}

func (dbLedger) outputs(eco, key int64) (decimal.Decimal, int64, error) {
	var row struct {
		Amount decimal.Decimal
		Count  int64
	}
	err := sqldb.GetDB(nil).Table((&sqldb.SpentInfo{}).TableName()).
		Select("coalesce(sum(output_value),'0') amount, count(*) count").
		Where("input_tx_hash is NULL AND output_key_id = ? AND ecosystem = ?", key, eco).Take(&row).Error
	return row.Amount, row.Count, err
}

func (dbLedger) digits(eco int64) (int32, error) {
	e := &sqldb.Ecosystem{}
	if _, err := e.Get(nil, eco); err != nil {
		return 0, err
	}
	return int32(e.Digits), nil
}

// poolLedger is the ledger of the pool of the node
var poolLedger ledger = dbLedger{}

// batchLedger reads each balance once for the transactions a batch brings; its errors are
// errPoolLedger
type batchLedger struct {
	ledger
	accounts map[fundsBucket][2]decimal.Decimal
	outs     map[fundsBucket]outputsRead
	digit    map[int64]int32
}

type outputsRead struct {
	amount decimal.Decimal
	count  int64
}

func newBatchLedger() *batchLedger {
	return &batchLedger{ledger: poolLedger, accounts: make(map[fundsBucket][2]decimal.Decimal),
		outs: make(map[fundsBucket]outputsRead), digit: make(map[int64]int32)}
}

func (b *batchLedger) account(eco, key int64) (decimal.Decimal, decimal.Decimal, error) {
	k := fundsBucket{eco: eco, key: key}
	if r, ok := b.accounts[k]; ok {
		return r[0], r[1], nil
	}
	balance, limit, err := b.ledger.account(eco, key)
	if err == nil {
		b.accounts[k] = [2]decimal.Decimal{balance, limit}
	}
	return balance, limit, ledgerError(err)
}

func (b *batchLedger) outputs(eco, key int64) (decimal.Decimal, int64, error) {
	k := fundsBucket{eco: eco, key: key, outputs: true}
	if r, ok := b.outs[k]; ok {
		return r.amount, r.count, nil
	}
	amount, count, err := b.ledger.outputs(eco, key)
	if err == nil {
		b.outs[k] = outputsRead{amount, count}
	}
	return amount, count, ledgerError(err)
}

func (b *batchLedger) digits(eco int64) (int32, error) {
	if d, ok := b.digit[eco]; ok {
		return d, nil
	}
	d, err := b.ledger.digits(eco)
	if err == nil {
		b.digit[eco] = d
	}
	return d, ledgerError(err)
}

// fundsOf is what the transaction takes of a balance, nil if none; it is ErrPoolNoFunds when the
// account has less than a block takes of it
func fundsOf(t *Transaction, l ledger) (*funds, error) {
	if !t.IsSmartContract() {
		return nil, nil
	}
	sc := t.SmartContract()
	ts := sc.TxSmart
	switch {
	case ts.TransferSelf != nil:
		self := ts.TransferSelf
		value, err := decimal.NewFromString(self.Value)
		if err != nil || !value.IsPositive() {
			return nil, ErrPoolNoFunds
		}
		f := &funds{bucket: fundsBucket{eco: ts.EcosystemID, key: ts.KeyID}, amount: value, limit: value}
		switch {
		case strings.EqualFold(self.Source, "Account") && strings.EqualFold(self.Target, "UTXO"):
			f.balance, _, err = l.account(f.bucket.eco, f.bucket.key)
		case strings.EqualFold(self.Source, "UTXO") && strings.EqualFold(self.Target, "Account"):
			f.bucket.outputs = true
			f.balance, _, err = l.outputs(f.bucket.eco, f.bucket.key)
		default:
			return nil, ErrPoolNoFunds
		}
		return f, err
	case ts.UTXO != nil:
		for _, eco := range []int64{ts.EcosystemID, consts.DefaultTokenEcosystem} {
			_, count, err := l.outputs(eco, ts.KeyID)
			if err != nil {
				return nil, err
			}
			if count == 0 {
				return nil, ErrPoolNoFunds
			}
		}
		return nil, nil
	}
	digits, err := l.digits(consts.DefaultTokenEcosystem)
	if err != nil {
		return nil, err
	}
	payer, fee, pays, err := sc.PoolFee(t.Inner.txSize(), digits)
	if err != nil || !pays {
		return nil, err
	}
	f := &funds{bucket: fundsBucket{eco: consts.DefaultTokenEcosystem, key: payer}, amount: fee}
	f.balance, f.limit, err = l.account(consts.DefaultTokenEcosystem, payer)
	return f, err
}

// refusal is how the pool takes an error of Check or of fundsOf: a bad transaction counts for the
// ban of its account, one without its funds does not, and an error of reading a balance is the
// node's, so the transaction is not bad
func refusal(err error) (bad, ban bool) {
	switch {
	case errors.Is(err, errPoolLedger):
		return false, false
	case errors.Is(err, ErrPoolNoFunds):
		return true, false
	}
	return true, true
}

// hold reports whether the balance of the transaction has its funds, beyond what the
// transactions in the pool hold of it
func (p *Pool) hold(f *funds) bool {
	if f == nil {
		return true
	}
	return f.amount.LessThanOrEqual(f.limit) && p.held[f.bucket].Add(f.amount).LessThanOrEqual(f.balance)
}
