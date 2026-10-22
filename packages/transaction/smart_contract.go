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
	"github.com/IBAX-io/go-ibax/packages/common/gencache"
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

// errNoRegisteredKey refuses a transaction without a key in the header from an account that has
// registered none: the first transaction of an account carries its key
var errNoRegisteredKey = errors.New("the account has registered no key: send the key in the header")

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
	signature, err := utils.TxSignature(s.TxSignature)
	if err != nil {
		return err
	}
	// The key that signs is the one in the header, or the key the account registered when the
	// header has none. Whether the account may sign with it is checked when the transaction runs.
	// Validate runs only when a transaction enters the queue, so a key of an algorithm the network
	// takes no longer is refused here with the clock: the block time decides once it runs.
	if len(s.TxSmart.PublicKey) > 0 {
		key, err := crypto.ParseAccountKey(s.TxSmart.PublicKey)
		if err != nil {
			return err
		}
		if err := syspar.GetAccountAlgorithms().CheckSign(key.Algo, time.Now().Unix()); err != nil {
			return err
		}
		if ok, err := key.Verify(s.Hash, signature); err != nil || !ok {
			return fmt.Errorf("transaction signature does not verify with the key in the header: %v", err)
		}
		return nil
	}
	// A node signs with its key in the header: only an account signs with the key it registered
	if s.TxSmart.SignedBy != 0 {
		return errors.New("a transaction signed by another key carries that key in the header")
	}
	eco, keyID := s.TxSmart.EcosystemID, s.TxSmart.KeyID
	for _, fresh := range []bool{false, true} {
		pub, cached, err := registeredKey(eco, keyID, fresh)
		if err != nil {
			return err
		}
		key, err := crypto.ParseAccountKey(pub)
		if err != nil {
			return err
		}
		if err := syspar.GetAccountAlgorithms().CheckSign(key.Algo, time.Now().Unix()); err != nil {
			return err
		}
		if ok, _ := key.Verify(s.Hash, signature); ok {
			return nil
		}
		if !cached {
			break
		}
	}
	return fmt.Errorf("transaction signature does not verify with the key of account %s", converter.AddressToString(keyID))
}

// registeredKeys are the keys accounts registered in 1_keys, by ecosystem and key id: what the
// transactions without a key in the header are checked with on their way in. A key of the cache
// that does not verify is read again, for the account may have changed it.
var registeredKeys = gencache.New[registeredKeyID, []byte](1 << 16)

type registeredKeyID struct{ ecosystem, keyID int64 }

// registeredKey is the key the account registered in the ecosystem, from the cache unless fresh,
// and whether it came from the cache. An account without one sends its key in the header.
func registeredKey(ecosystem, keyID int64, fresh bool) ([]byte, bool, error) {
	id := registeredKeyID{ecosystem, keyID}
	if !fresh {
		if pub, ok := registeredKeys.Get(id); ok {
			return pub, true, nil
		}
	}
	key := &sqldb.Key{}
	found, err := key.SetTablePrefix(ecosystem).Get(nil, keyID)
	if err != nil {
		return nil, false, err
	}
	if !found || len(key.PublicKey) == 0 {
		return nil, false, fmt.Errorf("%w: account %s in ecosystem %d", errNoRegisteredKey, converter.AddressToString(keyID), ecosystem)
	}
	registeredKeys.Put(id, key.PublicKey)
	return key.PublicKey, false, nil
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

// storedTx is the form a block holds a transaction in and nodes pass on: the payload the client
// signed, the signature and the time the node took the transaction. The hash and the body the node
// runs are derived from the payload, so a stored transaction runs only what its signature covers.
// errNotStoredForm is a transaction of a stored type in an encoding other than its stored form
var errNotStoredForm = errors.New("the transaction is not in its stored form")

type storedTx struct {
	_msgpack    struct{} `msgpack:",as_array"`
	Payload     []byte
	TxSignature []byte
	Timestamp   int64
}

func (s *SmartTransactionParser) Marshal() ([]byte, error) {
	s.setTimestamp()
	if err := s.Validate(); err != nil {
		return nil, err
	}
	buf, err := msgpack.Marshal(&storedTx{Payload: s.Payload, TxSignature: s.TxSignature, Timestamp: s.Timestamp})
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
	var stored storedTx
	if err := msgpack.Unmarshal(buffer.Bytes(), &stored); err != nil {
		return err
	}
	// a transaction has one stored form: any other encoding of the same three fields, or one
	// carrying more, is refused rather than read as the transaction it names
	canonical, err := msgpack.Marshal(&stored)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical, buffer.Bytes()) {
		return errNotStoredForm
	}
	if len(stored.Payload) == 0 {
		return fmt.Errorf("a stored transaction carries no payload")
	}
	s.Payload, s.TxSignature, s.Timestamp = stored.Payload, stored.TxSignature, stored.Timestamp
	s.Hash = crypto.DoubleHash(s.Payload)
	s.TxSmart = new(types.SmartTransaction)
	if err := msgpack.Unmarshal(s.Payload, s.TxSmart); err != nil {
		return err
	}
	if s.TxSmart.Header == nil {
		return fmt.Errorf("a transaction carries no header")
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
