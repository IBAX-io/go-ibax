/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func unmarshalColumnVDESrcChainInfo(form *VDESrcChainInfoForm) (*model.VDESrcChainInfo, error) {
	var (
		err error
	)

	m := &model.VDESrcChainInfo{
		BlockchainHttp:      form.BlockchainHttp,
		BlockchainEcosystem: form.BlockchainEcosystem,
		Comment:             form.Comment,
	}

	return m, err
}

func VDESrcChainInfoCreateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	logger := getLogger(r)
	form := &VDESrcChainInfoForm{}
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		errorResponse(w, err)
		return
	}

	m.CreateTime = time.Now().Unix()

	if err = m.Create(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to insert table")
	}

	model.DBConn.Last(&m)

	jsonResponse(w, *m)
}

func VDESrcChainInfoUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	form := &VDESrcChainInfoForm{}

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err)
		return
	}

	m := &model.VDESrcChainInfo{}

	if m, err = unmarshalColumnVDESrcChainInfo(form); err != nil {
		errorResponse(w, err)
		return
	}

	m.ID = id
	m.UpdateTime = time.Now().Unix()
	if err = m.Updates(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Update table failed")
		return
	}

	result, err := m.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to get table record")
		return
	}

	jsonResponse(w, result)
}

func VDESrcChainInfoDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.VDESrcChainInfo{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete table record")
	}

	jsonResponse(w, "ok")
}

func VDESrcChainInfoListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	srcData := model.VDESrcChainInfo{}

	result, err := srcData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading chain info data list")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

func VDESrcChainInfoByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	srcData := model.VDESrcChainInfo{}
	srcData.ID = id
	result, err := srcData.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query chain info data by ID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
