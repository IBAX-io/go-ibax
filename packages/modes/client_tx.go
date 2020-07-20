/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package modes

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/crypto"
	"github.com/IBAX-io/go-ibax/packages/utils"

	"github.com/shopspring/decimal"


var ErrDiffKey = errors.New("Different keys")

type blockchainTxPreprocessor struct{}

func (p blockchainTxPreprocessor) ProcessClientTranstaction(txData []byte, key int64, le *log.Entry) (string, error) {
	rtx := &transaction.RawTransaction{}
	if err := rtx.Unmarshall(bytes.NewBuffer(txData)); err != nil {
		le.WithFields(log.Fields{"error": err}).Error("on unmarshalling to raw tx")
		return "", err
	}

	if len(rtx.SmartTx().Expedite) > 0 {
		if rtx.Expedite().LessThan(decimal.New(0, 0)) {
			return "", fmt.Errorf("expedite fee %s must be greater than 0", rtx.SmartTx().Expedite)
		}
	}

	if len(strings.TrimSpace(rtx.SmartTx().Lang)) > 2 {
		return "", fmt.Errorf(`localization size is greater than 2`)
	}

	var PublicKeys [][]byte
	PublicKeys = append(PublicKeys, crypto.CutPub(rtx.SmartTx().PublicKey))
	f, err := utils.CheckSign(PublicKeys, rtx.Hash(), rtx.Signature(), false)
	if err != nil {
		return "", err
	}
	if !f {
		return "", errors.New("sign err")
	}

	//check keyid is exist user
	if key == 0 {
		//ok, err := model.MemberHasRole(nil, 7, 1, converter.AddressToString(rtx.SmartTx().KeyID))
		ok, err := model.MemberHasRolebyName(nil, 1, "Miner", converter.AddressToString(rtx.SmartTx().KeyID))
		if err != nil {
			return "", err
		}

		if ok {

		} else {
			var mo model.MineOwner
			fo, erro := mo.GetPoolManage(rtx.SmartTx().KeyID)
			if erro != nil {
				return "", erro
			}

			if !fo {
				return "", errors.New("mineowner keyid not found")
			}
		}

		if err := model.SendTx(rtx, rtx.SmartTx().KeyID); err != nil {
			le.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("sending tx")
			return "", err
		}

		return string(converter.BinToHex(rtx.Hash())), nil
	}

	if err := model.SendTx(rtx, key); err != nil {
		le.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("sending tx")
		return "", err
	}

	return rtx.HashStr(), nil
}

func (p blockchainTxPreprocessor) ProcessClientTxBatches(txDatas [][]byte, key int64, le *log.Entry) (retTx []string, err error) {
	var rtxs []*model.RawTx
	for _, txData := range txDatas {
		rtx := &transaction.RawTransaction{}
		if err = rtx.Processing(txData); err != nil {
			return nil, err
		}
		rtxs = append(rtxs, rtx.SetRawTx())
		retTx = append(retTx, rtx.HashStr())
	}
	err = model.SendTxBatches(rtxs)
	return
}

type ObsTxPreprocessor struct{}

func (p ObsTxPreprocessor) ProcessClientTranstaction(txData []byte, key int64, le *log.Entry) (string, error) {

	tx, err := transaction.UnmarshallTransaction(bytes.NewBuffer(txData), true)
	if err != nil {
		le.WithFields(log.Fields{"type": consts.ParseError, "error": err}).Error("on unmarshaling user tx")
		return "", err
	}

	ts := &model.TransactionStatus{
		BlockID:  1,
		Hash:     tx.TxHash,
		Time:     time.Now().Unix(),
		WalletID: key,
		Type:     tx.TxType,
	}

	if err := ts.Create(); err != nil {
		le.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on creating tx status")
		return "", err
	}

	res, _, err := tx.CallOBSContract()
	if err != nil {
		le.WithFields(log.Fields{"type": consts.ParseError, "error": err}).Error("on execution contract")
		return "", err
	}

	if err := model.SetTransactionStatusBlockMsg(nil, 1, res, tx.TxHash); err != nil {
		le.WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": tx.TxHash}).Error("updating transaction status block id")
		return "", err
	}

	return string(converter.BinToHex(tx.TxHash)), nil
}
func (p ObsTxPreprocessor) ProcessClientTxBatches(txData [][]byte, key int64, le *log.Entry) ([]string, error) {
	return nil, nil
}
func GetClientTxPreprocessor() types.ClientTxPreprocessor {
	if conf.Config.IsSupportingOBS() {
		return ObsTxPreprocessor{}
	}

	return blockchainTxPreprocessor{}
}

// BlockchainSCRunner implementls SmartContractRunner for blockchain mode
type BlockchainSCRunner struct{}

// RunContract runs smart contract on blockchain mode
func (runner BlockchainSCRunner) RunContract(data, hash []byte, keyID, tnow int64, le *log.Entry) error {
	if err := tx.CreateTransaction(data, hash, keyID, tnow); err != nil {
		le.WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("Executing contract")
		return err
	}

	return nil
}

// OBSSCRunner implementls SmartContractRunner for obs mode
type OBSSCRunner struct{}

// RunContract runs smart contract on obs mode
func (runner OBSSCRunner) RunContract(data, hash []byte, keyID, tnow int64, le *log.Entry) error {
	proc := GetClientTxPreprocessor()
	_, err := proc.ProcessClientTranstaction(data, keyID, le)
	if err != nil {
		le.WithFields(log.Fields{"error": consts.ContractError}).Error("on run internal NewUser")
		return err
	}

	return nil
}

// GetSmartContractRunner returns mode boundede implementation of SmartContractRunner
func GetSmartContractRunner() types.SmartContractRunner {
	if !conf.Config.IsSupportingOBS() {
		return BlockchainSCRunner{}
	}

	return OBSSCRunner{}
}
