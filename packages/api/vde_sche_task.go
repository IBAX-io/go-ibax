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

func unmarshalColumnVDEScheTask(form *VDEScheTaskForm) (*model.VDEScheTask, error) {
	var (
		parms              map[string]interface{}
		contract_run_parms map[string]interface{}
		err                error
	)

	err = json.Unmarshal([]byte(form.Parms), &parms)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("unmarshal Parms error")
	}
	err = json.Unmarshal([]byte(form.ContractRunParms), &contract_run_parms)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("unmarshal ContractRunParms error")
		return nil, err
	}

	m := &model.VDEScheTask{
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

func VDEScheTaskCreateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		ContractSrcGetHashHex  string
		ContractDestGetHashHex string
		err                    error
	)
	logger := getLogger(r)
	form := &VDEScheTaskForm{}
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	m := &model.VDEScheTask{}
	if m, err = unmarshalColumnVDEScheTask(form); err != nil {
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
		m.ContractMode = 4 // encryption up to chain
	}

	m.CreateTime = time.Now().Unix()

	if err = m.Create(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to insert table")
	}
	model.DBConn.Last(&m)

	jsonResponse(w, *m)
}

func VDEScheTaskUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		ContractSrcGetHashHex  string
		ContractDestGetHashHex string
		err                    error
	)
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	form := &VDEScheTaskForm{}

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err)
		return
	}

	m := &model.VDEScheTask{}

	if m, err = unmarshalColumnVDEScheTask(form); err != nil {
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
		m.ContractMode = 4 //encryption up to chain
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to get table record")
		return
	}

	jsonResponse(w, result)
}

func VDEScheTaskDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.VDEScheTask{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete table record")
	}

	jsonResponse(w, "ok")
}

func VDEScheTaskListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	srcData := model.VDEScheTask{}

	result, err := srcData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading task data list")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

func VDEScheTaskByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	srcData := model.VDEScheTask{}
	srcData.ID = id
	result, err := srcData.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data by ID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func VDEScheTaskByTaskUUIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	srcData := model.VDEScheTask{}
	result, err := srcData.GetAllByTaskUUID(params["taskuuid"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data by TaskUUID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
