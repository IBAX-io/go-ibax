/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"encoding/hex"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// clientTxType is the type byte of a transaction a client sends: its payload, then its signature
const clientTxType = 0x80

// signedTx is a transaction ready to send: its hash in hex and its bytes
type signedTx struct {
	hash string
	data []byte
}

// signer signs the transactions of the accounts for a network
type signer struct {
	networkID int64
	// clock tells the time of the header of each transaction it signs: a node takes a transaction
	// only within consts.MaxTxBack of the time of the block, so a run of any length stamps each
	// transaction when it signs it, not once for the process
	clock func() int64
}

func newSigner(info *chainInfo) *signer {
	return &signer{networkID: info.NetworkID, clock: func() int64 { return time.Now().Unix() }}
}

// chainClock is the clock of a chain whose time was from at the start and goes speed seconds a
// second (faults.Config.ClockSpeed) until it reaches the system clock, as the fast clock of a test
// chain does
func chainClock(from int64, speed float64) func() int64 {
	start := time.Now()
	return func() int64 {
		now := time.Now()
		t := from + int64(now.Sub(start).Seconds()*speed)
		if speed > 1 && t > now.Unix() {
			t = now.Unix()
		}
		return t
	}
}

// sign fills the header of tx with the account and signs it in ecosystem 1
func (s *signer) sign(a *account, tx *types.SmartTransaction) (*signedTx, error) {
	return s.signIn(a, 1, tx)
}

// signIn fills the header of tx with the account and the ecosystem, whose tables the contracts
// read and write by names without one, and signs it: the hash is the double hash of the payload,
// and the signature is of the hash, as AccountKey.Verify checks it. The header carries the key of
// an account that has not registered it only.
func (s *signer) signIn(a *account, eco int64, tx *types.SmartTransaction) (*signedTx, error) {
	if tx.Header == nil {
		tx.Header = &types.Header{}
	}
	tx.EcosystemID = eco
	tx.KeyID = a.keyID
	tx.Time = s.clock()
	tx.NetworkID = s.networkID
	tx.PublicKey = nil
	if a.unregistered {
		tx.PublicKey = a.public.Bytes()
	}
	tx.Lang = "en"
	payload, err := tx.Marshal()
	if err != nil {
		return nil, err
	}
	hash := crypto.DoubleHash(payload)
	signature, err := crypto.AccountPrivateKey{Algo: a.algo, Raw: a.private}.Sign(hash)
	if err != nil {
		return nil, err
	}
	data := append([]byte{clientTxType}, converter.EncodeLengthPlusData(payload)...)
	data = append(data, converter.EncodeLengthPlusData(signature)...)
	return &signedTx{hash: hex.EncodeToString(hash), data: data}, nil
}

func (s *signer) contract(a *account, id int, params map[string]any) (*signedTx, error) {
	return s.sign(a, &types.SmartTransaction{Header: &types.Header{ID: id}, Params: params})
}

// contractIn signs a transaction of the contract id in the ecosystem eco
func (s *signer) contractIn(a *account, eco int64, id int, params map[string]any) (*signedTx, error) {
	return s.signIn(a, eco, &types.SmartTransaction{Header: &types.Header{ID: id}, Params: params})
}

func (s *signer) transferSelf(a *account, value, source, target string) (*signedTx, error) {
	return s.sign(a, &types.SmartTransaction{TransferSelf: &types.TransferSelf{Value: value, Source: source, Target: target}})
}

func (s *signer) utxo(a *account, toID int64, value, comment string) (*signedTx, error) {
	return s.sign(a, &types.SmartTransaction{UTXO: &types.UTXO{ToID: toID, Value: value, Comment: comment}})
}
