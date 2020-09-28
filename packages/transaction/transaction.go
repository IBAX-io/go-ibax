/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"

	"github.com/pkg/errors"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"

	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/transaction/custom"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
	"github.com/IBAX-io/go-ibax/packages/utils/tx"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/IBAX-io/go-ibax/packages/crypto"
)

type RawTransaction struct {
	txType, time int64
	hash         []byte
	data         []byte
	payload      []byte
	signature    []byte
	expedite     string
	smartTx      tx.SmartContract
}

func (rtx *RawTransaction) Unmarshall(buffer *bytes.Buffer) error {
	if buffer.Len() == 0 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("empty transaction buffer")
		return fmt.Errorf("empty transaction buffer")
	}

	rtx.data = buffer.Bytes()

	b, err := buffer.ReadByte()
	if err != nil {
		return err
	}
	rtx.txType = int64(b)

	if IsContractTransaction(rtx.txType) {
		if err = converter.BinUnmarshalBuff(buffer, &rtx.payload); err != nil {
			return err
		}
		rtx.signature = buffer.Bytes()

		if err = msgpack.Unmarshal(rtx.Payload(), &rtx.smartTx); err != nil {
			log.WithFields(log.Fields{"error": err}).Error("on unmarshalling to sc")
			return err
		}
		rtx.expedite = rtx.smartTx.Expedite
		rtx.time = rtx.smartTx.Time
	} else {
		buffer.UnreadByte()
		rtx.payload = buffer.Bytes()
	}

	rtx.hash = crypto.DoubleHash(rtx.payload)

	return nil
}

func (rtx *RawTransaction) Type() int64 {
	return rtx.txType
}

func (rtx *RawTransaction) Hash() []byte {
	return rtx.hash
}

func (rtx *RawTransaction) HashStr() string {
	return fmt.Sprintf("%x", rtx.hash)
}

func (rtx *RawTransaction) Bytes() []byte {
	return rtx.data
}

func (rtx *RawTransaction) Payload() []byte {
	return rtx.payload
}

func (rtx *RawTransaction) Signature() []byte {
	return rtx.signature
}

func (rtx *RawTransaction) Expedite() decimal.Decimal {
	expedite, _ := decimal.NewFromString(rtx.expedite)
	return expedite
}

func (rtx *RawTransaction) Time() int64 {
	return rtx.time
}

func (rtx *RawTransaction) SmartTx() tx.SmartContract {
	return rtx.smartTx
}

func (rtx *RawTransaction) Processing(txData []byte) error {
	if err := rtx.Unmarshall(bytes.NewBuffer(txData)); err != nil {
		return err
	}
	if len(rtx.SmartTx().Expedite) > 0 {
		if rtx.Expedite().LessThan(decimal.New(0, 0)) {
			return fmt.Errorf("expedite fee %s must be greater than 0", rtx.SmartTx().Expedite)
		}
	}
	if len(strings.TrimSpace(rtx.SmartTx().Lang)) > 2 {
		return fmt.Errorf(`localization size is greater than 2`)
	}
	var PublicKeys [][]byte
	PublicKeys = append(PublicKeys, crypto.CutPub(rtx.SmartTx().PublicKey))
	_, err := utils.CheckSign(PublicKeys, rtx.Hash(), rtx.Signature(), false)
	if err != nil {
		return err
	}
	return nil
}
func (rtx *RawTransaction) SetRawTx() *model.RawTx {
	return &model.RawTx{
		Hash:     rtx.Hash(),
		Time:     rtx.Time(),
		TxType:   rtx.Type(),
		Data:     rtx.Bytes(),
		Expedite: rtx.Expedite().String(),
		WalletID: rtx.SmartTx().Header.KeyID,
	}
}

	TxFullData    []byte // full transaction, with type and data
	TxHash        []byte
	TxSignature   []byte
	TxKeyID       int64
	TxTime        int64
	TxType        int64
	TxCost        int64 // Maximum cost of executing contract
	TxFuel        int64
	TxUsedCost    decimal.Decimal // Used cost of CPU resources
	TxPtr         interface{}     // Pointer to the corresponding struct in consts/struct.go
	TxData        map[string]interface{}
	TxSmart       *tx.SmartContract
	TxContract    *smart.Contract
	TxHeader      *tx.Header
	tx            custom.TransactionInterface
	DbTransaction *model.DbTransaction
	SysUpdate     bool
	Rand          *rand.Rand
	Notifications types.Notifications
	GenBlock      bool
	TimeLimit     int64

	SmartContract *smart.SmartContract
	RollBackTx    []*model.RollbackTx
}

