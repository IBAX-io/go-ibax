/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/json"
	//"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type TaskData struct {
	DataUUID string `json:"data_uuid"`
	Data     []byte `json:"data"`
	Hash     string `json:"hash"`
	DataInfo string `json:"data_info"`
}

func unmarshalColumnSubNodeSrcData(form *SubNodeSrcDataForm) (*model.SubNodeSrcData, error) {
	var (
		datainfo map[string]interface{}
		taskdata TaskData
		err      error
	)
	err = json.Unmarshal([]byte(form.Data), &taskdata)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("unmarshal Data error")
		return nil, err
	}
	//fmt.Println("taskdata.Data:", string(taskdata.Data))
	//fmt.Println("==========taskdata.Data=========")
	err = json.Unmarshal([]byte(taskdata.DataInfo), &datainfo)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("unmarshal DataInfo error")
		return nil, err
	}

	m := &model.SubNodeSrcData{
		TaskUUID: form.TaskUUID,
		DataUUID: taskdata.DataUUID,
		Hash:     taskdata.Hash,
		Data:     taskdata.Data,
		DataInfo: converter.MarshalJson(datainfo),
		//DataState: int64(form.DataState),
		//DataErr:   form.DataErr,
	}

	return m, err
}

type subnode_taskdataResult struct {
	TaskUUID string `json:"task_uuid"`
	DataUUID string `json:"data_uuid"`
	Hash     string `json:"hash"`
}

func SubNodeSrcDataCreateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)

	logger := getLogger(r)
	m.CreateTime = time.Now().Unix()
	if err = m.Create(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to insert table")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, &subnode_taskdataResult{
		TaskUUID: form.TaskUUID,
		DataUUID: m.DataUUID,
		Hash:     m.Hash,
	})
	return
}

func SubNodeSrcDataUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	form := &SubNodeSrcDataForm{}

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err)
		return
	}

	m := &model.SubNodeSrcData{}

	if m, err = unmarshalColumnSubNodeSrcData(form); err != nil {
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

func SubNodeSrcDataDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.SubNodeSrcData{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete table record")
	}

	jsonResponse(w, "ok")
}

func SubNodeSrcDataListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	srcData := model.SubNodeSrcData{}

	result, err := srcData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading task data list")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

func SubNodeSrcDataByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	srcData := model.SubNodeSrcData{}
	srcData.ID = id
	result, err := srcData.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data by ID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func SubNodeSrcDataByTaskUUIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	srcData := model.SubNodeSrcData{}
	result, err := srcData.GetAllByTaskUUID(params["taskuuid"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data by TaskUUID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
