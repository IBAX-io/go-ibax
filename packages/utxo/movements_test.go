/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package utxo

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	alice    = 100
	bob      = 200
	producer = 300
	taxes    = 400
)

func output(key int64, value string, kind int32) sqldb.SpentInfo {
	return sqldb.SpentInfo{OutputKeyId: key, OutputValue: value, Type: kind, Ecosystem: 1}
}

// The outputs of a transfer of 1000 from alice to bob, out of alice's 5000, paying 30 and 10 of taxes
var transferOutputs = []sqldb.SpentInfo{
	output(producer, "30", consts.UTXO_Type_Packaging),
	output(taxes, "10", consts.UTXO_Type_Taxes),
	output(bob, "1000", consts.UTXO_Type_Transfer),
	output(alice, "3960", consts.UTXO_Type_Output),
}

var transfer = txFacts{Type: types.UtxoTxType, Sender: alice, Ecosystem: 1, ToID: bob, Value: "1000", Comment: "rent"}

func TestClassifyTransfer(t *testing.T) {
	sent, err := classify(alice, 1, transfer, transferOutputs, []sqldb.SpentInfo{output(alice, "5000", consts.UTXO_Type_Transfer)})
	require.NoError(t, err)
	assert.Equal(t, Movement{Kind: KindTransfer, Sender: "100", Recipient: "200", Amount: "1000", Fee: "40",
		Comment: "rent", Net: "-1040"}, sent)

	received, err := classify(bob, 1, transfer, transferOutputs, nil)
	require.NoError(t, err)
	assert.Equal(t, Movement{Kind: KindTransfer, Sender: "100", Recipient: "200", Amount: "1000", Fee: "40",
		Comment: "rent", Net: "1000"}, received)
}

func TestClassifyReward(t *testing.T) {
	packaging, err := classify(producer, 1, transfer, transferOutputs, nil)
	require.NoError(t, err)
	assert.Equal(t, Movement{Kind: KindReward, Sender: "100", Recipient: "300", Amount: "30", Fee: "40", Net: "30"}, packaging)

	tax, err := classify(taxes, 1, transfer, transferOutputs, nil)
	require.NoError(t, err)
	assert.Equal(t, KindReward, tax.Kind)
	assert.Equal(t, "10", tax.Net)
}

// A recipient who also made the block has both in its net
func TestClassifyTransferToTheProducer(t *testing.T) {
	outputs := []sqldb.SpentInfo{
		output(bob, "30", consts.UTXO_Type_Packaging),
		output(taxes, "10", consts.UTXO_Type_Taxes),
		output(bob, "1000", consts.UTXO_Type_Transfer),
	}
	received, err := classify(bob, 1, transfer, outputs, nil)
	require.NoError(t, err)
	assert.Equal(t, KindTransfer, received.Kind)
	assert.Equal(t, "1030", received.Net)
}

// A transfer in ecosystem 2 pays its ecosystem 1 fee from the sender's ecosystem 1 outputs
func TestClassifyFeeOfAnotherEcosystem(t *testing.T) {
	facts := txFacts{Type: types.UtxoTxType, Sender: alice, Ecosystem: 2, ToID: bob, Value: "7", Comment: "rent"}
	outputs := []sqldb.SpentInfo{
		output(producer, "3", consts.UTXO_Type_Packaging),
		output(taxes, "1", consts.UTXO_Type_Taxes),
		output(alice, "96", consts.UTXO_Type_Output),
	}
	paid, err := classify(alice, 1, facts, outputs, []sqldb.SpentInfo{output(alice, "100", consts.UTXO_Type_Output)})
	require.NoError(t, err)
	assert.Equal(t, Movement{Kind: KindFee, Sender: "100", Recipient: "200", Amount: "0", Fee: "4", Comment: "rent", Net: "-4"}, paid)

	reward, err := classify(producer, 1, facts, outputs, nil)
	require.NoError(t, err)
	assert.Equal(t, KindReward, reward.Kind)
	assert.Equal(t, "3", reward.Amount)
}

func TestClassifyMoves(t *testing.T) {
	toUTXO := txFacts{Type: types.TransferSelfTxType, Sender: alice, Ecosystem: 1, Value: "500", Target: "utxo"}
	in, err := classify(alice, 1, toUTXO, []sqldb.SpentInfo{output(alice, "500", consts.UTXO_Type_Self_Account)}, nil)
	require.NoError(t, err)
	assert.Equal(t, Movement{Kind: KindMove, To: "utxo", Sender: "100", Recipient: "100", Amount: "500", Fee: "0", Net: "500"}, in)

	toAccount := txFacts{Type: types.TransferSelfTxType, Sender: alice, Ecosystem: 1, Value: "200", Target: "account"}
	out, err := classify(alice, 1, toAccount, []sqldb.SpentInfo{output(alice, "300", consts.UTXO_Type_Self_UTXO)},
		[]sqldb.SpentInfo{output(alice, "500", consts.UTXO_Type_Self_Account)})
	require.NoError(t, err)
	assert.Equal(t, Movement{Kind: KindMove, To: "account", Sender: "100", Recipient: "100", Amount: "200", Fee: "0", Net: "-200"}, out)

	// Moving all of it leaves no change
	all, err := classify(alice, 1, toAccount, nil, []sqldb.SpentInfo{output(alice, "200", consts.UTXO_Type_Self_Account)})
	require.NoError(t, err)
	assert.Equal(t, "-200", all.Net)

	_, err = classify(alice, 1, txFacts{Type: types.TransferSelfTxType, Sender: alice, Value: "1", Target: "elsewhere"}, nil, nil)
	assert.Error(t, err)
}

func TestClassifyGenesis(t *testing.T) {
	genesis, err := classify(alice, 1, txFacts{Type: types.FirstBlockTxType, Sender: alice},
		[]sqldb.SpentInfo{output(alice, "2100000000", consts.UTXO_Type_First_Block)}, nil)
	require.NoError(t, err)
	assert.Equal(t, Movement{Kind: KindGenesis, Sender: "0", Recipient: "100", Amount: "2100000000", Fee: "0", Net: "2100000000"}, genesis)
}

// Outputs the transaction cannot have made are the node's data gone wrong, never a guess
func TestClassifyRefusesTheUnexplained(t *testing.T) {
	_, err := classify(bob, 1, txFacts{Type: types.SmartContractTxType, Sender: alice}, []sqldb.SpentInfo{output(bob, "1", consts.UTXO_Type_Transfer)}, nil)
	assert.Error(t, err)

	_, err = classify(bob, 1, transfer, []sqldb.SpentInfo{output(bob, "x", consts.UTXO_Type_Transfer)}, nil)
	assert.Error(t, err)
}

func TestMovementsRefusesALimitOutOfRange(t *testing.T) {
	for _, limit := range []int{0, MaxLimit + 1} {
		_, err := Movements(alice, 1, 0, limit)
		assert.Error(t, err)
	}
}
