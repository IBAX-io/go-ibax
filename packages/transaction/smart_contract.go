/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/converter"

	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
)

type SmartTransactionParser struct {
	*smart.SmartContract
}

func (s *SmartTransactionParser) txType() byte      { return s.TxSmart.TxType() }
func (s *SmartTransactionParser) txHash() []byte    { return s.Hash }
func (s *SmartTransactionParser) txPayload() []byte { return s.Payload }
func (s *SmartTransactionParser) txTime() int64     { return s.Timestamp }
func (s *SmartTransactionParser) txKeyID() int64    { return s.TxSmart.KeyID }
func (s *SmartTransactionParser) txExpedite() decimal.Decimal {
	dec, _ := decimal.NewFromString(s.TxSmart.Expedite)
	return dec
}
func (s *SmartTransactionParser) setTimestamp() {
	s.Timestamp = time.Now().Unix()
}

func (s *SmartTransactionParser) Init(t *Transaction) error {
	s.Rand = t.Rand
	s.GenBlock = t.GenBlock
	s.BlockData = t.BlockData
	s.PreBlockData = t.PreBlockData
	s.Notifications = t.Notifications
	s.DbTransaction = t.DbTransaction
	s.TxSize = int64(len(s.Payload))
	s.VM = script.GetVM()
	s.CLB = false
	s.Rollback = true
	s.SysUpdate = false
	s.RollBackTx = make([]*sqldb.RollbackTx, 0)
	if s.GenBlock {
		s.TimeLimit = syspar.GetMaxBlockGenerationTime()
	}
	return nil
}

func (s *SmartTransactionParser) Validate() error {
	txSmart := s.TxSmart
	if len(txSmart.Expedite) > 0 {
		expedite, _ := decimal.NewFromString(txSmart.Expedite)
		if expedite.LessThan(decimal.Zero) {
			return fmt.Errorf("expedite fee %s must be greater than 0", expedite)
		}
	}
	if len(strings.TrimSpace(txSmart.Lang)) > 2 {
		return fmt.Errorf(`localization size is greater than 2`)
	}

	var publicKeys [][]byte
	publicKeys = append(publicKeys, crypto.CutPub(s.TxSmart.PublicKey))
	_, err := utils.CheckSign(publicKeys, s.Hash, s.TxSignature, false)
	if err != nil {
		return err
	}

	return nil
}

func (s *SmartTransactionParser) Action(t *Transaction) (err error) {
	t.TxResult, err = s.CallContract(t.SqlDbSavePoint)
	t.RollBackTx = s.RollBackTx
	t.SysUpdate = s.SysUpdate
	if err == nil && s.TxSmart != nil {
		if s.Penalty {
			if s.FlushRollback != nil {
				flush := s.FlushRollback
				for i := len(flush) - 1; i >= 0; i-- {
					flush[i].FlushVM()
				}
			}
		}
		err = t.TxCheckLimits.CheckLimit(t)
	}
	if err != nil {
		if s.FlushRollback != nil {
			flush := s.FlushRollback
			for i := len(flush) - 1; i >= 0; i-- {
				flush[i].FlushVM()
			}
		}
	}

	return
}

func (s *SmartTransactionParser) TxRollback() error {
	return nil
}

func (s *SmartTransactionParser) BinMarshal(smartTx *types.SmartTransaction, privateKey []byte, internal bool) ([]byte, error) {
	var (
		publicKey, signature []byte
		err                  error
	)
	if publicKey, err = crypto.PrivateToPublic(privateKey); err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("converting node private key to public")
	}
	smartTx.PublicKey = publicKey
	if internal {
		smartTx.SignedBy = crypto.Address(publicKey)
	}
	s.setTimestamp()
	s.TxSmart = smartTx
	var buf []byte
	buf, err = msgpack.Marshal(smartTx)
	if err != nil {
		return nil, err
	}
	s.Payload = buf
	s.Hash = crypto.DoubleHash(s.Payload)
	signature, err = crypto.Sign(privateKey, s.Hash)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("signing by node private key")
		return nil, err
	}
	s.TxSignature = converter.EncodeLengthPlusData(signature)
	buf, err = msgpack.Marshal(s)
	if err != nil {
		return nil, err
	}

	buf = append([]byte{s.txType()}, buf...)
	return buf, nil
}

func (s *SmartTransactionParser) Unmarshal(buffer *bytes.Buffer) error {
	buffer.UnreadByte()
	if err := msgpack.Unmarshal(buffer.Bytes()[1:], s); err != nil {
		return err
	}
	return nil
}

func (s *SmartTransactionParser) parseFromContract(fillData bool) error {
	var err error
	if err := msgpack.Unmarshal(s.Payload, &s.TxSmart); err != nil {
		log.WithFields(log.Fields{"error": err}).Error("on unmarshalling to sc")
		return err
	}
	err = s.Validate()
	if err != nil {
		return err
	}
	smartTx := s.TxSmart
	contract := smart.GetContractByID(int32(smartTx.ID))
	if contract == nil {
		log.WithFields(log.Fields{"contract_id": smartTx.ID, "type": consts.NotFound}).Error("unknown contract")
		return fmt.Errorf(`unknown contract %d`, smartTx.ID)
	}

	s.TxContract = contract
	s.TxData = make(map[string]interface{})
	txInfo := contract.Info().Tx

	if txInfo != nil {
		if fillData {
			if s.TxData, err = smart.FillTxData(*txInfo, smartTx.Params); err != nil {
				return errors.Wrap(err, fmt.Sprintf("contract '%s'", contract.Name))
			}
		} else {
			s.TxData = smartTx.Params
			for key, item := range s.TxData {
				if v, ok := item.(map[interface{}]interface{}); ok {
					imap := make(map[string]interface{})
					for ikey, ival := range v {
						imap[fmt.Sprint(ikey)] = ival
					}
					s.TxData[key] = imap
				}
			}
		}
	}

	return nil
}
