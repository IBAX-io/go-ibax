/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/gorilla/mux"
)

type txinfoResult struct {
	BlockID string        `json:"blockid"`
	Confirm int           `json:"confirm"`
	Data    *smart.TxInfo `json:"data,omitempty"`
}

type txInfoForm struct {
	nopeValidator
	ContractInfo bool   `schema:"contractinfo"`
	Data         string `schema:"data"`
}

type multiTxInfoResult struct {
	Results map[string]*txinfoResult `json:"results"`
}

func getTxInfo(r *http.Request, txHash string) (*txinfoResult, error) {
	var status txinfoResult
	hash, err := hex.DecodeString(txHash)
	if err != nil {
		return nil, errHashWrong
	}
	ltx := &sqldb.LogTransaction{Hash: hash}
	found, err := ltx.GetByHash(nil, hash)
	if err != nil {
		return nil, err
	}
	if !found {
		return &status, nil
	}
	status.BlockID = converter.Int64ToStr(ltx.Block)
	var confirm sqldb.Confirmation
	found, err = confirm.GetConfirmation(ltx.Block)
	if err != nil {
		return nil, err
	}
	if found {
		status.Confirm = int(confirm.Good)
	}

	return &status, nil
}

func getTxInfoHandler(w http.ResponseWriter, r *http.Request) {
	form := &txInfoForm{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	params := mux.Vars(r)
	status, err := getTxInfo(r, params["hash"])
	if err != nil {
		errorResponse(w, err)
		return
	}

	jsonResponse(w, status)
}

func getTxInfoMultiHandler(w http.ResponseWriter, r *http.Request) {
	form := &txInfoForm{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	result := &multiTxInfoResult{}
	result.Results = map[string]*txinfoResult{}
	hashes := strings.Split(form.Data, ",")
	for _, hash := range hashes {
		status, err := getTxInfo(r, hash)
		if err != nil {
			errorResponse(w, err)
			return
		}
		result.Results[hash] = status
	}

	jsonResponse(w, result)
}
