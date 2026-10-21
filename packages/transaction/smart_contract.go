/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"fmt"
	"reflect"
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
	storedSize
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
	s.TxSize = s.txSize()
	s.VM = script.GetVM()
	s.ChildChain = false
	s.Rollback = true
	s.SysUpdate = false
	s.OutputsMap = t.OutputsMap
	s.PrevSysPar = t.PrevSysPar
	s.EcoParams = t.EcoParams
	s.TxInputsMap = make(map[sqldb.KeyUTXO][]sqldb.SpentInfo)
	s.TxOutputsMap = make(map[sqldb.KeyUTXO][]sqldb.SpentInfo)
	s.RollBackTx = make([]*types.RollbackTx, 0)
	if s.GenBlock {
		s.TimeLimit = syspar.GetMaxBlockGenerationTime()
		if syspar.IsBftConsensus() && t.TxCheckLimits != nil {
			// the proposer makes its block within max_block_generation_time
			s.TimeLimit = t.TxCheckLimits.TimeLeft()
		}
	}
	s.Key = &sqldb.Key{}
	return nil
}

// checkTransferModel refuses the transfers of the UTXO model, TransferSelf and UTXO, on a chain only
// the honor nodes generate: their fees stay out of 1_history, which the clearing of the alliance
// reconciles, and they move the platform quota around its transfer rule. honor_nodes_only never
// changes after genesis, so every node refuses them alike.
func (s *SmartTransactionParser) checkTransferModel() error {
	if s.TxSmart.TxType() != types.SmartContractTxType && syspar.IsHonorNodesOnly() {
		return errors.New("a chain of the honor nodes only takes no TransferSelf nor UTXO transaction")
	}
	return nil
}

func (s *SmartTransactionParser) Validate() error {
	if err := s.TxSmart.Validate(); err != nil {
		return err
	}
	if err := s.checkTransferModel(); err != nil {
		return err
	}
	// The header carries the key that signs; whether the account may sign with it is checked
	// when the transaction runs. Validate runs only when a transaction enters the queue, so a key
	// of an algorithm the network takes no longer is refused here with the clock: the block time
	// decides once it runs.
	key, err := crypto.ParseAccountKey(s.TxSmart.PublicKey)
	if err != nil {
		return err
	}
	if err := syspar.GetAccountAlgorithms().CheckSign(key.Algo, time.Now().Unix()); err != nil {
		return err
	}
	signature, err := utils.TxSignature(s.TxSignature)
	if err != nil {
		return err
	}
	if ok, err := key.Verify(s.Hash, signature); err != nil || !ok {
		return fmt.Errorf("transaction signature does not verify with the key in the header: %v", err)
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
	}()

	// Every transaction proves the key that signs it before it runs, whatever its type. The
	// transfer transactions move funds outside a contract, so this is the only place they are
	// checked; contract transactions are checked here too, before their body runs.
	if err = s.CheckTxSign(); err != nil {
		return err
	}
	// A block another node made is checked as the queue checks a transaction
	if err = s.checkTransferModel(); err != nil {
		return err
	}

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
	if err == smart.ErrTimeLeft {
		return ErrTimeLeft
	}
	if err == nil && s.TxSmart != nil {
		err = in.TxCheckLimits.CheckLimit(s)
	}
	if err != nil {
		return
	}
	return
}

func (s *SmartTransactionParser) TxRollback() error {
	return syspar.SysUpdate(s.DbTransaction)
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
	signature, err := crypto.NodeSign(privateKey, s.Hash)
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
	// A stored transaction carries the payload, the hash of that payload and the transaction the
	// node runs as fields of their own, and nothing ties the three together. The signature covers
	// the hash, so the payload is the part taken as given: a body its payload does not encode would
	// otherwise run under a signature given for another transaction, and anyone who saw a signed
	// transaction could put a body of their own under its hash and its signature. The client format
	// derives the hash and the body from the payload; the stored format is held to the same relation
	// here, before the queue and before a received block runs anything.
	if len(s.Payload) == 0 || !bytes.Equal(s.Hash, crypto.DoubleHash(s.Payload)) {
		return fmt.Errorf("the hash of a transaction is not the hash of its payload")
	}
	signed := new(types.SmartTransaction)
	if err := msgpack.Unmarshal(s.Payload, &signed); err != nil {
		return err
	}
	if !reflect.DeepEqual(s.TxSmart, signed) {
		return fmt.Errorf("a transaction carries a body its payload does not encode")
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
