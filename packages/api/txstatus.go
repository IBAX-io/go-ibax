/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

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
		return nil, errHashWrong
	}
	ts := &sqldb.TransactionStatus{}
	found, err := ts.Get([]byte(converter.HexToBin(hash)))
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.ConversionError, "error": err}).Error("getting transaction status by hash")
		return nil, err
	}
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound, "key": []byte(converter.HexToBin(hash))}).Debug("getting transaction status by hash")
		return nil, errHashNotFound.Errorf(hash)
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

type multiTxStatusResult struct {
	Results map[string]*txstatusResult `json:"results"`
}

type txstatusRequest struct {
	Hashes []string `json:"hashes"`
}

func getTxStatusHandler(w http.ResponseWriter, r *http.Request) {
	result := &multiTxStatusResult{}
	result.Results = map[string]*txstatusResult{}

	var request txstatusRequest
	if err := json.Unmarshal([]byte(r.FormValue("data")), &request); err != nil {
		errorResponse(w, errHashWrong)
		return
	}
	for _, hash := range request.Hashes {
		status, err := getTxStatus(r, hash)
		if err != nil {
			errorResponse(w, err)
			return
		}
		result.Results[hash] = status
	}

	jsonResponse(w, result)
}
