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

// Unmarshall reads a transaction in its stored form, as a block holds it and nodes pass it on
func (rtx *Transaction) Unmarshall(buffer *bytes.Buffer, fill bool) error {
	return rtx.unmarshall(buffer, fill, false)
}

// UnmarshallClient reads a transaction a client sends: a frame of the client (ClientTxType), which
// it turns into the stored form with the time of the node, or a transaction in its stored form.
// Only the node a client sends to reads a frame of the client: in a block, every node would store
// it with a time of its own, and they would disagree on the block.
func (rtx *Transaction) UnmarshallClient(buffer *bytes.Buffer, fill bool) error {
	return rtx.unmarshall(buffer, fill, true)
}

func (rtx *Transaction) unmarshall(buffer *bytes.Buffer, fill, client bool) error {
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
	case types.ClientTxType:
		if !client {
			return fmt.Errorf("a frame of a client is no stored transaction")
		}
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
		if err = itx.Unmarshal(bytes.NewBuffer(newbuf[1:]), fill); err != nil {
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
	inner.setTxSize(int64(len(rtx.FullData)))
	rtx.Inner = inner
	return nil
}
