/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/smart"
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
// CreateTransaction puts a transaction the node made in the pool, first for the next block
func CreateTransaction(data []byte) error {
	tx, err := UnmarshallTransaction(bytes.NewBuffer(data), true)
	if err != nil {
		return err
	}
	if err = TxPool.Admit([]*Transaction{tx}, RateOnBlock)[0]; err != nil {
		log.WithFields(log.Fields{"type": consts.ParseError, "error": err}).Error("creating new transaction")
	}
	return err
}

// DelayedTransaction is a delayed contract's transaction the node made for the block it makes
func DelayedTransaction(data []byte, highRate int64) (*Pending, error) {
	tx, err := UnmarshallTransaction(bytes.NewBuffer(data), true)
	if err != nil {
		return nil, err
	}
	rate := RateClient
	if highRate == int64(types.StopNetworkTxType) {
		rate = RateStopNetwork
	}
	return &Pending{Tx: tx, Data: tx.FullData, Rate: rate}, nil
}
