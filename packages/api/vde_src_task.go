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
	"github.com/IBAX-io/go-ibax/packages/crypto"
	"github.com/IBAX-io/go-ibax/packages/model"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func unmarshalColumnVDESrcTask(form *VDESrcTaskForm) (*model.VDESrcTask, error) {
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
	m := &model.VDESrcTask{
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

		TxHash:     form.TxHash,
		ChainState: int64(form.ChainState),
		BlockId:    int64(form.BlockId),
		ChainId:    int64(form.ChainId),
		ChainErr:   form.ChainErr,
	}

	return m, err
}

func VDESrcTaskCreateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		ContractSrcGetHashHex  string
		ContractDestGetHashHex string
		err                    error
	)
	logger := getLogger(r)
	form := &VDESrcTaskForm{}
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	m := &model.VDESrcTask{}
	if m, err = unmarshalColumnVDESrcTask(form); err != nil {
		fmt.Println(err)
		errorResponse(w, err)
		return
	}
	//
	if len(m.ContractSrcGetHash) == 0 {
		if ContractSrcGetHashHex, err = crypto.HashHex([]byte(m.ContractSrcGet)); err != nil {
			fmt.Println("ContractSrcGetHashHex Raw data hash failed ")
			errorResponse(w, err)
			return
		}
		m.ContractSrcGetHash = ContractSrcGetHashHex
	}
	if len(m.ContractDestGetHash) == 0 {
		if ContractDestGetHashHex, err = crypto.HashHex([]byte(m.ContractDestGet)); err != nil {
			fmt.Println("ContractDestGetHashHex Raw data hash failed ")
			errorResponse(w, err)
			return
		}
		m.ContractDestGetHash = ContractDestGetHashHex
	}
	if m.ContractMode == 0 {
		m.ContractMode = 3 //
	}

	m.CreateTime = time.Now().Unix()

	if err = m.Create(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to insert table")
	}

	model.DBConn.Last(&m)

	jsonResponse(w, *m)
}

func VDESrcTaskUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		ContractSrcGetHashHex  string
		ContractDestGetHashHex string
		err                    error
	)
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	form := &VDESrcTaskForm{}

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err)
		return
	}

	m := &model.VDESrcTask{}

	if m, err = unmarshalColumnVDESrcTask(form); err != nil {
		errorResponse(w, err)
		return
	}
	//fmt.Println("====m.TaskState,m.TaskType:", m.TaskState, m.TaskType)
	//
	if len(m.ContractSrcGetHash) == 0 {
		if ContractSrcGetHashHex, err = crypto.HashHex([]byte(m.ContractSrcGet)); err != nil {
			fmt.Println("ContractSrcGetHashHex Raw data hash failed ")
		m.ContractDestGetHash = ContractDestGetHashHex
	}
	if m.ContractMode == 0 {
		m.ContractMode = 3 //
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

func VDESrcTaskDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.VDESrcTask{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete table record")
	}

	jsonResponse(w, "ok")
}

func VDESrcTaskListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	srcData := model.VDESrcTask{}

	result, err := srcData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading task data list")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

func VDESrcTaskByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	srcData := model.VDESrcTask{}
	srcData.ID = id
	result, err := srcData.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data by ID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func VDESrcTaskByTaskUUIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	srcData := model.VDESrcTask{}
	result, err := srcData.GetAllByTaskUUID(params["taskuuid"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data by TaskUUID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