// GetLogger returns logger
func (t Transaction) GetLogger() *log.Entry {
	logger := log.WithFields(log.Fields{"tx_type": t.TxType, "tx_time": t.TxTime, "tx_wallet_id": t.TxKeyID})
	if t.BlockData != nil {
		logger = logger.WithFields(log.Fields{"block_id": t.BlockData.BlockID, "block_time": t.BlockData.Time, "block_wallet_id": t.BlockData.KeyID, "block_state_id": t.BlockData.EcosystemID, "block_hash": t.BlockData.Hash, "block_version": t.BlockData.Version})
	}
	if t.PrevBlock != nil {
		logger = logger.WithFields(log.Fields{"block_id": t.BlockData.BlockID, "block_time": t.BlockData.Time, "block_wallet_id": t.BlockData.KeyID, "block_state_id": t.BlockData.EcosystemID, "block_hash": t.BlockData.Hash, "block_version": t.BlockData.Version})
	}
	return logger
}

var txCache = &transactionCache{cache: make(map[string]*Transaction)}

// UnmarshallTransaction is unmarshalling transaction
func UnmarshallTransaction(buffer *bytes.Buffer, fillData bool) (*Transaction, error) {
	rtx := &RawTransaction{}
	if err := rtx.Unmarshall(buffer); err != nil {
		return nil, err
	}

	if t, ok := txCache.Get(string(rtx.Hash())); ok {
		return t, nil
	}

	t := new(Transaction)
	t.TxFullData = rtx.Bytes()
	t.TxType = rtx.Type()
	t.TxHash = rtx.Hash()
	t.TxBinaryData = rtx.Payload()
	t.TxUsedCost = decimal.New(0, 0)

	// smart contract transaction
	if IsContractTransaction(rtx.Type()) {
		t.TxSignature = rtx.Signature()
		// skip byte with transaction type
		if err := t.parseFromContract(fillData); err != nil {
			return nil, err
		}
		// struct transaction (only first block transaction for now)
	} else if consts.IsStruct(rtx.Type()) {
		if err := t.parseFromStruct(); err != nil {
			return t, err
		}

		// all other transactions
	}
	txCache.Set(t)

	return t, nil
}

// IsContractTransaction checks txType
func IsContractTransaction(txType int64) bool {
	return txType > 127 || txType == consts.TxTypeApiContract || txType == consts.TxTypeEcosystemMiner || txType == consts.TxTypeSystemMiner
}

func (t *Transaction) parseFromStruct() error {
	t.TxPtr = consts.MakeStruct(consts.TxTypes[t.TxType])
	if err := converter.BinUnmarshal(&t.TxBinaryData, t.TxPtr); err != nil {
		log.WithFields(log.Fields{"error": err, "type": consts.UnmarshallingError, "tx_type": t.TxType}).Error("getting parser for tx type")
		return err
	}
	head := consts.Header(t.TxPtr)
	t.TxKeyID = head.KeyID
	t.TxTime = int64(head.Time)

	trParser, err := GetTransaction(t, consts.TxTypes[t.TxType])
	if err != nil {
		return err
	}
	err = trParser.Validate()
	if err != nil {
		return utils.ErrInfo(err)
	}
	t.tx = trParser

	return nil
}

func (t *Transaction) fillTxData(fieldInfos []*script.FieldInfo, params map[string]interface{}) error {
	var err error
	t.TxData, err = smart.FillTxData(fieldInfos, params)
	if err != nil {
		return err
	}
	return nil
}

