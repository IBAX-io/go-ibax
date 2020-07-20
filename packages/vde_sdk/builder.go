/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package vde_sdk

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"

	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/IBAX-io/go-ibax/packages/crypto"
)

// Header is contain header data
type Header struct {
	ID          int
// SmartContract is storing smart contract data
type SmartContract struct {
	Header
	TokenEcosystem int64
	MaxSum         string
	PayOver        string
	SignedBy       int64
	Params         map[string]interface{}
}

func newTransaction(smartTx SmartContract, privateKey []byte, internal bool) (data, hash []byte, err error) {
	var publicKey []byte
	if publicKey, err = crypto.PrivateToPublic(privateKey); err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("converting node private key to public")
		return
	}
	smartTx.PublicKey = publicKey

	if internal {
		smartTx.SignedBy = crypto.Address(publicKey)
	}

	if data, err = msgpack.Marshal(smartTx); err != nil {
		log.WithFields(log.Fields{"type": consts.MarshallingError, "error": err}).Error("marshalling smart contract to msgpack")
		return
	}
	hash = crypto.DoubleHash(data)
	signature, err := crypto.Sign(privateKey, hash)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("signing by node private key")
		return
	}

	data = append(append([]byte{128}, converter.EncodeLengthPlusData(data)...), converter.EncodeLengthPlusData(signature)...)
	return
}

func NewInternalTransaction(smartTx SmartContract, privateKey []byte) (data, hash []byte, err error) {
	return newTransaction(smartTx, privateKey, true)
}

func NewTransaction(smartTx SmartContract, privateKey []byte) (data, hash []byte, err error) {
	return newTransaction(smartTx, privateKey, false)
}

/*
// CreateTransaction creates transaction
func CreateTransaction(data, hash []byte, keyID int64) error {
	tx := &model.Transaction{
		Hash:     hash,
		Data:     data[:],
		Type:     1,
		KeyID:    keyID,
		HighRate: model.TransactionRateOnBlock,
	}
	if err := tx.Create(); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating new transaction")
		return err
	}
	return nil
}*/
