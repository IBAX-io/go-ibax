/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/hex"
	"io"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"

	log "github.com/sirupsen/logrus"
)

type sendTxResult struct {
	Hashes map[string]string `json:"hashes"`
}

func getTxData(r *http.Request, key string) ([]byte, error) {
	logger := getLogger(r)

	file, _, err := r.FormFile(key)
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("request.FormFile")
		return nil, err
	}
	defer file.Close()

	var txData []byte
	if txData, err = io.ReadAll(file); err != nil {
		logger.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("reading multipart file")
		return nil, err
	}

	return txData, nil
}

func (m Mode) sendTxHandler(w http.ResponseWriter, r *http.Request) {
	client := getClient(r)

	if block.IsKeyBanned(client.KeyID) {
		errorResponse(w, errBannded.Errorf(block.BannedTill(client.KeyID)))
		return
	}

	err := r.ParseMultipartForm(multipartBuf)
	if err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	result := &sendTxResult{Hashes: make(map[string]string)}
	var mtx = make(map[string][]byte, 0)
	for key := range r.MultipartForm.File {
		txData, err := getTxData(r, key)
		if err != nil {
			errorResponse(w, err)
			return
		if err != nil {
			errorResponse(w, err)
			return
		}
		mtx[key] = txData
	}

	hash, err := txHandlerBatches(r, m, mtx)
	if err != nil {
		errorResponse(w, err)
		return
	}
	for _, key := range hash {
		result.Hashes[key] = key
	}
	jsonResponse(w, result)
}

func (m Mode) sendSignTxHandler(w http.ResponseWriter, r *http.Request) {
	//client := getClient(r)
	keyid := int64(0)
	if block.IsKeyBanned(keyid) {
		errorResponse(w, errBannded.Errorf(block.BannedTill(keyid)))
		return
	}

	err := r.ParseMultipartForm(multipartBuf)
	if err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	result := &sendTxResult{Hashes: make(map[string]string)}
	for key := range r.MultipartForm.File {
		txData, err := getTxData(r, key)
		if err != nil {
			errorResponse(w, err)
			return
		}

		hash, err := txSignHandler(r, keyid, txData, m)
		if err != nil {
			errorResponse(w, err)
			return
		}
		result.Hashes[key] = hash
	}

	for key := range r.Form {
		txData, err := hex.DecodeString(r.FormValue(key))
		if err != nil {
			errorResponse(w, err)
			return
		}

		hash, err := txSignHandler(r, keyid, txData, m)
		if err != nil {
			errorResponse(w, err)
			return
		}
		result.Hashes[key] = hash
	}

	jsonResponse(w, result)
}

func txHandler(r *http.Request, txData []byte, m Mode) (string, error) {
	client := getClient(r)
	logger := getLogger(r)
	if int64(len(txData)) > syspar.GetMaxTxSize() {
		logger.WithFields(log.Fields{"type": consts.ParameterExceeded, "max_size": syspar.GetMaxTxSize(), "size": len(txData)}).Error("transaction size exceeds max size")
		block.BadTxForBan(client.KeyID)
		return "", errLimitTxSize.Errorf(len(txData))
	}

	hash, err := m.ClientTxProcessor.ProcessClientTranstaction(txData, client.KeyID, logger)
	if err != nil {
		return "", err
	}

	return hash, nil
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
		block.BadTxForBan(client.KeyID)
		return nil, errLimitTxSize.Errorf(len(txData))
	}

	hash, err := m.ClientTxProcessor.ProcessClientTxBatches(txData, client.KeyID, logger)
	if err != nil {
		return nil, err
	}

	return hash, nil
}

func txSignHandler(r *http.Request, keyid int64, txData []byte, m Mode) (string, error) {
	logger := getLogger(r)

	if int64(len(txData)) > syspar.GetMaxTxSize() {
		logger.WithFields(log.Fields{"type": consts.ParameterExceeded, "max_size": syspar.GetMaxTxSize(), "size": len(txData)}).Error("transaction size exceeds max size")
		block.BadTxForBan(keyid)
		return "", errLimitTxSize.Errorf(len(txData))
	}

	hash, err := m.ClientTxProcessor.ProcessClientTranstaction(txData, keyid, logger)
	if err != nil {
		return "", err
	}

	return hash, nil
}
