/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func unmarshalColumnVDESrcTaskFromSche(form *VDESrcTaskFromScheForm) (*model.VDESrcTaskFromSche, error) {
	var (
		parms              map[string]interface{}
		contract_run_parms map[string]interface{}
		err                error
	)

	err = json.Unmarshal([]byte(form.Parms), &parms)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("unmarshal Parms error")
		return nil, err
	}
	err = json.Unmarshal([]byte(form.ContractRunParms), &contract_run_parms)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("unmarshal ContractRunParms error")
		return nil, err
	}
	//fmt.Println("TaskType,TaskState:", form.TaskType, int64(form.TaskType), form.TaskState, int64(form.TaskState))
	m := &model.VDESrcTaskFromSche{
		TaskUUID:            form.TaskUUID,
		TaskName:            form.TaskName,
		TaskSender:          form.TaskSender,
		Comment:             form.Comment,
		Parms:               converter.MarshalJson(parms),
		TaskType:            int64(form.TaskType),
		TaskState:           int64(form.TaskState),
		ContractSrcName:     form.ContractSrcName,
		ContractSrcGet:      form.ContractSrcGet,
		ContractSrcGetHash:  form.ContractSrcGetHash,
		ContractDestName:    form.ContractDestName,
		ContractDestGet:     form.ContractDestGet,
		ContractDestGetHash: form.ContractDestGetHash,

		ContractRunHttp:      form.ContractRunHttp,
		ContractRunEcosystem: form.ContractRunEcosystem,
		ContractRunParms:     converter.MarshalJson(contract_run_parms),

		ContractMode: int64(form.ContractMode),

		ContractStateSrc:     int64(form.ContractStateSrc),
		ContractStateDest:    int64(form.ContractStateDest),
		ContractStateSrcErr:  form.ContractStateSrcErr,
		ContractStateDestErr: form.ContractStateDestErr,

		TaskRunState:    int64(form.TaskRunState),
		TaskRunStateErr: form.TaskRunStateErr,
	)
	logger := getLogger(r)
	form := &VDESrcTaskFromScheForm{}
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	m := &model.VDESrcTaskFromSche{}
	if m, err = unmarshalColumnVDESrcTaskFromSche(form); err != nil {
		fmt.Println(err)
		errorResponse(w, err)
		return
	}

	m.CreateTime = time.Now().Unix()

	if err = m.Create(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to insert sche task table")
	}

	model.DBConn.Last(&m)

	jsonResponse(w, *m)
}

func VDESrcTaskFromScheUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	form := &VDESrcTaskFromScheForm{}

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err)
		return
	}

	m := &model.VDESrcTaskFromSche{}

	if m, err = unmarshalColumnVDESrcTaskFromSche(form); err != nil {
		errorResponse(w, err)
		return
	}
	//fmt.Println("====m.TaskState,m.TaskType:", m.TaskState, m.TaskType)
	m.ID = id
	m.UpdateTime = time.Now().Unix()
	if err = m.Updates(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Update sche task table failed")
		return
	}

	result, err := m.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to get sche task table record")
		return
	}

	jsonResponse(w, result)
}

func VDESrcTaskFromScheDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.VDESrcTaskFromSche{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete sche task table record")
	}

	jsonResponse(w, "ok")
}

func VDESrcTaskFromScheListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	srcData := model.VDESrcTaskFromSche{}

	result, err := srcData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading sche task data list")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

func VDESrcTaskFromScheByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	srcData := model.VDESrcTaskFromSche{}
	srcData.ID = id
	result, err := srcData.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query sche task data by ID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func VDESrcTaskFromScheByTaskUUIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	srcData := model.VDESrcTaskFromSche{}
	result, err := srcData.GetAllByTaskUUID(params["taskuuid"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query sche task data by TaskUUID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
