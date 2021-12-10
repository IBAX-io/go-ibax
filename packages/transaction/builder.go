/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/IBAX-io/go-ibax/packages/model"

	"github.com/IBAX-io/go-ibax/packages/crypto"
	log "github.com/sirupsen/logrus"
)

func newTransaction(smartTx types.SmartContract, privateKey []byte, internal bool) (data, hash []byte, err error) {
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

	data = append(append([]byte{types.SmartContractTxType}, converter.EncodeLengthPlusData(data)...), converter.EncodeLengthPlusData(signature)...)
	return
}

func NewInternalTransaction(smartTx types.SmartContract, privateKey []byte) (data, hash []byte, err error) {
	return newTransaction(smartTx, privateKey, true)
}

func NewTransaction(smartTx types.SmartContract, privateKey []byte) (data, hash []byte, err error) {
	return newTransaction(smartTx, privateKey, false)
}

// CreateTransaction creates transaction
func CreateTransaction(data, hash []byte, keyID, tnow int64) error {
	tx := &model.Transaction{
		Hash:     hash,
		Data:     data[:],
		Type:     types.SmartContractTxType,
		KeyID:    keyID,
		HighRate: model.TransactionRateOnBlock,
		Time:     tnow,
	}
	if err := tx.Create(nil); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating new transaction")
		return err
	}
	return nil
}

// CreateDelayTransactionHighRate creates transaction
func CreateDelayTransactionHighRate(data, hash []byte, keyID, highRate int64) *model.Transaction {

	t := int8(highRate)
	tx := &model.Transaction{
		Hash:     hash,
		Data:     data[:],
		Type:     getTxTxType(t),
		KeyID:    keyID,
		HighRate: model.GetTxRateByTxType(t),
	}
	return tx
}

func getTxTxType(rate int8) int8 {
	ret := int8(1)
	switch rate {
	case types.SmartContractTxType, types.StopNetworkTxType:
		ret = rate
	default:
	}

	return ret
}
