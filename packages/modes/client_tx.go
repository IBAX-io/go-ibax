/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package modes

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/transaction"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
)

var ErrDiffKey = errors.New("Different keys")

type blockchainTxPreprocessor struct{}

func (p blockchainTxPreprocessor) ProcessClientTxBatches(txDatas [][]byte, key int64, le *log.Entry) (retTx []string, err error) {
	var rtxs []*sqldb.RawTx
	for _, txData := range txDatas {
		rtx := &transaction.Transaction{}
		if err = rtx.Unmarshall(bytes.NewBuffer(txData), true); err != nil {
			return nil, err
		}
		rtxs = append(rtxs, rtx.SetRawTx())
		retTx = append(retTx, fmt.Sprintf("%x", rtx.Hash()))
	}
	err = sqldb.SendTxBatches(rtxs)
	return
}

type ClbTxPreprocessor struct{}

/*
func (p ClbTxPreprocessor) ProcessClientTranstaction(txData []byte, key int64, le *log.Entry) (string, error) {

	tx, err := transaction.UnmarshallTransaction(bytes.NewBuffer(txData), true)
	if err != nil {
		le.WithFields(log.Fields{"type": consts.ParseError, "error": err}).Error("on unmarshaling user tx")
		return "", err
	}

	ts := &sqldb.TransactionStatus{
		BlockId:  1,
		Hash:     tx.TxHash,
		Timestamp:     time.Now().Unix(),
		WalletID: key,
		Type:     tx.Rtx.Type(),
	}

	if err := ts.Create(); err != nil {
		le.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on creating tx status")
		return "", err
	}

	res, _, err := tx.CallCLBContract()
	if err != nil {
		le.WithFields(log.Fields{"type": consts.ParseError, "error": err}).Error("on execution contract")
		return "", err
	}

	if err := sqldb.SetTransactionStatusBlockMsg(nil, 1, res, tx.TxHash); err != nil {
		le.WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": tx.TxHash}).Error("updating transaction status block id")
		return "", err
	}

	return string(converter.BinToHex(tx.TxHash)), nil
}*/

func (p ClbTxPreprocessor) ProcessClientTxBatches(txData [][]byte, key int64, le *log.Entry) ([]string, error) {
	return nil, nil
}

func GetClientTxPreprocessor() types.ClientTxPreprocessor {
	if conf.Config.IsSupportingCLB() {
		return ClbTxPreprocessor{}
	}

	return blockchainTxPreprocessor{}
}

// BlockchainSCRunner implementls SmartContractRunner for blockchain mode
type BlockchainSCRunner struct{}

// RunContract runs smart contract on blockchain mode
func (runner BlockchainSCRunner) RunContract(data, hash []byte, keyID, tnow int64, le *log.Entry) error {
	if err := transaction.CreateTransaction(data, hash, keyID, tnow); err != nil {
		le.WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("Executing contract")
		return err
	}

	return nil
}

// CLBSCRunner implementls SmartContractRunner for clb mode
type CLBSCRunner struct{}

// RunContract runs smart contract on clb mode
func (runner CLBSCRunner) RunContract(data, hash []byte, keyID, tnow int64, le *log.Entry) error {
	proc := GetClientTxPreprocessor()
	_, err := proc.ProcessClientTxBatches([][]byte{data}, keyID, le)
	if err != nil {
		le.WithFields(log.Fields{"error": consts.ContractError}).Error("on run internal NewUser")
		return err
	}

	return nil
}

// GetSmartContractRunner returns mode boundede implementation of SmartContractRunner
func GetSmartContractRunner() types.SmartContractRunner {
	if !conf.Config.IsSupportingCLB() {
		return BlockchainSCRunner{}
	}

	return CLBSCRunner{}
}
