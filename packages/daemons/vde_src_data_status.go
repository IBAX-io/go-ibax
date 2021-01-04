/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"time"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/crypto/ecies"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"

	log "github.com/sirupsen/logrus"
)

func VDESrcDataStatus(ctx context.Context, d *daemon) error {
	m := &model.VDESrcDataStatus{}
	//ShareData, err := m.GetAllByDataSendStatus(0) //
	//ShareData, err := m.GetAllByDataSendStatusAndAgentMode(0, 0) //
	ShareData, err := m.GetAllByDataSendStatusAndAgentMode(0, 2) //sendstatus:0
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all unsent task data")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	if len(ShareData) == 0 {
		//log.Info("task data from src to dest not found")
		time.Sleep(time.Millisecond * 2)
		return nil
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
		//hash := tcpclient.SendVDESrcData(item.VDEDestIP,item.TaskUUID, item.DataUUID, converter.Int64ToStr(item.AgentMode), item.DataInfo, ItemDataBytes)
		hash := tcpclient.SendVDESrcData(item.VDEDestIP, item.TaskUUID, item.DataUUID, converter.Int64ToStr(item.AgentMode), item.DataInfo, item.VDESrcPubkey, item.VDEAgentPubkey, item.VDEAgentIP, item.VDEDestPubkey, item.VDEDestIP, ItemDataBytes)
		if string(hash) == "0" {
			//item.DataSendState = 3 //
			item.DataSendState = 0 //
			item.DataSendErr = "Network error"
		} else if string(hash) == string(item.Hash) {
			item.DataSendState = 1 //success
		} else {
			item.DataSendState = 2 //
			item.DataSendErr = "Hash mismatch"
		}
		err = item.Updates()
		if err != nil {
			log.WithError(err)
		}
		//time.Sleep(time.Millisecond * 2)
	} //for

	return nil
}

func VDESrcDataStatusAgent(ctx context.Context, d *daemon) error {
	m := &model.VDESrcDataStatus{}
	//ShareData, err := m.GetAllByDataSendStatus(0) //0
	ShareData, err := m.GetAllByDataSendStatusAndAgentMode(0, 1) //0
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all unsent task data")
		return err
	}
	if len(ShareData) == 0 {
		//log.Info("task data from src to agent not found")
		time.Sleep(time.Millisecond * 100)
		return nil
	}
			item.DataSendErr = "Network error"
		} else if string(hash) == string(item.Hash) {
			item.DataSendState = 1 //
		} else {
			item.DataSendState = 2 //
			item.DataSendErr = "Hash mismatch"
		}
		err = item.Updates()
		if err != nil {
			log.WithError(err)
		}

	} //for

	return nil
}
