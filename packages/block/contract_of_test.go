/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// The transfers of the UTXO model run no contract but insert their history as one does: each kind
// has its own name the state learns the ids it takes by, which no contract has
func TestContractOf(t *testing.T) {
	tx := func(c *smart.Contract, s *types.SmartTransaction) *transaction.Transaction {
		return &transaction.Transaction{Inner: &transaction.SmartTransactionParser{SmartContract: &smart.SmartContract{TxContract: c, TxSmart: s}}}
	}
	for _, c := range []struct {
		t    *transaction.Transaction
		want string
	}{
		{tx(&smart.Contract{Name: "@1TokensSend"}, &types.SmartTransaction{}), "@1TokensSend"},
		{tx(nil, &types.SmartTransaction{TransferSelf: &types.TransferSelf{Value: "1", Source: "UTXO", Target: "Account"}}), "transfer self"},
		{tx(nil, &types.SmartTransaction{TransferSelf: &types.TransferSelf{Value: "1", Source: "Account", Target: "UTXO"}}), "transfer self"},
		{tx(nil, &types.SmartTransaction{UTXO: &types.UTXO{ToID: 1, Value: "1"}}), "utxo transfer"},
		{tx(nil, &types.SmartTransaction{}), ""},
		{tx(nil, nil), ""},
		{&transaction.Transaction{}, ""},
	} {
		if got := contractOf(c.t); got != c.want {
			t.Errorf("contractOf = %q, want %q", got, c.want)
		}
	}
}