func (t *Transaction) parseFromContract(fillData bool) error {
	smartTx := tx.SmartContract{}
	if err := msgpack.Unmarshal(t.TxBinaryData, &smartTx); err != nil {
		log.WithFields(log.Fields{"tx_hash": t.TxHash, "error": err, "type": consts.UnmarshallingError}).Error("unmarshalling smart tx msgpack")
		return err
	}
	t.TxPtr = nil
	t.TxSmart = &smartTx
	t.TxTime = smartTx.Time
	t.TxKeyID = smartTx.KeyID

	contract := smart.GetContractByID(int32(smartTx.ID))
	if contract == nil {
		log.WithFields(log.Fields{"contract_id": smartTx.ID, "type": consts.NotFound}).Error("unknown contract")
		return fmt.Errorf(`unknown contract %d`, smartTx.ID)
	}

	t.TxContract = contract
	t.TxHeader = &smartTx.Header

	t.TxData = make(map[string]interface{})
	txInfo := contract.Block.Info.(*script.ContractInfo).Tx

	if txInfo != nil {
		if fillData {
			if err := t.fillTxData(*txInfo, smartTx.Params); err != nil {
				return errors.Wrap(err, fmt.Sprintf("contract '%s'", contract.Name))
			}
		} else {
			t.TxData = smartTx.Params
			for key, item := range t.TxData {
				if v, ok := item.(map[interface{}]interface{}); ok {
					imap := make(map[string]interface{})
					for ikey, ival := range v {
						imap[fmt.Sprint(ikey)] = ival
					}
					t.TxData[key] = imap
				}
			}
		}
	}

	return nil
}

func (t *Transaction) Check(checkTime int64, checkForDupTr bool) error {
	err := CheckLogTx(t.TxHash, checkForDupTr, false)
	if err != nil {
		return err
	}
	logger := log.WithFields(log.Fields{"tx_time": t.TxTime})
	// time in the transaction cannot be more than MAX_TX_FORW seconds of block time
	if t.TxTime > checkTime {
		if t.TxTime-consts.MAX_TX_FORW > checkTime {
			logger.WithFields(log.Fields{"tx_max_forw": consts.MAX_TX_FORW, "tx_time": t.TxTime, "check_time": checkTime, "type": consts.ParameterExceeded}).Error("time in the tx cannot be more than MAX_TX_FORW seconds of block time ")
			return ErrNotComeTime
		}
		logger.WithFields(log.Fields{"tx_time": t.TxTime, "check_time": checkTime, "type": consts.ParameterExceeded}).Error("time in the tx cannot be more than of block time ")
		return ErrEarlyTime
	}

	if t.TxType != consts.TxTypeStopNetwork {
		// time in transaction cannot be less than -24 of block time
		if t.TxTime < checkTime-consts.MAX_TX_BACK {
			logger.WithFields(log.Fields{"tx_max_back": consts.MAX_TX_BACK, "check_time": checkTime, "type": consts.ParameterExceeded, "tx_time": t.TxTime, "tx_type": t.TxType}).Error("time in the tx cannot be less then -24 hour of block time")
			return ErrExpiredTime
		}
	}

	if t.TxContract == nil {
		if t.BlockData != nil && t.BlockData.BlockID != 1 {
			if t.TxKeyID == 0 {
				logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("Empty user id")
				return ErrEmptyKey
			}
		}
	}

	return nil
}

func (tx *Transaction) CheckTime(checkTime int64) error {
	if tx.TxKeyID == 0 {
		return ErrEmptyKey
	}

	logger := log.WithFields(log.Fields{"tx_hash": hex.EncodeToString(tx.TxHash), "tx_time": tx.TxTime, "check_time": checkTime, "type": consts.ParameterExceeded})
	if tx.TxTime > checkTime {
		if tx.TxTime-consts.MAX_TX_FORW > checkTime {
			logger.WithFields(log.Fields{"tx_max_forw": consts.MAX_TX_FORW}).Errorf("time in the tx cannot be more than %d seconds of block time ", consts.MAX_TX_FORW)
			return ErrNotComeTime
		}
		logger.Error("time in the tx cannot be more than of block time ")
		return ErrEarlyTime
	}

	if tx.TxType != consts.TxTypeStopNetwork {
		if tx.TxTime < checkTime-consts.MAX_TX_BACK {
			logger.WithFields(log.Fields{"tx_max_back": consts.MAX_TX_BACK, "tx_type": tx.TxType}).Errorf("time in the tx cannot be less then %d seconds of block time", consts.MAX_TX_BACK)
			return ErrExpiredTime
		}
	}
	return nil
}

