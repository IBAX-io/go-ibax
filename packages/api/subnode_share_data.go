/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type taskdataResult struct {
	TaskUUID string `json:"task_uuid"`
	//DataUUID string `json:"data_uuid"`
	Hash string `json:"hash"`
}

func shareDataCreate(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)

	logger := getLogger(r)
	form := &shareDataForm{}
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	m := &model.ShareDataStatus{}
	if m, err = unmarshalColumnShareData(form); err != nil {
		fmt.Println(err)
		errorResponse(w, err)
		return
	}
	m.TaskType = form.TaskType
	m.Time = time.Now().Unix()

	if err = m.TaskDataStatusCreate(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to insert database table")
	}

	//model.DBConn.Last(&m)

	//jsonResponse(w, *m)
	jsonResponse(w, &taskdataResult{
		TaskUUID: form.TaskUUID,
		//DataUUID: m.DataUUID,
		Hash: m.Hash,
	})
	return
}

func shareDataUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	form := &shareDataForm{}

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err)
		return
	}

	m := &model.ShareDataStatus{}

	if m, err = unmarshalColumnShareData(form); err != nil {
		errorResponse(w, err)
		return
	}

	m.ID = id
	m.Time = time.Now().Unix()
	if err = m.Updates(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The update task database failed")
		return
	}

	result, err := m.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to get one-on-one hit data")
		return
	}

	jsonResponse(w, result)
}

func shareDataDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.ShareDataStatus{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete Shared data")
	}

	jsonResponse(w, "ok")
}

func shareDataListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	shareData := model.ShareDataStatus{}

	result, err := shareData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading task data list")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func shareDataByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	shareData := model.ShareDataStatus{}
	shareData.ID = id
	result, err := shareData.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data by ID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func shareDataByTaskUUIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	shareData := model.ShareDataStatus{}
	result, err := shareData.GetAllByTaskUUID(params["taskuuid"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data by TaskUUID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func shareDataStatusByTaskUUIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	shareDataStatus := model.DataUpToChainStatus{}
	result, err := shareDataStatus.GetAllByTaskUUID(params["taskuuid"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data status by TaskUUID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("unmarshal dist error")
	}

	m := &model.ShareDataStatus{
		TaskUUID:     form.TaskUUID,
		TaskName:     form.TaskName,
		TaskSender:   form.TaskSender,
		Hash:         form.Hash,
		Data:         []byte(form.Data),
		Dist:         converter.MarshalJson(dist),
		Ecosystem:    int64(form.Ecosystem),
		TcpSendState: int64(form.TcpSendState),
		//TcpSendStateFlag: form.TcpSendStateFlag,
		TcpSendStateFlag: strings.Repeat("00000000", 32),
		ChainState:       int64(form.ChainState),
		TxHash:           []byte(form.TxHash),
		BlockId:          int64(form.BlockID),
	}

	return m, err
}
