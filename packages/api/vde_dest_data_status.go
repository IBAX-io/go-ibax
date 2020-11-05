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

func unmarshalColumnVDEDestDataStatus(form *VDEDestDataStatusForm) (*model.VDEDestDataStatus, error) {
	var (
		datainfo map[string]interface{}
		err      error
	)

	err = json.Unmarshal([]byte(form.DataInfo), &datainfo)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("unmarshal DataInfo error")
	}

	m := &model.VDEDestDataStatus{
		TaskUUID:       form.TaskUUID,
		DataUUID:       form.DataUUID,
		Hash:           form.Hash,
		Data:           []byte(form.Data),
		DataInfo:       converter.MarshalJson(datainfo),
		VDESrcPubkey:   form.VDESrcPubkey,
		VDEDestPubkey:  form.VDEDestPubkey,
		VDEDestIp:      form.VDEDestIp,
		VDEAgentPubkey: form.VDEAgentPubkey,
		VDEAgentIp:     form.VDEAgentIp,
		AgentMode:      int64(form.AgentMode),
		AuthState:      int64(form.AuthState),
		SignState:      int64(form.SignState),
		HashState:      int64(form.HashState),
	}

	return m, err
}

func VDEDestDataStatusCreateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)

	logger := getLogger(r)
	form := &VDEDestDataStatusForm{}
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	m := &model.VDEDestDataStatus{}
	if m, err = unmarshalColumnVDEDestDataStatus(form); err != nil {
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

func VDEDestDataStatusUpdateHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err error
	)
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

func VDEDestDataStatusDeleteHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	id := converter.StrToInt64(params["id"])

	m := &model.VDEDestDataStatus{}
	m.ID = id
	if err := m.Delete(); err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Failed to delete table record")
	}

	jsonResponse(w, "ok")
}

func VDEDestDataStatusListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	destData := model.VDEDestDataStatus{}

	result, err := destData.GetAll()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading task data list")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

func VDEDestDataStatusByIDHandlre(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	id := converter.StrToInt64(params["id"])
	srcData := model.VDEDestDataStatus{}
	srcData.ID = id
	result, err := srcData.GetOneByID()
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query data by ID failed")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}

type DataList struct {
	ID             int64  `json:"id"`
	DataUUID       string `json:"data_uuid"`
	TaskUUID       string `json:"task_uuid"`
	Hash           string `json:"hash"`
	Data           []byte `json:"data"`
	DataInfo       string `json:"data_info"`
	VDESrcPubkey   string `json:"vde_src_pubkey"`
	VDEDestPubkey  string `json:"vde_dest_pubkey"`
	VDEDestIp      string `json:"vde_dest_ip"`
	VDEAgentPubkey string `json:"vde_agent_pubkey"`
	VDEAgentIp     string `json:"vde_agent_ip"`
	AgentMode      int64  `json:"agent_mode"`
	CreateTime     int64  `json:"create_time"`
}
type VDEDestDataStatusList struct {
	Count int64      `json:"count"`
	List  []DataList `json:"list"`
}

//type VDEDestDataStatusList struct {
//	Count string `json:"count"`
//	List []struct {
//		ID string `json:"id"`
//		DataUUID         string `json:"data_uuid"`
//		TaskUUID         string `json:"task_uuid"`
//		Hash             string `json:"hash"`
//		Data             []byte `json:"data"`
//		DataInfo         string `json:"data_info"`
//		VDESrcPubkey     string `json:"vde_src_pubkey"`
//		VDEDestPubkey    string `json:"vde_dest_pubkey"`
//		VDEDestIp        string `json:"vde_dest_ip"`
//		VDEAgentPubkey   string `json:"vde_agent_pubkey"`
//		VDEAgentIp       string `json:"vde_agent_ip"`
//		AgentMode        int64  `json:"agent_mode"`
//		CreateTime       int64  `json:"create_time"`
//	} `json:"list"`
//}

func VDEDestDataStatusByTaskUUIDHandlre(w http.ResponseWriter, r *http.Request) {
	var (
		err            error
		DataStatusList VDEDestDataStatusList
	)

	logger := getLogger(r)
	form := &ListVDEDestDataStatusForm{}
	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	fmt.Println("List TaskUUID:", form.TaskUUID)
	fmt.Println("List BeginTime:", form.BeginTime)
	fmt.Println("List EndTime:", form.EndTime)

	destData := model.VDEDestDataStatus{}
	result, err := destData.GetAllByTaskUUIDAndDataStatusAndTime(form.TaskUUID, 1, 1, 1, form.BeginTime, form.EndTime)
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("The query data by TaskUUID failed")
		errorResponse(w, err)
		return
	}
	if len(result) > 0 {
		for _, item := range result {
			var DataListItem DataList
			DataListItem.ID = item.ID
			DataListItem.TaskUUID = item.TaskUUID
			DataListItem.DataUUID = item.DataUUID
			DataListItem.Hash = item.Hash
			DataListItem.Data = item.Data
			DataListItem.DataInfo = item.DataInfo
			DataListItem.VDESrcPubkey = item.VDESrcPubkey
			DataListItem.VDEDestPubkey = item.VDEDestPubkey
			DataListItem.VDEDestIp = item.VDEDestIp
			DataListItem.VDEAgentPubkey = item.VDEAgentPubkey
			DataListItem.VDEAgentIp = item.VDEAgentIp
			DataListItem.AgentMode = item.AgentMode
			DataListItem.CreateTime = item.CreateTime

			DataStatusList.List = append(DataStatusList.List, DataListItem)
			DataStatusList.Count = DataStatusList.Count + 1
		}
		//fmt.Println("DataStatusList:",DataStatusList)
		jsonResponse(w, DataStatusList)
	} else {
		DataStatusList.Count = 0
		DataStatusList.List = []DataList{}
		jsonResponse(w, DataStatusList)
	}
}