func (t *Transaction) Play(point int) (string, []smart.FlushInfo, error) {
	// smart-contract
	if t.TxContract != nil {
		// check that there are enough money in CallContract
		return t.CallContract(point)
	}

	if t.tx == nil {
		return "", nil, utils.ErrInfo(fmt.Errorf("can't find parser for %d", t.TxType))
	}

	return "", nil, t.tx.Action()
}

// CallContract calls the contract functions according to the specified flags
func (t *Transaction) CallContract(point int) (resultContract string, flushRollback []smart.FlushInfo, err error) {
	sc := smart.SmartContract{
		OBS:           false,
		Rollback:      true,
		SysUpdate:     false,
		VM:            smart.GetVM(),
		TxSmart:       *t.TxSmart,
		TxData:        t.TxData,
		TxContract:    t.TxContract,
		TxCost:        t.TxCost,
		TxUsedCost:    t.TxUsedCost,
		BlockData:     t.BlockData,
		PreBlockData:  t.PrevBlock,
		TxHash:        t.TxHash,
		TxSignature:   t.TxSignature,
		TxSize:        int64(len(t.TxBinaryData)),
		PublicKeys:    t.PublicKeys,
		DbTransaction: t.DbTransaction,
		Rand:          t.Rand,
		GenBlock:      t.GenBlock,
		TimeLimit:     t.TimeLimit,
		Notifications: t.Notifications,
		RollBackTx:    make([]*model.RollbackTx, 0),
	}
	resultContract, err = sc.CallContract(point)
	t.RollBackTx = sc.RollBackTx
	t.TxFuel = sc.TxFuel
	t.SysUpdate = sc.SysUpdate
	if sc.FlushRollback != nil {
		flushRollback = make([]smart.FlushInfo, len(sc.FlushRollback))
		copy(flushRollback, sc.FlushRollback)
	}
	return
}

func (t *Transaction) CallOBSContract() (resultContract string, flushRollback []smart.FlushInfo, err error) {
	sc := smart.SmartContract{
		OBS:           true,
		Rollback:      false,
		SysUpdate:     false,
		VM:            smart.GetVM(),
		TxSmart:       *t.TxSmart,
		TxData:        t.TxData,
		TxContract:    t.TxContract,
		TxCost:        t.TxCost,
		TxUsedCost:    t.TxUsedCost,
		BlockData:     t.BlockData,
		TxHash:        t.TxHash,
		TxSignature:   t.TxSignature,
		TxSize:        int64(len(t.TxBinaryData)),
		PublicKeys:    t.PublicKeys,
		DbTransaction: t.DbTransaction,
		Rand:          t.Rand,
		GenBlock:      t.GenBlock,
		TimeLimit:     t.TimeLimit,
	}
	resultContract, err = sc.CallContract(0)
	t.SysUpdate = sc.SysUpdate
	t.Notifications = sc.Notifications
	return
}

// CleanCache cleans cache of transaction parsers
func CleanCache() {
	txCache.Clean()
}

// GetTxTypeAndUserID returns tx type, wallet and citizen id from the block data
func GetTxTypeAndUserID(binaryBlock []byte) (txType int64, keyID int64) {
	tmp := binaryBlock[:]
	txType = converter.BinToDecBytesShift(&binaryBlock, 1)
	if consts.IsStruct(txType) {
		var txHead consts.TxHeader
		converter.BinUnmarshal(&tmp, &txHead)
		keyID = txHead.KeyID
	}
	return
}

func GetTransaction(t *Transaction, txType string) (custom.TransactionInterface, error) {
	switch txType {
	case consts.TxTypeParserFirstBlock:
		return &custom.FirstBlockTransaction{Logger: t.GetLogger(), DbTransaction: t.DbTransaction, Data: t.TxPtr}, nil
	case consts.TxTypeParserStopNetwork:
		return &custom.StopNetworkTransaction{Logger: t.GetLogger(), Data: t.TxPtr}, nil
	}
	log.WithFields(log.Fields{"tx_type": txType, "type": consts.UnknownObject}).Error("unknown txType")
	return nil, fmt.Errorf("Unknown txType: %s", txType)
}
