/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strings"
)

type transactionApi struct {
}

func newTransactionApi() *transactionApi {
	return &transactionApi{}
}

type sendTxResult struct {
	Hashes map[string]string `json:"hashes"`
}

func txHandlerBatches(r *http.Request, m Mode, mtx map[string][]byte) ([]string, error) {
	client := getClient(r)
	logger := getLogger(r)
	var txData [][]byte
	for _, datum := range mtx {
		txData = append(txData, datum)
	}
	if int64(len(txData)) > syspar.GetMaxTxSize() {
		logger.WithFields(log.Fields{"type": consts.ParameterExceeded, "max_size": syspar.GetMaxTxSize(), "size": len(txData)}).Error("transaction size exceeds max size")
		transaction.BadTxForBan(client.KeyID)
		return nil, fmt.Errorf("the size of tx is too big (%d)", len(txData))
	}

	hash, err := m.ClientTxProcessor.ProcessClientTxBatches(txData, client.KeyID, logger)
	if err != nil {
		return nil, err
	}

	return hash, nil
}

func (t *transactionApi) SendTx(ctx RequestContext, auth Auth, mtx map[string][]byte) (*sendTxResult, *Error) {
	r := ctx.HTTPRequest()
	client := getClient(r)

	if transaction.IsKeyBanned(client.KeyID) {
		return nil, DefaultError(fmt.Sprintf("The key %d is banned till %s", client.KeyID, transaction.BannedTill(client.KeyID)))
	}
	if mtx == nil {
		return nil, InvalidParamsError(paramsEmpty)
	}

	result := &sendTxResult{Hashes: make(map[string]string)}

	hash, err := txHandlerBatches(r, auth.Mode, mtx)
	if err != nil {
		return nil, DefaultError(err.Error())
	}
	for _, key := range hash {
		result.Hashes[key] = key
	}
	return result, nil
}

type txstatusError struct {
	Type  string `json:"type,omitempty"`
	Error string `json:"error,omitempty"`
	Id    string `json:"id,omitempty"`
}

type txstatusResult struct {
	BlockID string         `json:"blockid"`
	Message *txstatusError `json:"errmsg,omitempty"`
	Result  string         `json:"result"`
	Penalty int64          `json:"penalty"`
}

func getTxStatus(r *http.Request, hash string) (*txstatusResult, error) {
	logger := getLogger(r)

	var status txstatusResult
	if _, err := hex.DecodeString(hash); err != nil {
		logger.WithFields(log.Fields{"type": consts.ConversionError, "error": err}).Error("decoding tx hash from hex")
		return nil, errors.New("hash is incorrect")
	}
	ts := &sqldb.TransactionStatus{}
	found, err := ts.Get([]byte(converter.HexToBin(hash)))
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.ConversionError, "error": err}).Error("getting transaction status by hash")
		return nil, err
	}
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound, "key": []byte(converter.HexToBin(hash))}).Debug("getting transaction status by hash")
		return nil, errors.New(fmt.Sprintf("hash %s has not been found", hash))
	}
	checkErr := func() {
		if len(ts.Error) > 0 {
			if err := json.Unmarshal([]byte(ts.Error), &status.Message); err != nil {
				logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "text": ts.Error, "error": err}).Warn("unmarshalling txstatus error")
				status.Message = &txstatusError{
					Type:  "txError",
					Error: ts.Error,
				}
			}
		}
	}
	if ts.BlockID > 0 {
		status.BlockID = converter.Int64ToStr(ts.BlockID)
		status.Penalty = ts.Penalty
		if ts.Penalty == 1 {
			checkErr()
		} else {
			status.Result = ts.Error
		}
	} else {
		checkErr()
	}
	return &status, nil
}

func (t *transactionApi) TxStatus(ctx RequestContext, auth Auth, hashes string) (*map[string]*txstatusResult, *Error) {
	result := map[string]*txstatusResult{}
	if hashes == "" {
		return nil, InvalidParamsError(paramsEmpty)
	}

	list := strings.Split(hashes, ",")

	r := ctx.HTTPRequest()

	for _, hash := range list {
		status, err := getTxStatus(r, hash)
		if err != nil {
			return nil, DefaultError(err.Error())
		}
		result[hash] = status
	}

	return &result, nil
}

func (b *transactionApi) TxInfo(hash string, contractInfo *bool) (*TxInfoResult, *Error) {
	if hash == "" {
		return nil, InvalidParamsError(paramsEmpty)
	}
	var getInfo bool
	if contractInfo != nil {
		getInfo = *contractInfo
	}
	status, err := getTxInfo(hash, getInfo)
	if err != nil {
		return nil, DefaultError(err.Error())
	}
	return status, nil
}

func (b *transactionApi) TxInfoMultiple(hashList []string, contractInfo *bool) (*MultiTxInfoResult, *Error) {
	if hashList == nil {
		return nil, InvalidParamsError(paramsEmpty)
	}
	result := &MultiTxInfoResult{
		Results: make(map[string]*TxInfoResult),
	}
	var getInfo bool
	if contractInfo != nil {
		getInfo = *contractInfo
	}
	for _, hash := range hashList {
		status, err := getTxInfo(hash, getInfo)
		if err != nil {
			return nil, DefaultError(err.Error())
		}
		result.Results[hash] = status
	}

	return result, nil
}
