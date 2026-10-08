/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/transaction"
)

const transfersSource = "go-ibax transaction.(*Transaction).Unmarshall (client tx type 0x80) then " +
	"SmartTransactionParser.Validate (SmartTransaction.Validate, utils.CheckSign) via tools/cryptovectors; " +
	"data is written by the client. Test keys only."

type transfersFile struct {
	Source string         `json:"source"`
	Cases  []transferCase `json:"cases"`
}

type transferCase struct {
	// client side
	Cryptoer        string          `json:"cryptoer"`
	Hasher          string          `json:"hasher"`
	NetworkID       int64           `json:"networkID"`
	SignedNetworkID *int64          `json:"signedNetworkID,omitempty"`
	Tampered        bool            `json:"tampered,omitempty"`
	EcosystemID     int64           `json:"ecosystemID"`
	Time            int64           `json:"time"`
	PrivateKey      string          `json:"privateKey"`
	Payload         json.RawMessage `json:"payload"`
	Hash            string          `json:"hash"`
	Data            string          `json:"data"`
	// node side
	Node transferVerdict `json:"node"`
}

type transferVerdict struct {
	Error               string         `json:"error,omitempty"`
	Type                int            `json:"type,omitempty"`
	Hash                string         `json:"hash,omitempty"`
	KeyID               string         `json:"keyID,omitempty"`
	KeyMatchesPublicKey bool           `json:"keyMatchesPublicKey,omitempty"`
	EcosystemID         string         `json:"ecosystemID,omitempty"`
	NetworkID           string         `json:"networkID,omitempty"`
	UTXO                *utxoVerdict   `json:"utxo,omitempty"`
	TransferSelf        *selfTxVerdict `json:"transferSelf,omitempty"`
}

type utxoVerdict struct {
	ToID    string `json:"toID"`
	Value   string `json:"value"`
	Comment string `json:"comment"`
}

type selfTxVerdict struct {
	Value  string `json:"value"`
	Source string `json:"source"`
	Target string `json:"target"`
}

func updateTransfers(raw []byte) (any, []string, error) {
	var f transfersFile
	if err := decode(raw, &f); err != nil {
		return nil, nil, err
	}
	f.Source = transfersSource
	for i := range f.Cases {
		c := &f.Cases[i]
		if err := useSuite(c.Cryptoer, c.Hasher); err != nil {
			return nil, nil, err
		}
		data, err := hex.DecodeString(c.Data)
		if err != nil {
			return nil, nil, fmt.Errorf("case %d: data: %w", i, err)
		}
		c.Node = nodeAcceptsTransfer(c.NetworkID, data)
	}
	return f, nil, nil
}

// nodeAcceptsTransfer decodes and validates a client transaction the way a node on networkID does
// before it queues it.
func nodeAcceptsTransfer(networkID int64, data []byte) (verdict transferVerdict) {
	conf.Config.LocalConf.NetworkID = networkID
	defer func() {
		if r := recover(); r != nil {
			verdict = transferVerdict{Error: fmt.Sprintf("panic: %v", r)}
		}
	}()

	tx := &transaction.Transaction{}
	if err := tx.Unmarshall(bytes.NewBuffer(data), true); err != nil {
		return transferVerdict{Error: err.Error()}
	}
	if !tx.IsSmartContract() {
		return transferVerdict{Error: fmt.Sprintf("not a client transaction: type %d", tx.Type())}
	}
	parser := tx.SmartContract()
	if err := parser.Validate(); err != nil {
		return transferVerdict{Error: err.Error()}
	}

	smartTx := parser.TxSmart
	verdict = transferVerdict{
		Type:                int(tx.Type()),
		Hash:                hex.EncodeToString(tx.Hash()),
		KeyID:               strconv.FormatInt(smartTx.KeyID, 10),
		KeyMatchesPublicKey: crypto.Address(smartTx.PublicKey) == smartTx.KeyID,
		EcosystemID:         strconv.FormatInt(smartTx.EcosystemID, 10),
		NetworkID:           strconv.FormatInt(smartTx.NetworkID, 10),
	}
	if smartTx.UTXO != nil {
		verdict.UTXO = &utxoVerdict{
			ToID:    strconv.FormatInt(smartTx.UTXO.ToID, 10),
			Value:   smartTx.UTXO.Value,
			Comment: smartTx.UTXO.Comment,
		}
	}
	if smartTx.TransferSelf != nil {
		verdict.TransferSelf = &selfTxVerdict{
			Value:  smartTx.TransferSelf.Value,
			Source: smartTx.TransferSelf.Source,
			Target: smartTx.TransferSelf.Target,
		}
	}
	return verdict
}
