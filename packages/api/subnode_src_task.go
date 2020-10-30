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

func unmarshalColumnSubNodeSrcTask(form *SubNodeSrcTaskForm) (*model.SubNodeSrcTask, error) {
	var (
		parms          map[string]interface{}
		task_run_parms map[string]interface{}
		err            error
	)

	err = json.Unmarshal([]byte(form.Parms), &parms)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("unmarshal Parms error")
		return nil, err
	}
	err = json.Unmarshal([]byte(form.TaskRunParms), &task_run_parms)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("unmarshal TaskRunParms error")
		return nil, err
	}
	//fmt.Println("TaskType,TaskState:", form.TaskType, int64(form.TaskType), form.TaskState, int64(form.TaskState))
	m := &model.SubNodeSrcTask{
		TaskUUID:   form.TaskUUID,
		TaskName:   form.TaskName,
		TaskSender: form.TaskSender,
		Comment:    form.Comment,
		Parms:      converter.MarshalJson(parms),
		TaskType:   int64(form.TaskType),
		TaskState:  int64(form.TaskState),

		TaskRunParms: converter.MarshalJson(task_run_parms),
		//TaskRunState:    int64(form.TaskRunState),
		//TaskRunStateErr: form.TaskRunStateErr,

		//TxHash:     form.TxHash,
		//ChainState: int64(form.ChainState),
		//BlockId:    int64(form.BlockId),
		//ChainId:    int64(form.ChainId),
		//ChainErr:   form.ChainErr,
	}

	return m, err
}

func SubNodeSrcTaskCreateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	logger := getLogger(r)
	form := &SubNodeSrcTaskForm{}
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	m := &model.SubNodeSrcTask{}
	if m, err = unmarshalColumnSubNodeSrcTask(form); err != nil {
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

func SubNodeSrcTaskUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	form := &SubNodeSrcTaskForm{}

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err)
		return
	}

	m := &model.SubNodeSrcTask{}

	if m, err = unmarshalColumnSubNodeSrcTask(form); err != nil {
		errorResponse(w, err)
		return
	}
	//fmt.Println("====m.TaskState,m.TaskType:", m.TaskState, m.TaskType)

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

func SubNodeSrcTaskDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.SubNodeSrcTask{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete table record")
	}

	jsonResponse(w, "ok")

	result, err := srcData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading task data list")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

func SubNodeSrcTaskByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	srcData := model.SubNodeSrcTask{}
	srcData.ID = id
	result, err := srcData.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data by ID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func SubNodeSrcTaskByTaskUUIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	srcData := model.SubNodeSrcTask{}
	result, err := srcData.GetAllByTaskUUID(params["taskuuid"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data by TaskUUID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
