/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
	"github.com/IBAX-io/go-ibax/packages/crypto/ecies"
)

func SubNodeAgentData(ctx context.Context, d *daemon) error {
	var (
	//LogMode              int64
	//log_type             int64
	//log_err              string
	//chain_state          int64
	//blockchain_http      string
	//blockchain_ecosystem string
	)
	m := &model.SubNodeAgentData{}
	ShareData, err := m.GetAllByDataSendStatus(0) //0not send，1success，2fail
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

	//chaininfo := &model.SubNodeAgentChainInfo{}
	//AgentChainInfo, err := chaininfo.Get()
	//if err != nil {
	//	//log.WithFields(log.Fields{"error": err}).Error("VDE Agent fromchain getting chain info")
	//	log.Info("Agent chain info not found")
	//	time.Sleep(time.Millisecond * 100)
	//	return err
	//}
	//if AgentChainInfo == nil {
	//	log.Info("Agent chain info not found")
	//	//fmt.Println("Agent chain info not found")
	//	LogMode = 0 //0
	//	blockchain_http = ""
	//	blockchain_ecosystem = ""
	//} else {
	//	LogMode = AgentChainInfo.LogMode
	//	blockchain_http = AgentChainInfo.BlockchainHttp
	//	blockchain_ecosystem = AgentChainInfo.BlockchainEcosystem
	//}

	nodePrivateKey, err := utils.GetNodePrivateKey()
	if err != nil || len(nodePrivateKey) < 1 {
		if err == nil {
			log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node private key is empty")
		}
		return err
	}

	// send task data
	for _, item := range ShareData {
		//
		dataBase64, err := base64.StdEncoding.DecodeString(string(item.Data))
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("base64 DecodeString err")
			fmt.Println("base64 DecodeString err")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		privateData, err := ecies.EccDeCrypto(dataBase64, nodePrivateKey)
		if err != nil {
			fmt.Println("Decryption error:", err)
			log.WithFields(log.Fields{"type": consts.CryptoError}).Error("Decryption error")
			continue
		}
		item.Data = privateData
		//

		//ItemDataBytes := item.Data
		ItemDataBytes, err := ecies.EccCryptoKey(item.Data, item.SubNodeDestPubkey)
		if err != nil {
			log.WithError(err)
			continue
		}
		//fmt.Println("item.AgentMode:", converter.Int64ToStr(item.AgentMode))
		fmt.Println("Send agent data, TaskUUID, DataUUID:", item.TaskUUID, item.DataUUID)
		hash := tcpclient.SendSubNodeAgentData(item.SubNodeDestIP, item.TaskUUID, item.DataUUID, converter.Int64ToStr(item.AgentMode), converter.Int64ToStr(item.TranMode), item.DataInfo, item.SubNodeSrcPubkey, item.SubNodeAgentPubkey, item.SubNodeAgentIP, item.SubNodeDestPubkey, item.SubNodeDestIP, ItemDataBytes)
		if string(hash) == "0" {
			//item.DataSendState = 3 //
			item.DataSendState = 0 //
			item.DataSendErr = "Network error"
			log.Info("Network error")
		} else if string(hash) == string(item.Hash) {
			item.DataSendState = 1 //
			item.DataSendErr = "Send successfully"
			log.Info("Send successfully")
		} else {
			item.DataSendState = 2
			item.DataSendErr = "Hash mismatch"
			log.Info("Hash mismatch")
		}
		err = item.Updates()
		if err != nil {
			log.WithError(err)
		}
		//} else if LogMode == 1 || LogMode == 2 {
		//	if LogMode == 1 { //1
		//		chain_state = 5
		//	} else {
		//		chain_state = 0
		//	}
		//	DataSendLog := "TaskUUID:" + item.TaskUUID + " DataUUID:" + item.DataUUID + "Log:" + log_err
		//
		//	SrcDataLog := model.SubNodeAgentDataLog{
		//		DataUUID:            item.DataUUID,
		//		TaskUUID:            item.TaskUUID,
		//		Log:                 DataSendLog,
		//		LogType:             log_type,
		//		LogSender:           item.SubNodeAgentPubkey,
		//		BlockchainHttp:      blockchain_http,
		//		BlockchainEcosystem: blockchain_ecosystem,
		//		ChainState:          chain_state,
		//		CreateTime:          time.Now().Unix()}
		//
		//	if err = SrcDataLog.Create(); err != nil {
		//		log.WithFields(log.Fields{"error": err}).Error("Insert subnode_agent_data_log table failed")
		//		continue
		//	}
		//	//fmt.Println("Insert vde_agent_data_log table ok")
		//} else {
		//	fmt.Println("Log mode err!")
		//}

	} //for

	return nil
}
