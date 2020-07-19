/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

func SubNodeSrcData(ctx context.Context, d *daemon) error {
	var (
		TaskParms map[string]interface{}

		subnode_src_pubkey   string
		subnode_dest_pubkey  string
		subnode_dest_ip      string
		subnode_agent_pubkey string
		subnode_agent_ip     string
		agent_mode           string
		tran_mode            string
		//log_mode             string
		blockchain_table     string
		blockchain_http      string
		blockchain_ecosystem string

		//chain_state int64

		ok  bool
		err error
	)

	m := &model.SubNodeSrcData{}
	ShareData, err := m.GetAllByDataStatus(0) //0 not deal，1 success，2 fail
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

		m := &model.SubNodeSrcTask{}
		ShareTask, err := m.GetAllByTaskUUIDAndTaskState(item.TaskUUID, 1) //1 valid task，2 stop task
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("SubNodeSrcData SubNodeSrcTask getting one task by TaskUUID")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		if len(ShareTask) > 0 {
			TaskParms_Str = ShareTask[0].Parms
		} else {
			//m2 := &model.SubNodeSrcTaskFromSche{}
			//ShareTask2, err := m2.GetAllByTaskUUIDAndTaskState(item.TaskUUID, 1) //1 valid task，2 stop task
			//if err != nil {
			//	log.WithFields(log.Fields{"error": err}).Error("VDESrcData VDESrcTask getting one task by TaskUUID")
			//	time.Sleep(time.Millisecond * 2)
			//	continue
			//}
			//if len(ShareTask2) > 0 {
			//	TaskParms_Str = ShareTask2[0].Parms
			//} else {
			//	log.WithFields(log.Fields{"error": err}).Error("VDESrcData VDESrcTask getting one task by TaskUUID not found")
			//	item.DataState = 2 //not found
			//	err = item.Updates()
			//	if err != nil {
			//		log.WithError(err)
			//	}
			//	time.Sleep(time.Millisecond * 2)
			//	continue
			//}

			log.WithFields(log.Fields{"error": err}).Error("SubNodeSrcData SubNodeSrcTask getting one task by TaskUUID not found")
			item.DataState = 2 //not found err
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			time.Sleep(time.Millisecond * 2)
			continue
		}

		err = json.Unmarshal([]byte(TaskParms_Str), &TaskParms)
		if err != nil {
			log.Info("Error parsing task parameter")
			log.WithError(err)
			item.DataState = 3 // param err
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}

		if subnode_src_pubkey, ok = TaskParms["subnode_src_pubkey"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("subnode_src_pubkey parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if subnode_dest_pubkey, ok = TaskParms["subnode_dest_pubkey"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("subnode_dest_pubkey parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if subnode_dest_ip, ok = TaskParms["subnode_dest_ip"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("subnode_dest_ip parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if subnode_agent_pubkey, ok = TaskParms["subnode_agent_pubkey"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("subnode_agent_pubkey parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if subnode_agent_ip, ok = TaskParms["subnode_agent_ip"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("subnode_agent_ip parse error")
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
		if tran_mode, ok = TaskParms["tran_mode"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("tran_mode parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		//if log_mode, ok = TaskParms["log_mode"].(string); !ok {
		//	log.WithFields(log.Fields{"error": err}).Error("log_mode parse error")
		//	item.DataState = 3 //Indicates an error in parsing task parameters
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}
		if blockchain_table, ok = TaskParms["blockchain_table"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("blockchain_table parse error")
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

		//Handle the case of multiple target VDE nodes
		subnode_dest_pubkey_slice := strings.Split(subnode_dest_pubkey, ";")
		subnode_dest_ip_slice := strings.Split(subnode_dest_ip, ";")
		subnode_agent_pubkey_slice := strings.Split(subnode_agent_pubkey, ";")
		subnode_agent_ip_slice := strings.Split(subnode_agent_ip, ";")
		agent_mode_slice := strings.Split(agent_mode, ";")

		subnode_dest_num := len(subnode_dest_pubkey_slice)
		if len(subnode_dest_ip_slice) != subnode_dest_num {
			log.WithFields(log.Fields{"error": err}).Error("subnode_dest_ip parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if len(subnode_agent_pubkey_slice) != subnode_dest_num {
			log.WithFields(log.Fields{"error": err}).Error("subnode_agent_pubkey parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if len(subnode_agent_ip_slice) != subnode_dest_num {
			log.WithFields(log.Fields{"error": err}).Error("vde_agent_ip parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if len(agent_mode_slice) != subnode_dest_num {
			log.WithFields(log.Fields{"error": err}).Error("agent_mode parse error")
			item.DataState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if tran_mode == "1" || tran_mode == "3" { //hash data or all data under chain
			fmt.Println("==subnode_dest_pubkey_slice:", subnode_dest_pubkey_slice)
			for index, subnode_dest_pubkey_item := range subnode_dest_pubkey_slice {
				//Generate data send request
				fmt.Println("==index:", index)
				fmt.Println("==subnode_dest_pubkey_item:", subnode_dest_pubkey_item)
				SrcDataStatus := model.SubNodeSrcDataStatus{
					DataUUID:           item.DataUUID,
					TaskUUID:           item.TaskUUID,
					Hash:               item.Hash,
					Data:               item.Data,
					DataInfo:           item.DataInfo,
					TranMode:           converter.StrToInt64(tran_mode),
					SubNodeSrcPubkey:   subnode_src_pubkey,
					SubNodeDestPubkey:  subnode_dest_pubkey_item,
					SubNodeDestIP:      subnode_dest_ip_slice[index],
					SubNodeAgentPubkey: subnode_agent_pubkey_slice[index],
					SubNodeAgentIP:     subnode_agent_ip_slice[index],
					AgentMode:          converter.StrToInt64(agent_mode_slice[index]),
					CreateTime:         time.Now().Unix()}
				if err = SrcDataStatus.Create(); err != nil {
					log.WithFields(log.Fields{"error": err}).Error("Insert subnode_src_data_status table failed")
					continue
				}
				fmt.Println("Insert subnode_src_data_status table ok")
			}
		}
		if tran_mode == "1" { //hash upto chain
			//Generate data hash upto chain request
			SrcDataChainStatus := model.SubNodeSrcDataChainStatus{
			if err = SrcDataChainStatus.Create(); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Insert subnode_src_data_chain_status table failed")
				continue
			}
			fmt.Println("Insert subnode_src_data_chain_status table ok")
		}
		if tran_mode == "2" { //all data upto chain
			//Generate all data upto chain request
			for _, subnode_dest_pubkey_item := range subnode_dest_pubkey_slice {
				//Generate data send request
				SrcDataChainStatus := model.SubNodeSrcDataChainStatus{
					DataUUID:            item.DataUUID,
					TaskUUID:            item.TaskUUID,
					Hash:                item.Hash,
					Data:                item.Data,
					DataInfo:            item.DataInfo,
					TranMode:            converter.StrToInt64(tran_mode),
					SubNodeDestPubkey:   subnode_dest_pubkey_item,
					BlockchainTable:     blockchain_table,
					BlockchainHttp:      blockchain_http,
					BlockchainEcosystem: blockchain_ecosystem,
					CreateTime:          time.Now().Unix()}
				if err = SrcDataChainStatus.Create(); err != nil {
					log.WithFields(log.Fields{"error": err}).Error("Insert subnode_src_data_chain_status table failed")
					continue
				}
				fmt.Println("Insert subnode_src_data_chain_status table ok")
			}
		}

		////Generate a chain request on the log
		//if log_mode == "1" || log_mode == "2" { //1,2 Log
		//
		//	if log_mode == "1" { //1
		//		chain_state = 5
		//	} else {
		//		chain_state = 0
		//	}
		//
		//	DataSendLog := "TaskUUID:" + item.TaskUUID + " DataUUID:" + item.DataUUID
		//	LogType := int64(1) //src log
		//	SrcDataLog := model.VDESrcDataLog{
		//		DataUUID:            item.DataUUID,
		//		TaskUUID:            item.TaskUUID,
		//		Log:                 DataSendLog,
		//		LogType:             LogType,
		//		LogSender:           vde_src_pubkey,
		//		BlockchainHttp:      blockchain_http,
		//		BlockchainEcosystem: blockchain_ecosystem,
		//		ChainState:          chain_state,
		//		CreateTime:          time.Now().Unix()}
		//
		//	if err = SrcDataLog.Create(); err != nil {
		//		log.WithFields(log.Fields{"error": err}).Error("Insert vde_src_data_log table failed")
		//		continue
		//	}
		//	//fmt.Println("Insert vde_src_data_log table ok")
		//}

		item.DataState = 1 //
		err = item.Updates()
		if err != nil {
			log.WithError(err)
			continue
		}
	} //for
	return nil
}
