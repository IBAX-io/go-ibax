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

type VDETaskData struct {
	DataUUID string `json:"data_uuid"`
	Data     []byte `json:"data"`
	Hash     string `json:"hash"`
	DataInfo string `json:"data_info"`
}

func unmarshalColumnVDESrcData(form *VDESrcDataForm) (*model.VDESrcData, error) {
	var (
		datainfo map[string]interface{}
		taskdata VDETaskData
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

	m := &model.VDESrcData{
		TaskUUID:  form.TaskUUID,
		DataUUID:  taskdata.DataUUID,
		Hash:      taskdata.Hash,
		Data:      taskdata.Data,

	return m, err
}

//
//func unmarshalColumnVDESrcData(form *VDESrcDataForm) (*model.VDESrcData, error) {
//	var (
//		datainfo map[string]interface{}
//		err  error
//	)
//
//	err = json.Unmarshal([]byte(form.DataInfo), &datainfo)
//	if err != nil {
//		log.WithFields(log.Fields{"error": err}).Error("unmarshal DataInfo error")
//	}
//
//	m := &model.VDESrcData{
//		TaskUUID:     form.TaskUUID,
//		DataUUID:     form.DataUUID,
//		Hash:         form.Hash,
//		Data:         []byte(form.Data),
//		DataInfo:     converter.MarshalJson(datainfo),
//		DataState:    int64(form.DataState),
//		DataErr:      form.DataErr,
//	}
//
//	return m, err
//}

type VDETaskdataResult struct {
	TaskUUID string `json:"task_uuid"`
	DataUUID string `json:"data_uuid"`
	Hash     string `json:"hash"`
}

func VDESrcDataCreateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)

	logger := getLogger(r)
	form := &VDESrcDataForm{}
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	m := &model.VDESrcData{}
	if m, err = unmarshalColumnVDESrcData(form); err != nil {
		fmt.Println(err)
		errorResponse(w, err)
		return
	}

	m.CreateTime = time.Now().Unix()
	if err = m.Create(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to insert table")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, &VDETaskdataResult{
		TaskUUID: form.TaskUUID,
		DataUUID: m.DataUUID,
		Hash:     m.Hash,
	})
	return
}

func VDESrcDataUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	form := &VDESrcDataForm{}

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err)
		return
	}

	m := &model.VDESrcData{}

	if m, err = unmarshalColumnVDESrcData(form); err != nil {
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

func VDESrcDataDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.VDESrcData{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete table record")
	}

	jsonResponse(w, "ok")
}

func VDESrcDataListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	srcData := model.VDESrcData{}

	result, err := srcData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading task data list")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

func VDESrcDataByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	srcData := model.VDESrcData{}
	srcData.ID = id
	result, err := srcData.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data by ID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

func VDESrcDataByTaskUUIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	srcData := model.VDESrcData{}
	result, err := srcData.GetAllByTaskUUID(params["taskuuid"])
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query task data by TaskUUID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
