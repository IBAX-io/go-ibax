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
		var itx SmartContractTransaction
		var sc = &smart.SmartContract{}
		inner = &itx
		if err = converter.BinUnmarshalBuff(buffer, &sc.Payload); err != nil {
			return err
		}

		itx.SmartContract = sc
		itx.TxHash = crypto.DoubleHash(itx.Payload)

		if t, ok := txCache.Get(string(itx.TxHash)); ok {
			rtx = t
			return nil
		}

		itx.TxSignature = buffer.Bytes()

		if err = itx.parseFromContract(true); err != nil {
			return err
		}

	case types.FirstBlockTxType:
		var itx FirstBlockTransaction
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
		var itx = StopNetworkTransaction{}
		inner = &itx

		if err := itx.Unmarshal(buffer); err != nil {
			log.WithFields(log.Fields{"error": err, "type": consts.UnmarshallingError, "tx_type": rtx.TxType()}).Error("getting parser for tx type")
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

//
//func (rtx *Transaction) GetExpedite() decimal.Decimal {
//	expedite, _ := decimal.NewFromString(rtx.expedite)
//	return expedite
//}

func (rtx *Transaction) HashStr() string {
	return fmt.Sprintf("%x", rtx.TxHash())
}

//
//func (rtx *Transaction) Signature() []byte {
//	return rtx.signature
//}

//func (rtx *Transaction) SmartTx() *types.SmartContract {
//	return rtx.smartTx
//}

func (rtx *Transaction) Processing(txData []byte) error {
	if err := rtx.Unmarshall(bytes.NewBuffer(txData)); err != nil {
		return err
	}
	return nil
}

func (rtx *Transaction) SetRawTx() *sqldb.RawTx {
	return &sqldb.RawTx{
		Hash:     rtx.TxHash(),
		Time:     rtx.TxTime(),
		TxType:   rtx.TxType(),
		Data:     rtx.FullData,
		Expedite: rtx.TxExpedite().String(),
		WalletID: rtx.TxKeyID(),
	}
}
