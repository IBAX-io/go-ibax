/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/converter"

	log "github.com/sirupsen/logrus"

	"time"

	"github.com/IBAX-io/go-ibax/packages/model"
)

func VDESrcData(ctx context.Context, d *daemon) error {
	var (
		TaskParms map[string]interface{}

		vde_src_pubkey       string
		vde_dest_pubkey      string
		vde_dest_ip          string
		vde_agent_pubkey     string
		vde_agent_ip         string
		agent_mode           string
		hash_mode            string
		log_mode             string
		blockchain_http      string
		blockchain_ecosystem string

		chain_state int64

		ok  bool
		err error
	)

	m := &model.VDESrcData{}
	ShareData, err := m.GetAllByDataStatus(0) //0
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data")
		time.Sleep(time.Millisecond * 100)
		return err
	}
	if len(ShareData) == 0 {
		//log.Info("task data not found")
		time.Sleep(time.Millisecond * 100)
		return nil
	}

	// deal with task data
	var TaskParms_Str string
	for _, item := range ShareData {
		//fmt.Println("SrcData:", item.TaskUUID, item.DataUUID)

		m := &model.VDESrcTask{}
		ShareTask, err := m.GetAllByTaskUUIDAndTaskState(item.TaskUUID, 1) //1
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("VDESrcData VDESrcTask getting one task by TaskUUID")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		if len(ShareTask) > 0 {
			TaskParms_Str = ShareTask[0].Parms
		} else {
			m2 := &model.VDESrcTaskFromSche{}
			ShareTask2, err := m2.GetAllByTaskUUIDAndTaskState(item.TaskUUID, 1) //1
			if err != nil {
				log.WithFields(log.Fields{"error": err}).Error("VDESrcData VDESrcTask getting one task by TaskUUID")
				time.Sleep(time.Millisecond * 2)
				continue
			}
			if len(ShareTask2) > 0 {
				TaskParms_Str = ShareTask2[0].Parms
			} else {
				log.WithFields(log.Fields{"error": err}).Error("VDESrcData VDESrcTask getting one task by TaskUUID not found")
				item.DataState = 2 //
				err = item.Updates()
				if err != nil {
					log.WithError(err)
				}
				time.Sleep(time.Millisecond * 2)
				continue
			}
		}

		//ShareTask, err := m.GetOneByTaskUUID(item.TaskUUID, 1)  //1
		//if err != nil {
		//
		//	log.WithFields(log.Fields{"error": err}).Error("VDESrcData VDESrcTask getting one task by TaskUUID")
		//	time.Sleep(time.Millisecond * 100)
		//	item.DataState = 2 //
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}

		//err = json.Unmarshal([]byte(ShareTask.Parms), &TaskParms)
		err = json.Unmarshal([]byte(TaskParms_Str), &TaskParms)
		if err != nil {
			log.Info("Error parsing task parameter")
			log.WithError(err)
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}

		if vde_src_pubkey, ok = TaskParms["vde_src_pubkey"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("src_vde_pubkey parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if vde_dest_pubkey, ok = TaskParms["vde_dest_pubkey"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("vde_dest_pubkey parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if vde_dest_ip, ok = TaskParms["vde_dest_ip"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("vde_dest_ip parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if vde_agent_pubkey, ok = TaskParms["vde_agent_pubkey"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("vde_agent_pubkey parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if vde_agent_ip, ok = TaskParms["vde_agent_ip"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("vde_agent_ip parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if agent_mode, ok = TaskParms["agent_mode"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("agent_mode parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if hash_mode, ok = TaskParms["hash_mode"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("hash_mode parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if log_mode, ok = TaskParms["log_mode"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("log_mode parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if blockchain_http, ok = TaskParms["blockchain_http"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("blockchain_http parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if blockchain_ecosystem, ok = TaskParms["blockchain_ecosystem"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("blockchain_ecosystem parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		//fmt.Println("TaskParms:",TaskParms)
		// fmt.Println("TaskParms:")
		// fmt.Println("vde_src_pubkey:",vde_src_pubkey)
		// fmt.Println("vde_dest_pubkey:",vde_dest_pubkey)
		// fmt.Println("vde_dest_ip:",vde_dest_ip)
		// fmt.Println("vde_agent_pubkey:",vde_agent_pubkey)
		// fmt.Println("vde_agent_ip:",vde_agent_ip)
		//fmt.Println("agent_mode,hash_mode,log_mode:", agent_mode, hash_mode, log_mode)
		//fmt.Println("blockchain_http,blockchain_ecosystem:", blockchain_http, blockchain_ecosystem)

		//Handle the case of multiple target VDE nodes
		vde_dest_pubkey_slice := strings.Split(vde_dest_pubkey, ";")
		vde_dest_ip_slice := strings.Split(vde_dest_ip, ";")
		vde_agent_pubkey_slice := strings.Split(vde_agent_pubkey, ";")
		vde_agent_ip_slice := strings.Split(vde_agent_ip, ";")
		agent_mode_slice := strings.Split(agent_mode, ";")

		vde_dest_num := len(vde_dest_pubkey_slice)
		if len(vde_dest_ip_slice) != vde_dest_num {
			log.WithFields(log.Fields{"error": err}).Error("vde_dest_ip parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if len(vde_agent_pubkey_slice) != vde_dest_num {
			log.WithFields(log.Fields{"error": err}).Error("vde_agent_pubkey parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if len(vde_agent_ip_slice) != vde_dest_num {
			log.WithFields(log.Fields{"error": err}).Error("vde_agent_ip parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if len(agent_mode_slice) != vde_dest_num {
			log.WithFields(log.Fields{"error": err}).Error("agent_mode parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}

		for index, vde_dest_pubkey_item := range vde_dest_pubkey_slice {
			//Generate data send request
			SrcDataStatus := model.VDESrcDataStatus{
				DataUUID:       item.DataUUID,
				TaskUUID:       item.TaskUUID,
				Hash:           item.Hash,
				Data:           item.Data,
				DataInfo:       item.DataInfo,
				VDESrcPubkey:   vde_src_pubkey,
				VDEDestPubkey:  vde_dest_pubkey_item,
				VDEDestIP:      vde_dest_ip_slice[index],
				VDEAgentPubkey: vde_agent_pubkey_slice[index],
				VDEAgentIP:     vde_agent_ip_slice[index],
				AgentMode:      converter.StrToInt64(agent_mode_slice[index]),
				CreateTime:     time.Now().Unix()}

			if err = SrcDataStatus.Create(); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Insert vde_src_data_status table failed")
				continue
			}
		}
		////Generate data send request
		//SrcDataStatus := model.VDESrcDataStatus{
		//	DataUUID:       item.DataUUID,
		//	TaskUUID:       item.TaskUUID,
		//	Hash:           item.Hash,
		//	Data:           item.Data,
		//	DataInfo:       item.DataInfo,
		//	VDESrcPubkey:   vde_src_pubkey,
		//	VDEDestPubkey:  vde_dest_pubkey,
		//	VDEDestIP:      vde_dest_ip,
		//	VDEAgentPubkey: vde_agent_pubkey,
		//	VDEAgentIP:     vde_agent_ip,
		//	AgentMode:      converter.StrToInt64(agent_mode),
		//	CreateTime:     time.Now().Unix()}
		//
		//if err = SrcDataStatus.Create(); err != nil {
		//	log.WithFields(log.Fields{"error": err}).Error("Insert vde_src_data_status table failed")
		//	continue
		//}
		//fmt.Println("Insert vde_src_data_status table ok")

		//Generate a chain request on the Data
		if hash_mode == "1" { //1
			SrcDataHash := model.VDESrcDataHash{
		if log_mode == "1" || log_mode == "2" { //1,2 Log

			if log_mode == "1" { //1 Log not up to chain，2log up to chain
				chain_state = 5
			} else {
				chain_state = 0
			}

			DataSendLog := "TaskUUID:" + item.TaskUUID + " DataUUID:" + item.DataUUID
			LogType := int64(1) //src log
			SrcDataLog := model.VDESrcDataLog{
				DataUUID:            item.DataUUID,
				TaskUUID:            item.TaskUUID,
				Log:                 DataSendLog,
				LogType:             LogType,
				LogSender:           vde_src_pubkey,
				BlockchainHttp:      blockchain_http,
				BlockchainEcosystem: blockchain_ecosystem,
				ChainState:          chain_state,
				CreateTime:          time.Now().Unix()}

			if err = SrcDataLog.Create(); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Insert vde_src_data_log table failed")
				continue
			}
			//fmt.Println("Insert vde_src_data_log table ok")
		}

		item.DataState = 1 //
		err = item.Updates()
		if err != nil {
			log.WithError(err)
			continue
		}
	} //for
	return nil
}
