/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
)

func (rtx *Transaction) Unmarshall(buffer *bytes.Buffer, fill bool) error {
	if buffer.Len() == 0 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("empty transaction buffer")
		return fmt.Errorf("empty transaction buffer")
	}
	rtx.FullData = buffer.Bytes()
	rtx.InToCxt = &InToCxt{
		DbTransaction: new(sqldb.DbTransaction),
	}
	txT, err := buffer.ReadByte()
	if err != nil {
		return err
	}

	var inner TransactionCaller
	switch txT {
	case types.SmartContractTxType, types.TransferSelfTxType, types.UtxoTxType:
		itx := &SmartTransactionParser{
			SmartContract: &smart.SmartContract{TxSmart: new(types.SmartTransaction)},
		}
		inner = itx
		if err = itx.Unmarshal(buffer, fill); err != nil {
			return err
		}
	case byte(128): //reset unmarshal client buf
		itx := &SmartTransactionParser{
			SmartContract: &smart.SmartContract{TxSmart: new(types.SmartTransaction)},
		}
		inner = itx
		if err := converter.BinUnmarshalBuff(buffer, &itx.Payload); err != nil {
			return err
		}
		itx.Hash = crypto.DoubleHash(itx.Payload)
		itx.TxSignature = buffer.Bytes()
		if err := msgpack.Unmarshal(itx.Payload, &itx.TxSmart); err != nil {
			return err
		}

		var newbuf []byte
		newbuf, err = itx.Marshal()
		if err != nil {
			return err
		}
		if err = itx.Unmarshal(bytes.NewBuffer(newbuf), fill); err != nil {
			return err
		}

		rtx.FullData = newbuf
	case types.FirstBlockTxType:
		var itx FirstBlockParser
		inner = &itx
		if err := itx.Unmarshal(buffer); err != nil {
			log.WithFields(log.Fields{"error": err, "type": consts.UnmarshallingError, "tx_type": itx.txType()}).Error("getting parser for tx type")
			return err
		}
	case types.StopNetworkTxType:
		var itx = StopNetworkParser{}
		inner = &itx

		if err := itx.Unmarshal(buffer); err != nil {
			log.WithFields(log.Fields{"error": err, "type": consts.UnmarshallingError, "tx_type": rtx.Type()}).Error("getting parser for tx type")
			return err
		}
	default:
		return fmt.Errorf("unsupported tx type %d", txT)
	}
	rtx.Inner = inner
	if cache, ok := txCache.Get(fmt.Sprintf("%x", rtx.Hash())); ok {
		rtx = cache
		return nil
	}
	txCache.Set(rtx)
	return nil
}

func (rtx *Transaction) SetRawTx() *sqldb.RawTx {
	return &sqldb.RawTx{
		Hash:     rtx.Hash(),
		Time:     rtx.Timestamp(),
		TxType:   rtx.Type(),
		Data:     rtx.FullData,
		Expedite: rtx.Expedite().String(),
		WalletID: rtx.KeyID(),
	}
}
