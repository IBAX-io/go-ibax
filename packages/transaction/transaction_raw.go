/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
)

func (rtx *Transaction) Unmarshall(buffer *bytes.Buffer) error {
	if buffer.Len() == 0 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("empty transaction buffer")
		return fmt.Errorf("empty transaction buffer")
	}
	rtx.FullData = buffer.Bytes()

	txT, err := buffer.ReadByte()
	if err != nil {
		return err
	}

	var inner TransactionCaller
	switch txT {
	case types.SmartContractTxType, byte(128):
		itx := new(SmartTransactionParser)
		inner = itx
		if err = itx.Unmarshal(buffer); err != nil {
			//if err = converter.BinUnmarshalBuff(buffer, &sc.Payload); err != nil {
			return err
		}
		if t, ok := txCache.Get(string(itx.Hash)); ok {
			rtx = t
			return nil
		}

		if err = itx.parseFromContract(true); err != nil {
			return err
		}

	case types.FirstBlockTxType:
		var itx FirstBlockParser
		inner = &itx
		if err := itx.Unmarshal(buffer); err != nil {
			log.WithFields(log.Fields{"error": err, "type": consts.UnmarshallingError, "tx_type": itx.txType()}).Error("getting parser for tx type")
			return err
		}

		if t, ok := txCache.Get(string(itx.TxHash)); ok {
			rtx = t
			return nil
		}
		err = itx.Validate()
		if err != nil {
			return err
		}
	case types.StopNetworkTxType:
		var itx = StopNetworkParser{}
		inner = &itx

		if err := itx.Unmarshal(buffer); err != nil {
			log.WithFields(log.Fields{"error": err, "type": consts.UnmarshallingError, "tx_type": rtx.Type()}).Error("getting parser for tx type")
			return err
		}
		if t, ok := txCache.Get(fmt.Sprintf("%x", itx.TxHash)); ok {
			rtx = t
			return nil
		}
		err = itx.Validate()
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported tx type %d", txT)
	}

	rtx.Inner = inner
	txCache.Set(rtx)

	return nil
}

func (rtx *Transaction) Processing(txData []byte) error {
	if err := rtx.Unmarshall(bytes.NewBuffer(txData)); err != nil {
		return err
	}
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
