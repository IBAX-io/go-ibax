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

func unmarshalColumnVDESrcTaskAuth(form *VDESrcTaskAuthForm) (*model.VDESrcTaskAuth, error) {
	var (
		err error
	)

	m := &model.VDESrcTaskAuth{
		TaskUUID:             form.TaskUUID,
		Comment:              form.Comment,
		VDEPubKey:            form.VDEPubKey,
		ContractRunHttp:      form.ContractRunHttp,
		ContractRunEcosystem: form.ContractRunEcosystem,
		ChainState:           form.ChainState,
	}

	return m, err
}

func VDESrcTaskAuthCreateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	logger := getLogger(r)
	form := &VDESrcTaskAuthForm{}
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	m := &model.VDESrcTaskAuth{}
	if m, err = unmarshalColumnVDESrcTaskAuth(form); err != nil {
		fmt.Println(err)
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

func VDESrcTaskAuthUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	params := mux.Vars(r)
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err)
		return
	}

	m := &model.VDESrcTaskAuth{}

	if m, err = unmarshalColumnVDESrcTaskAuth(form); err != nil {
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

func VDESrcTaskAuthDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.VDESrcTaskAuth{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete table record")
	}

	jsonResponse(w, "ok")
}

func VDESrcTaskAuthListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	srcData := model.VDESrcTaskAuth{}

	result, err := srcData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading task data list")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

func VDESrcTaskAuthByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	srcData := model.VDESrcTaskAuth{}
	srcData.ID = id
	result, err := srcData.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query member data by ID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func VDESrcTaskAuthByPubKeyHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	srcData := model.VDESrcTaskAuth{}
	result, err := srcData.GetOneByPubKey(params["pubkey"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query member data by pubkey failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func VDESrcTaskAuthByTaskUUIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	srcData := model.VDESrcTaskAuth{}
	result, err := srcData.GetOneByTaskUUID(params["taskuuid"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task auth data by TaskUUID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
