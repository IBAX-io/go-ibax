/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons
	log "github.com/sirupsen/logrus"
)

func SubNodeSrcDataStatus(ctx context.Context, d *daemon) error {
	m := &model.SubNodeSrcDataStatus{}
	//ShareData, err := m.GetAllByDataSendStatus(0) //0
	//ShareData, err := m.GetAllByDataSendStatusAndAgentMode(0, 0) //0
	ShareData, err := m.GetAllByDataSendStatusAndAgentMode(0, 2) //sendstatus:0Indicates that the contract has not been installed, 1 means that the contract is successfully installed, 2 means that the contract is not installed successfully; 0 means that the contract has not been uploaded yet, and 1 means that a request has been generated
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all unsent task data")
		time.Sleep(time.Millisecond * 200)
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
		ItemDataBytes, err := ecies.EccCryptoKey(item.Data, item.SubNodeDestPubkey)
		if err != nil {
			log.WithError(err)
			continue
		}
		//fmt.Println("item.AgentMode:", converter.Int64ToStr(item.AgentMode))
		//hash := tcpclient.SendVDESrcData(item.VDEDestIP,item.TaskUUID, item.DataUUID, converter.Int64ToStr(item.AgentMode), item.DataInfo, ItemDataBytes)
		hash := tcpclient.SendSubNodeSrcData(item.SubNodeDestIP, item.TaskUUID, item.DataUUID, converter.Int64ToStr(item.AgentMode), converter.Int64ToStr(item.TranMode), item.DataInfo, item.SubNodeSrcPubkey, item.SubNodeAgentPubkey, item.SubNodeAgentIP, item.SubNodeDestPubkey, item.SubNodeDestIP, ItemDataBytes)
		if string(hash) == "0" {
			//item.DataSendState = 3 //
			item.DataSendState = 0 //
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
		//time.Sleep(time.Millisecond * 2)
	} //for
	time.Sleep(time.Millisecond * 100)
	return nil
}

func SubNodeSrcDataStatusAgent(ctx context.Context, d *daemon) error {
	m := &model.SubNodeSrcDataStatus{}
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

	// send task data
	for _, item := range ShareData {
		//ItemDataBytes := item.Data
		ItemDataBytes, err := ecies.EccCryptoKey(item.Data, item.SubNodeAgentPubkey)
		if err != nil {
			log.WithError(err)
			continue
		}
		//fmt.Println("item.AgentMode:", converter.Int64ToStr(item.AgentMode))

		hash := tcpclient.SendSubNodeSrcDataAgent(item.SubNodeAgentIP, item.TaskUUID, item.DataUUID, converter.Int64ToStr(item.AgentMode), converter.Int64ToStr(item.TranMode), item.DataInfo, item.SubNodeSrcPubkey, item.SubNodeAgentPubkey, item.SubNodeAgentIP, item.SubNodeDestPubkey, item.SubNodeDestIP, ItemDataBytes)
		if string(hash) == "0" {
			//item.DataSendState = 3 //
			item.DataSendState = 0 //
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
