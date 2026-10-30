/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/types"
)

type fakeLedger struct {
	balances map[fundsBucket]decimal.Decimal
	limits   map[fundsBucket]decimal.Decimal
	counts   map[fundsBucket]int64
	reads    int
	err      error
}

func newFakeLedger() *fakeLedger {
	return &fakeLedger{balances: map[fundsBucket]decimal.Decimal{}, limits: map[fundsBucket]decimal.Decimal{},
		counts: map[fundsBucket]int64{}}
}

func (l *fakeLedger) account(eco, key int64) (decimal.Decimal, decimal.Decimal, error) {
	l.reads++
	k := fundsBucket{eco: eco, key: key}
	limit, ok := l.limits[k]
	if !ok {
		limit = l.balances[k]
	}
	return l.balances[k], limit, l.err
}

func (l *fakeLedger) outputs(eco, key int64) (decimal.Decimal, int64, error) {
	l.reads++
	k := fundsBucket{eco: eco, key: key, outputs: true}
	return l.balances[k], l.counts[k], l.err
}

func (l *fakeLedger) digits(int64) (int32, error) { l.reads++; return 0, l.err }

// smartTx is a transaction of the key that calls a contract the wallet binds, 0 for none
func smartTx(hash string, key, wallet int64, contract, expedite string) *Transaction {
	now := time.Now().Unix()
	return &Transaction{
		FullData: []byte("data of " + hash),
		Inner: &SmartTransactionParser{SmartContract: &smart.SmartContract{
			Hash:      []byte(hash),
			Timestamp: now * 1000,
			TxContract: &smart.Contract{Name: contract, Block: &script.CodeBlock{
				Info: &script.ContractInfo{Owner: &script.OwnerInfo{WalletID: wallet}}}},
			TxSmart: &types.SmartTransaction{Header: &types.Header{KeyID: key, Time: now, EcosystemID: 1},
				Expedite: expedite},
		}},
	}
}

func selfTx(hash string, key int64, value, source, target string) *Transaction {
	t := smartTx(hash, key, 0, "", "")
	ts := t.SmartContract().TxSmart
	ts.TransferSelf = &types.TransferSelf{Value: value, Source: source, Target: target}
	t.SmartContract().TxContract = nil
	return t
}

func utxoTx(hash string, key, eco int64) *Transaction {
	t := smartTx(hash, key, 0, "", "")
	ts := t.SmartContract().TxSmart
	ts.UTXO = &types.UTXO{ToID: 1, Value: "1"}
	ts.EcosystemID = eco
	t.SmartContract().TxContract = nil
	return t
}

func admitFunded(t *testing.T, p *Pool, l ledger, tx *Transaction, want error) {
	t.Helper()
	f, err := fundsOf(tx, l)
	if err == nil {
		err = p.place(tx, RateClient, time.Now().Unix(), f)
	}
	if !errors.Is(err, want) {
		t.Fatalf("%s: %v, want %v", tx.Hash(), err, want)
	}
}

func account(key int64) fundsBucket { return fundsBucket{eco: 1, key: key} }

// A contract transaction holds its least fee of the account of the signer, or of the wallet the
// contract binds, while it is in the pool: the account takes as many as its balance pays for
func TestPoolFundsContract(t *testing.T) {
	poolLimits(t, 100, 1<<20, 100, 10)
	l := newFakeLedger()
	// the least fee is 1 of the size, 2 with an expedite of 1, of 0 digits
	l.balances[account(1)] = decimal.NewFromInt(3)
	l.balances[account(9)] = decimal.NewFromInt(1)
	p := NewPool()
	admitFunded(t, p, l, smartTx("a", 1, 0, "Any", "1"), nil)
	admitFunded(t, p, l, smartTx("b", 1, 0, "Any", ""), nil)
	admitFunded(t, p, l, smartTx("c", 1, 0, "Any", ""), ErrPoolNoFunds)
	admitFunded(t, p, l, smartTx("none", 2, 0, "Any", ""), ErrPoolNoFunds)
	admitFunded(t, p, l, smartTx("bound", 2, 9, "Any", ""), nil)
	admitFunded(t, p, l, smartTx("bound2", 3, 9, "Any", ""), ErrPoolNoFunds)
	if got := p.held[account(1)]; !got.Equal(decimal.NewFromInt(3)) {
		t.Fatalf("held %v", got)
	}
	p.Remove([][]byte{[]byte("a")})
	if got := p.held[account(1)]; !got.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("held after the block of a %v", got)
	}
	admitFunded(t, p, l, smartTx("c", 1, 0, "Any", ""), nil)
	p.Bad([]byte("b"), "bad")
	p.Remove([][]byte{[]byte("c"), []byte("bound")})
	if len(p.held) != 0 {
		t.Fatalf("held after removal %v", p.held)
	}
}

