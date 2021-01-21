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

func unmarshalColumnVDESrcMember(form *VDESrcMemberForm) (*model.VDESrcMember, error) {
	var (
		err error
	)

	m := &model.VDESrcMember{
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

func VDESrcMemberCreateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	logger := getLogger(r)
	form := &VDESrcMemberForm{}
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	m := &model.VDESrcMember{}
	if m, err = unmarshalColumnVDESrcMember(form); err != nil {
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

func VDESrcMemberUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	form := &VDESrcMemberForm{}

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err)
		return
	}

	m := &model.VDESrcMember{}

	if m, err = unmarshalColumnVDESrcMember(form); err != nil {
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

func VDESrcMemberDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.VDESrcMember{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete table record")
	}

	jsonResponse(w, "ok")
}

func VDESrcMemberListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	srcData := model.VDESrcMember{}

	result, err := srcData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading task data list")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

func VDESrcMemberByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	srcData := model.VDESrcMember{}
	srcData.ID = id
		return
	}

	jsonResponse(w, result)
}

func VDESrcMemberByPubKeyHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	srcData := model.VDESrcMember{}
	result, err := srcData.GetOneByPubKey(params["pubkey"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query member data by pubkey failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
