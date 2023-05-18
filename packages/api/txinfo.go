/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/common"
	"github.com/IBAX-io/go-ibax/packages/types"
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

func getTxInfo(r *http.Request, txHash string, getInfo bool) (*txinfoResult, error) {
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
	if getInfo {
		status.Data, err = transactionData(ltx.Block, hex.EncodeToString(ltx.Hash))
		if err != nil {
			return nil, err
		}
		status.Data.Status = ltx.Status
		status.Data.Ecosystem = ltx.EcosystemID
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
	status, err := getTxInfo(r, params["hash"], form.ContractInfo)
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
		status, err := getTxInfo(r, hash, form.ContractInfo)
		if err != nil {
			errorResponse(w, err)
			return
		}
		result.Results[hash] = status
	}

	jsonResponse(w, result)
}

func transactionData(blockId int64, txHash string) (*smart.TxInfo, error) {
	info := &smart.TxInfo{}
	bk := &sqldb.BlockChain{}
	f, err := bk.Get(blockId)
	if err != nil {
		return nil, err
	}
	if !f {
		return nil, errors.New("not found")
	}

	blck, err := block.UnmarshallBlock(bytes.NewBuffer(bk.Data), false)
	if err != nil {
		return nil, err
	}

	for _, tx := range blck.Transactions {
		hashStr := hex.EncodeToString(tx.Hash())
		//find next
		if hashStr != txHash {
			continue
		}
		info.Address = converter.AddressToString(tx.KeyID())
		info.Hash = hashStr
		info.Size = common.StorageSize(len(tx.Payload())).TerminalString()
		info.CreatedAt = tx.Timestamp()

		if tx.IsSmartContract() {
			info.Expedite = tx.SmartContract().TxSmart.Expedite
			if tx.SmartContract().TxContract != nil {
				info.ContractName = tx.SmartContract().TxContract.Name
			}
			info.Params = tx.SmartContract().TxData
			if tx.Type() == types.TransferSelfTxType {
				info.Params = make(map[string]any)
				info.Params["transferSelf"] = tx.SmartContract().TxSmart.TransferSelf
			}
			if tx.Type() == types.UtxoTxType {
				info.Params = make(map[string]any)
				info.Params["utxo"] = tx.SmartContract().TxSmart.UTXO
			}
		}
		//find out break
		break

	}
	info.BlockId = blck.Header.BlockId
	info.BlockHash = hex.EncodeToString(blck.Header.BlockHash)

	return info, nil
}
