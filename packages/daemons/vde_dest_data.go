/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"

	"time"

	"github.com/IBAX-io/go-ibax/packages/model"
)

func VDEDestData(ctx context.Context, d *daemon) error {
	var (
		TaskParms map[string]interface{}

		//vde_src_pubkey  string
		vde_dest_pubkey string
		//vde_dest_ip          string
		//vde_agent_pubkey     string
		//vde_agent_ip         string
		//agent_mode           string
		hash_mode            string
		log_mode             string
		blockchain_http      string
		blockchain_ecosystem string

		chain_state int64

		myHashState int64

		ok  bool
		err error
	)

	m := &model.VDEDestData{}
	ShareData, err := m.GetAllByDataStatus(0) //0 not deal，1 sucess，2 fail
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	if len(ShareData) == 0 {
		//log.Info("task data not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}

	// deal with task data
	var TaskParms_Str string
	for _, item := range ShareData {
		//fmt.Println("TaskUUID,DataUUID:", item.TaskUUID, item.DataUUID)
		m := &model.VDEDestTaskFromSrc{}
		ShareTask, err := m.GetAllByTaskUUIDAndTaskState(item.TaskUUID, 1) //1valid，0stop task
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("VDEDestTaskFromSrc getting one task by TaskUUID")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		if len(ShareTask) > 0 {
			TaskParms_Str = ShareTask[0].Parms
		} else {
			m2 := &model.VDEDestTaskFromSche{}
			ShareTask2, err := m2.GetAllByTaskUUIDAndTaskState(item.TaskUUID, 1) //1 valid task，2stop task
			if err != nil {
				log.WithFields(log.Fields{"error": err}).Error("VDEDestTaskFromSche getting one task by TaskUUID")
				time.Sleep(time.Millisecond * 2)
				continue
			}
			if len(ShareTask2) > 0 {
				TaskParms_Str = ShareTask2[0].Parms
			} else {
				log.Info("VDEDestTaskFromSrc and  VDEDestTaskFromSche getting one task by TaskUUID not found!")
				//log.WithFields(log.Fields{"error": err}).Error("VDEDestTaskFromSrc and  VDEDestTaskFromSche getting one task by TaskUUID not found")
				//item.DataState = 2 //
				//err = item.Updates()
				//if err != nil {
				//	log.WithError(err)
				//}
				time.Sleep(time.Millisecond * 2)
				continue
			}
		}
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
		//fmt.Println("ShareTask.Parms:",ShareTask.Parms)
		//if vde_src_pubkey, ok = TaskParms["vde_src_pubkey"].(string); !ok {
		//	log.WithFields(log.Fields{"error": err}).Error("src_vde_pubkey parse error")
		//	item.DataState = 3 //Indicates an error in parsing task parameters
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}
		if vde_dest_pubkey, ok = TaskParms["vde_dest_pubkey"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("vde_dest_pubkey parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		//if vde_dest_ip, ok = TaskParms["vde_dest_ip"].(string); !ok {
		//	log.WithFields(log.Fields{"error": err}).Error("vde_dest_ip parse error")
		//	item.DataState = 3 //Indicates an error in parsing task parameters
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}
		//if vde_agent_pubkey, ok = TaskParms["vde_agent_pubkey"].(string); !ok {
		//	log.WithFields(log.Fields{"error": err}).Error("vde_agent_pubkey parse error")
		//	item.DataState = 3 //Indicates an error in parsing task parameters
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}
		//if vde_agent_ip, ok = TaskParms["vde_agent_ip"].(string); !ok {
		//	log.WithFields(log.Fields{"error": err}).Error("vde_agent_ip parse error")
		//	item.DataState = 3 //Indicates an error in parsing task parameters
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}
		//if agent_mode, ok = TaskParms["agent_mode"].(string); !ok {
		//	log.WithFields(log.Fields{"error": err}).Error("agent_mode parse error")
		//	item.DataState = 3 //Indicates an error in parsing task parameters
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}
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
		//fmt.Println("agent_mode,hash_mode,log_mode:",agent_mode,hash_mode,log_mode)

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

		//if item.AgentMode == 0 { //0
		//	//fmt.Println("get task info from src")
		//	m := &model.VDEDestTaskFromSrc{}
		//	ShareTask, err := m.GetOneByTaskUUID(item.TaskUUID, 1)  //1
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	err = json.Unmarshal([]byte(ShareTask.Parms), &TaskParms)
		//	if err != nil {
		//		log.Info("Error parsing task parameter")
		//		log.WithError(err)
		//		item.DataState = 3 //
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	//fmt.Println("ShareTask.Parms:",ShareTask.Parms)
		//	if vde_src_pubkey, ok = TaskParms["vde_src_pubkey"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("src_vde_pubkey parse error")
		//		item.DataState = 3 //
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if vde_dest_pubkey, ok = TaskParms["vde_dest_pubkey"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("vde_dest_pubkey parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if vde_dest_ip, ok = TaskParms["vde_dest_ip"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("vde_dest_ip parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if vde_agent_pubkey, ok = TaskParms["vde_agent_pubkey"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("vde_agent_pubkey parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if vde_agent_ip, ok = TaskParms["vde_agent_ip"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("vde_agent_ip parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if agent_mode, ok = TaskParms["agent_mode"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("agent_mode parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if hash_mode, ok = TaskParms["hash_mode"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("hash_mode parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if log_mode, ok = TaskParms["log_mode"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("log_mode parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	//fmt.Println("agent_mode,hash_mode,log_mode:",agent_mode,hash_mode,log_mode)
		//
		//	if blockchain_http, ok = TaskParms["blockchain_http"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("blockchain_http parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if blockchain_ecosystem, ok = TaskParms["blockchain_ecosystem"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("blockchain_ecosystem parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//}else if item.AgentMode == 1 {
		//	//fmt.Println("get task info from sche")
		//	m := &model.VDEDestTaskFromSche{}
		//	ShareTask, err := m.GetOneByTaskUUID(item.TaskUUID, 1)  //1
		//	if err != nil {
		//		log.WithFields(log.Fields{"error": err}).Error("VDEDestTaskFromSche getting one task by TaskUUID")
		//		time.Sleep(time.Millisecond * 100)
		//		continue
		//	}
		//	if ShareTask == nil {
		//		log.Info("task by TaskUUID not found")
		//		item.DataState = 2 //
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	err = json.Unmarshal([]byte(ShareTask.Parms), &TaskParms)
		//	if err != nil {
		//		log.Info("Error parsing task parameter")
		//		log.WithError(err)
		//		item.DataState = 3 //
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//
		//	if vde_src_pubkey, ok = TaskParms["vde_src_pubkey"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("src_vde_pubkey parse error")
		//		item.DataState = 3 //
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if vde_dest_pubkey, ok = TaskParms["vde_dest_pubkey"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("vde_dest_pubkey parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if vde_dest_ip, ok = TaskParms["vde_dest_ip"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("vde_dest_ip parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if vde_agent_pubkey, ok = TaskParms["vde_agent_pubkey"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("vde_agent_pubkey parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if vde_agent_ip, ok = TaskParms["vde_agent_ip"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("vde_agent_ip parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if agent_mode, ok = TaskParms["agent_mode"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("agent_mode parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if hash_mode, ok = TaskParms["hash_mode"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("hash_mode parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		// 		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if log_mode, ok = TaskParms["log_mode"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("log_mode parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if blockchain_http, ok = TaskParms["blockchain_http"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("blockchain_http parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//	if blockchain_ecosystem, ok = TaskParms["blockchain_ecosystem"].(string); !ok {
		//		log.WithFields(log.Fields{"error": err}).Error("blockchain_ecosystem parse error")
		//		item.DataState = 3 //Indicates an error in parsing task parameters
		//		err = item.Updates()
		//		if err != nil {
		//			log.WithError(err)
		//		}
		//		continue
		//	}
		//}else {
		//	log.WithFields(log.Fields{"error": err}).Error("getting one task AgentMode")
		//	item.DataState = 4 //Indicates an error in parsing task parameters
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	time.Sleep(time.Millisecond * 100)
		//	continue
		//}

		//fmt.Println("TaskParms:",TaskParms)
		// fmt.Println("TaskParms:")
		//fmt.Println("vde_src_pubkey:", vde_src_pubkey)
		//fmt.Println("vde_dest_pubkey:", vde_dest_pubkey)
		//fmt.Println("vde_dest_ip:", vde_dest_ip)
		//fmt.Println("vde_agent_pubkey:", vde_agent_pubkey)
		//fmt.Println("vde_agent_ip:", vde_agent_ip)
		//fmt.Println("agent_mode,hash_mode,log_mode:", agent_mode, hash_mode, log_mode)
		//fmt.Println("blockchain_http,blockchain_ecosystem:", blockchain_http, blockchain_ecosystem)

		//Hash validity

		if hash_mode == "1" { //1HASH，2HASHnot
			myHashState = 0 //
		} else {
			myHashState = 3 //Indicates an error in parsing task parameters
		}
		//Generate a chain request on the log
		if log_mode == "1" || log_mode == "2" { //1,2 Log
			if log_mode == "1" { //1
				chain_state = 5
			} else {
				chain_state = 0
			}
			DataSendLog := "TaskUUID:" + item.TaskUUID + " DataUUID:" + item.DataUUID
			LogType := int64(2) //
			DestDataLog := model.VDEDestDataLog{
				DataUUID:            item.DataUUID,
				TaskUUID:            item.TaskUUID,
				Log:                 DataSendLog,
				LogType:             LogType,
				LogSender:           vde_dest_pubkey,
				BlockchainHttp:      blockchain_http,
				BlockchainEcosystem: blockchain_ecosystem,
				ChainState:          chain_state,
				CreateTime:          time.Now().Unix()}

			if err = DestDataLog.Create(); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Insert vde_dest_data_log table failed")
				continue
			}
			fmt.Println("Insert vde_dest_data_log table ok, DataUUID:", item.DataUUID)
		}

		//Generate data result
		DestDataStatus := model.VDEDestDataStatus{
			DataUUID:       item.DataUUID,
			TaskUUID:       item.TaskUUID,
			Hash:           item.Hash,
			Data:           item.Data,
			DataInfo:       item.DataInfo,
			VDESrcPubkey:   item.VDESrcPubkey,
			VDEDestPubkey:  item.VDEDestPubkey,
			VDEDestIp:      item.VDEDestIp,
			VDEAgentPubkey: item.VDEAgentPubkey,
			VDEAgentIp:     item.VDEAgentIp,
			AgentMode:      item.AgentMode,
			AuthState:      1,
			SignState:      1,
			HashState:      myHashState,
			CreateTime:     item.CreateTime}
		//CreateTime: time.Now().Unix()}

		if err = DestDataStatus.Create(); err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Insert vde_dest_data_status table failed")
			continue
		}
		fmt.Println("Insert vde_dest_data_status table ok, DataUUID:", item.DataUUID)

		item.DataState = 1 //
		err = item.Updates()
		//err = item.Delete()
		if err != nil {
			log.WithError(err)
			continue
		}

	} //for

	return nil
}
