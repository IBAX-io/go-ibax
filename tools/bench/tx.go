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
	// time is the time of the headers: pre-signed transactions of a run share it, and differ in
	// their content
	time int64
}

func newSigner(info *chainInfo) *signer {
	return &signer{networkID: info.NetworkID, time: time.Now().Unix()}
}

// sign fills the header of tx with the account and signs it: the hash is the double hash of the
// payload, and the signature is of the hash, as AccountKey.Verify checks it. The header carries
// the key of an account that has not registered it only.
func (s *signer) sign(a *account, tx *types.SmartTransaction) (*signedTx, error) {
	if tx.Header == nil {
		tx.Header = &types.Header{}
	}
	tx.EcosystemID = 1
	tx.KeyID = a.keyID
	tx.Time = s.time
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
	signature, err := crypto.AccountSign(a.algo, a.private, hash)
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

func (s *signer) transferSelf(a *account, value, source, target string) (*signedTx, error) {
	return s.sign(a, &types.SmartTransaction{TransferSelf: &types.TransferSelf{Value: value, Source: source, Target: target}})
}

func (s *signer) utxo(a *account, toID int64, value, comment string) (*signedTx, error) {
	return s.sign(a, &types.SmartTransaction{UTXO: &types.UTXO{ToID: toID, Value: value, Comment: comment}})
}
