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

func unmarshalColumnVDEDestMember(form *VDEDestMemberForm) (*model.VDEDestMember, error) {
	var (
		err error
	)

	m := &model.VDEDestMember{
		VDEPubKey:            form.VDEPubKey,
		VDEComment:           form.VDEComment,
		VDEName:              form.VDEName,
		VDEIp:                form.VDEIp,
		VDEType:              int64(form.VDEType),
		ContractRunHttp:      form.ContractRunHttp,
		ContractRunEcosystem: form.ContractRunEcosystem,
	}

	return m, err
}

func VDEDestMemberCreateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	logger := getLogger(r)
	form := &VDEDestMemberForm{}
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	m := &model.VDEDestMember{}
	if m, err = unmarshalColumnVDEDestMember(form); err != nil {
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

func VDEDestMemberUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	form := &VDEDestMemberForm{}

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err)
		return
	}

		return
	}

	result, err := m.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to get table record")
		return
	}

	jsonResponse(w, result)
}

func VDEDestMemberDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.VDEDestMember{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete table record")
	}

	jsonResponse(w, "ok")
}

func VDEDestMemberListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	srcData := model.VDEDestMember{}

	result, err := srcData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading task data list")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

func VDEDestMemberByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	srcData := model.VDEDestMember{}
	srcData.ID = id
	result, err := srcData.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query member data by ID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func VDEDestMemberByPubKeyHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	srcData := model.VDEDestMember{}
	result, err := srcData.GetOneByPubKey(params["pubkey"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query member data by pubkey failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
