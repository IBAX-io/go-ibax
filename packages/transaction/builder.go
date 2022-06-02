/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
)

func newTransaction(smartTx types.SmartTransaction, privateKey []byte, internal bool) (data, hash []byte, err error) {
	stp := &SmartTransactionParser{
		SmartContract: &smart.SmartContract{TxSmart: new(types.SmartTransaction)},
	}
	data, err = stp.BinMarshalWithPrivate(&smartTx, privateKey, internal)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.MarshallingError, "error": err}).Error("marshalling smart contract to msgpack")
		return
	}
	hash = stp.Hash
	return

}

func NewInternalTransaction(smartTx types.SmartTransaction, privateKey []byte) (data, hash []byte, err error) {
	return newTransaction(smartTx, privateKey, true)
}

func NewTransactionInProc(smartTx types.SmartTransaction, privateKey []byte) (data, hash []byte, err error) {
	return newTransaction(smartTx, privateKey, false)
}

// CreateTransaction creates transaction
func CreateTransaction(data, hash []byte, keyID, tnow int64) error {
	tx := &sqldb.Transaction{
		Hash:     hash,
		Data:     data[:],
		Type:     types.SmartContractTxType,
		KeyID:    keyID,
		HighRate: sqldb.TransactionRateOnBlock,
		Time:     tnow,
	}
	if err := tx.Create(nil); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating new transaction")
		return err
	}
	return nil
}

// CreateDelayTransactionHighRate creates transaction
func CreateDelayTransactionHighRate(data, hash []byte, keyID, highRate int64) *sqldb.Transaction {

	t := int8(highRate)
	tx := &sqldb.Transaction{
		Hash:     hash,
		Data:     data[:],
		Type:     getTxTxType(t),
		KeyID:    keyID,
		HighRate: sqldb.GetTxRateByTxType(t),
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