// The most an account pays in a transaction bounds the fee of each, not of all
func TestPoolFundsLimit(t *testing.T) {
	poolLimits(t, 100, 1<<20, 100, 10)
	l := newFakeLedger()
	l.balances[account(1)] = decimal.NewFromInt(10)
	l.limits[account(1)] = decimal.NewFromInt(1)
	p := NewPool()
	admitFunded(t, p, l, smartTx("a", 1, 0, "Any", "1"), ErrPoolNoFunds)
	for _, h := range []string{"b", "c", "d"} {
		admitFunded(t, p, l, smartTx(h, 1, 0, "Any", ""), nil)
	}
}

// An evidence a validator signs pays nothing, so it needs no balance, as a block runs it
func TestPoolFundsFree(t *testing.T) {
	poolLimits(t, 100, 1<<20, 100, 10)
	l := newFakeLedger()
	p := NewPool()
	tx := smartTx("evidence", 1, 0, smart.BftEvidenceContract, "")
	tx.SmartContract().TxSmart.SignedBy = 5
	admitFunded(t, p, l, tx, nil)
	if p.Len() != 1 || len(p.held) != 0 {
		t.Fatalf("pool %d, held %v", p.Len(), p.held)
	}
}

// A transfer to oneself holds its value of the balance it moves
func TestPoolFundsTransferSelf(t *testing.T) {
	poolLimits(t, 100, 1<<20, 100, 10)
	l := newFakeLedger()
	l.balances[account(1)] = decimal.NewFromInt(5)
	l.balances[fundsBucket{eco: 1, key: 1, outputs: true}] = decimal.NewFromInt(2)
	p := NewPool()
	admitFunded(t, p, l, selfTx("a", 1, "3", "Account", "UTXO"), nil)
	admitFunded(t, p, l, selfTx("b", 1, "3", "Account", "UTXO"), ErrPoolNoFunds)
	admitFunded(t, p, l, selfTx("c", 1, "2", "account", "utxo"), nil)
	admitFunded(t, p, l, selfTx("d", 1, "2", "UTXO", "Account"), nil)
	admitFunded(t, p, l, selfTx("e", 1, "1", "UTXO", "Account"), ErrPoolNoFunds)
	admitFunded(t, p, l, selfTx("zero", 1, "0", "Account", "UTXO"), ErrPoolNoFunds)
	admitFunded(t, p, l, selfTx("nan", 1, "x", "Account", "UTXO"), ErrPoolNoFunds)
	admitFunded(t, p, l, selfTx("where", 1, "1", "Account", "Account"), ErrPoolNoFunds)
}

// A transfer of unspent outputs takes its fee of them, so the account needs some in ecosystem 1
// and in that of the transfer
func TestPoolFundsUTXO(t *testing.T) {
	poolLimits(t, 100, 1<<20, 100, 10)
	l := newFakeLedger()
	l.counts[fundsBucket{eco: 1, key: 1, outputs: true}] = 1
	l.counts[fundsBucket{eco: 2, key: 2, outputs: true}] = 1
	l.counts[fundsBucket{eco: 1, key: 3, outputs: true}] = 1
	l.counts[fundsBucket{eco: 2, key: 3, outputs: true}] = 1
	p := NewPool()
	admitFunded(t, p, l, utxoTx("a", 1, 1), nil)
	admitFunded(t, p, l, utxoTx("b", 1, 2), ErrPoolNoFunds)
	admitFunded(t, p, l, utxoTx("c", 2, 2), ErrPoolNoFunds)
	admitFunded(t, p, l, utxoTx("d", 3, 2), nil)
	admitFunded(t, p, l, utxoTx("e", 4, 1), ErrPoolNoFunds)
}

// A batch reads each balance once, and an error of reading one is the node's, not the
// transaction's
func TestPoolFundsLedger(t *testing.T) {
	l := newFakeLedger()
	l.balances[account(1)] = decimal.NewFromInt(5)
	saved := poolLedger
	t.Cleanup(func() { poolLedger = saved })
	poolLedger = l
	b := newBatchLedger()
	for _, h := range []string{"a", "b", "c"} {
		if _, err := fundsOf(smartTx(h, 1, 0, "Any", ""), b); err != nil {
			t.Fatal(err)
		}
	}
	if l.reads != 2 {
		t.Fatalf("reads %d", l.reads)
	}
	l.err = errors.New("no database")
	_, err := fundsOf(smartTx("d", 2, 0, "Any", ""), b)
	if !errors.Is(err, errPoolLedger) {
		t.Fatalf("an error of the ledger: %v", err)
	}
	for _, c := range []struct {
		err       error
		bad, bans bool
	}{{err, false, false}, {ErrPoolNoFunds, true, false}, {ErrDuplicatedTx, true, true}} {
		if bad, ban := refusal(c.err); bad != c.bad || ban != c.bans {
			t.Fatalf("%v: bad %v, ban %v", c.err, bad, ban)
		}
	}
}
