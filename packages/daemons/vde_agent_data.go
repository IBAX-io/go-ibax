/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/crypto/ecies"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"

	log "github.com/sirupsen/logrus"
)

func VDEAgentData(ctx context.Context, d *daemon) error {
	var (
		LogMode              int64
		log_type             int64
		log_err              string
		chain_state          int64
		blockchain_http      string
		blockchain_ecosystem string
	)
	m := &model.VDEAgentData{}
	ShareData, err := m.GetAllByDataSendStatus(0) //0 not send，1 success，2 fail
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all unsent task data")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	if len(ShareData) == 0 {
		//log.Info("task data from agent to dest not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}
	chaininfo := &model.VDEAgentChainInfo{}
	AgentChainInfo, err := chaininfo.Get()
	if err != nil {
		//log.WithFields(log.Fields{"error": err}).Error("VDE Agent fromchain getting chain info")
		log.Info("Agent chain info not found")
		time.Sleep(time.Millisecond * 100)
		return err
	}
	if AgentChainInfo == nil {
		log.Info("Agent chain info not found")
		//fmt.Println("Agent chain info not found")
		LogMode = 0 //0
		blockchain_http = ""
		blockchain_ecosystem = ""
	} else {
		LogMode = AgentChainInfo.LogMode
		blockchain_http = AgentChainInfo.BlockchainHttp
		blockchain_ecosystem = AgentChainInfo.BlockchainEcosystem
	}

	// send task data
	for _, item := range ShareData {
		//ItemDataBytes := item.Data
		ItemDataBytes, err := ecies.EccCryptoKey(item.Data, item.VDEDestPubkey)
		if err != nil {
			log.WithError(err)
			continue
		}
		//fmt.Println("item.AgentMode:", converter.Int64ToStr(item.AgentMode))
		fmt.Println("Send agent data, DataUUID:", item.DataUUID)
		hash := tcpclient.SendVDEAgentData(item.VDEDestIp, item.TaskUUID, item.DataUUID, converter.Int64ToStr(item.AgentMode), item.DataInfo, item.VDESrcPubkey, item.VDEAgentPubkey, item.VDEAgentIp, item.VDEDestPubkey, item.VDEDestIp, ItemDataBytes)
		if string(hash) == "0" {
			//item.DataSendState = 3 //
			item.DataSendState = 0 //
			log.Info("Hash mismatch")
		}
		err = item.Updates()
		if err != nil {
			log.WithError(err)
		}
		log_err = item.DataSendErr
		//Generate a chain request on the log
		log_type = 3      //
		if LogMode == 3 { //0
			//fmt.Println("There is no need to generate a log")
		} else if LogMode == 1 || LogMode == 2 {
			if LogMode == 1 { //1
				chain_state = 5
			} else {
				chain_state = 0
			}
			DataSendLog := "TaskUUID:" + item.TaskUUID + " DataUUID:" + item.DataUUID + "Log:" + log_err

			SrcDataLog := model.VDEAgentDataLog{
				DataUUID:            item.DataUUID,
				TaskUUID:            item.TaskUUID,
				Log:                 DataSendLog,
				LogType:             log_type,
				LogSender:           item.VDEAgentPubkey,
				BlockchainHttp:      blockchain_http,
				BlockchainEcosystem: blockchain_ecosystem,
				ChainState:          chain_state,
				CreateTime:          time.Now().Unix()}

			if err = SrcDataLog.Create(); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Insert vde_agent_data_log table failed")
				continue
			}
			//fmt.Println("Insert vde_agent_data_log table ok")
		} else {
			fmt.Println("Log mode err!")
		}

	} //for

	return nil
}
