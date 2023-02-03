/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
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
	s.Timestamp = time.Now().UnixMilli()
}

func (s *SmartTransactionParser) Init(t *InToCxt) error {
	s.Rand = t.Rand
	s.GenBlock = t.GenBlock
	s.BlockHeader = t.BlockHeader
	s.PreBlockHeader = t.PreBlockHeader
	s.Notifications = t.Notifications
	s.DbTransaction = t.DbTransaction
	s.TxSize = int64(len(s.Payload))
	s.VM = script.GetVM()
	s.CLB = false
	s.Rollback = true
	s.SysUpdate = false
	s.OutputsMap = t.OutputsMap
	s.PrevSysPar = t.PrevSysPar
	s.ComPercents = t.ComPercents
	s.TxInputsMap = make(map[sqldb.KeyUTXO][]sqldb.SpentInfo)
	s.TxOutputsMap = make(map[sqldb.KeyUTXO][]sqldb.SpentInfo)
	s.RollBackTx = make([]*types.RollbackTx, 0)
	if s.GenBlock {
		s.TimeLimit = syspar.GetMaxBlockGenerationTime()
	}
	s.Key = &sqldb.Key{}
	return nil
}

func (s *SmartTransactionParser) Validate() error {
	if err := s.TxSmart.Validate(); err != nil {
		return err
	}
	_, err := utils.CheckSign([][]byte{crypto.CutPub(s.TxSmart.PublicKey)}, s.Hash, s.TxSignature, false)
	if err != nil {
		return err
	}
	return nil
}

func (s *SmartTransactionParser) Action(in *InToCxt, out *OutCtx) (err error) {
	var res string
	defer func() {
		if len(res) > 255 {
			res = res[:252] + "..."
		}
		ret := &pbgo.TxResult{
			Result: res,
			Hash:   out.TxResult.Hash,
		}
		if s.Penalty {
			ret.Code = pbgo.TxInvokeStatusCode_PENALTY
			ret.BlockId = s.BlockHeader.BlockId
		}
		out.Apply(
			WithOutCtxTxResult(ret),
			WithOutCtxSysUpdate(s.SysUpdate),
			WithOutCtxRollBackTx(s.RollBackTx),
		)
		if err != nil || s.Penalty {
			if s.FlushRollback != nil {
				flush := s.FlushRollback
				for i := len(flush) - 1; i >= 0; i-- {
					flush[i].FlushVM()
				}
			}
			return
		}
		ret.Code = pbgo.TxInvokeStatusCode_SUCCESS
		ret.BlockId = s.BlockHeader.BlockId
		out.Apply(
			WithOutCtxTxResult(ret),
			WithOutCtxTxOutputs(s.TxOutputsMap),
			WithOutCtxTxInputs(s.TxInputsMap),
		)
		//in.DbTransaction.BinLogSql = s.DbTransaction.BinLogSql
	}()

	_transferSelf := s.TxSmart.TransferSelf
	if _transferSelf != nil {
		_, err = smart.TransferSelf(s.SmartContract, _transferSelf.Value, _transferSelf.Source, _transferSelf.Target)
		if err != nil {
			return err
		}
		err = in.TxCheckLimits.CheckLimit(s)
		if err != nil {
			return
		}
		return
	}
	_utxo := s.TxSmart.UTXO
	if _utxo != nil {
		_, err = smart.UtxoToken(s.SmartContract, _utxo.ToID, _utxo.Value)
		if err != nil {
			return err
		}
		err = in.TxCheckLimits.CheckLimit(s)
		if err != nil {
			return
		}
		return
	}
	res, err = s.CallContract(in.SqlDbSavePoint)
	if err == nil && s.TxSmart != nil {
		err = in.TxCheckLimits.CheckLimit(s)
	}
	if err != nil {
		return
	}
	return
}

func (s *SmartTransactionParser) TxRollback() error {
	return nil
}

func (s *SmartTransactionParser) Marshal() ([]byte, error) {
	s.setTimestamp()
	if err := s.Validate(); err != nil {
		return nil, err
	}
	buf, err := msgpack.Marshal(s)
	if err != nil {
		return nil, err
	}
	buf = append([]byte{s.txType()}, buf...)
	return buf, nil
}

func (s *SmartTransactionParser) setSig(privateKey []byte) error {
	signature, err := crypto.Sign(privateKey, s.Hash)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("signing by node private key")
		return err
	}
	s.TxSignature = converter.EncodeLengthPlusData(signature)
	return nil
}

func (s *SmartTransactionParser) BinMarshalWithPrivate(smartTx *types.SmartTransaction, privateKey []byte, internal bool) ([]byte, error) {
	var (
		buf []byte
		err error
	)
	if err = smartTx.WithPrivate(privateKey, internal); err != nil {
		return nil, err
	}
	s.TxSmart = smartTx
	buf, err = s.TxSmart.Marshal()
	if err != nil {
		return nil, err
	}
	s.Payload = buf
	s.Hash = crypto.DoubleHash(s.Payload)
	err = s.setSig(privateKey)
	if err != nil {
		return nil, err
	}
	return s.Marshal()
}

func (s *SmartTransactionParser) Unmarshal(buffer *bytes.Buffer, fill bool) error {
	buffer.UnreadByte()
	if err := msgpack.Unmarshal(buffer.Bytes()[1:], s); err != nil {
		return err
	}
	if s.SmartContract.TxSmart.UTXO != nil || s.SmartContract.TxSmart.TransferSelf != nil {
		return nil
	}
	if err := s.parseFromContract(fill); err != nil {
		return err
	}
	return nil
}

func (s *SmartTransactionParser) parseFromContract(fillData bool) error {
	var err error
	smartTx := s.TxSmart
	contract := smart.GetContractByID(int32(smartTx.ID))
	if contract == nil {
		log.WithFields(log.Fields{"contract_id": smartTx.ID, "type": consts.NotFound}).Error("unknown contract")
		return fmt.Errorf(`unknown contract %d`, smartTx.ID)
	}

	s.TxContract = contract
	s.TxData = make(map[string]any)
	txInfo := contract.Info().Tx

	if txInfo != nil {
		if fillData {
			for k := range smartTx.Params {
				if _, ok := contract.Info().TxMap()[k]; !ok {
					return fmt.Errorf("'%s' parameter is not required", k)
				}
			}
			if s.TxData, err = smart.FillTxData(*txInfo, smartTx.Params); err != nil {
				return errors.Wrap(err, fmt.Sprintf("contract '%s'", contract.Name))
			}
		} else {
			s.TxData = smartTx.Params
			for key, item := range s.TxData {
				if v, ok := item.(map[any]any); ok {
					imap := make(map[string]any)
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

func (s *SmartTransactionParser) SysUpdateWorker(dbTx *sqldb.DbTransaction) error {
	if !s.SysUpdate {
		return nil
	}
	if err := syspar.SysUpdate(dbTx); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating syspar")
		return err
	}
	return nil
}

func (s *SmartTransactionParser) SysTableColByteaWorker(dbTx *sqldb.DbTransaction) error {
	if err := syspar.SysTableColType(dbTx); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating syspar")
		return err
	}
	return nil
}

func (s *SmartTransactionParser) FlushVM() {
	if s.FlushRollback == nil {
		return
	}
	flush := s.FlushRollback
	for i := len(flush) - 1; i >= 0; i-- {
		flush[i].FlushVM()
	}
	return
}

type TxOutCtx struct {
	SysUpdate        bool
	SysTableColBytea bool
	Flush            bool
	FlushRollback    []*smart.FlushInfo
	VM               *script.VM
	VM2              *script.VM
}
