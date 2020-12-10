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

func unmarshalColumnVDEAgentMember(form *VDEAgentMemberForm) (*model.VDEAgentMember, error) {
	var (
		err error
	)

	m := &model.VDEAgentMember{
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

	if err = m.Create(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to insert table")
	}

	model.DBConn.Last(&m)

	jsonResponse(w, *m)
}

func VDEAgentMemberUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	form := &VDEAgentMemberForm{}

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err)
		return
	}

	m := &model.VDEAgentMember{}

	if m, err = unmarshalColumnVDEAgentMember(form); err != nil {
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

func VDEAgentMemberDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.VDEAgentMember{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete table record")
	}

	jsonResponse(w, "ok")
}

func VDEAgentMemberListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	srcData := model.VDEAgentMember{}

	result, err := srcData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading task data list")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

func VDEAgentMemberByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	srcData := model.VDEAgentMember{}
	srcData.ID = id
	result, err := srcData.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query member data by ID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func VDEAgentMemberByPubKeyHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	srcData := model.VDEAgentMember{}
	result, err := srcData.GetOneByPubKey(params["pubkey"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query member data by pubkey failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
