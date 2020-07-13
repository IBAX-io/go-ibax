/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpclient

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network"

	log "github.com/sirupsen/logrus"
)

func SendSubNodeAgentData(host string, TaskUUID string, DataUUID string, AgentMode string, TranMode string, DataInfo string, SubNodeSrcPubkey string, SubNodeAgentPubkey string, SubNodeAgentIp string, SubNodeDestPubkey string, SubNodeDestIp string, dt []byte) (hash string) {
	conn, err := newConnection(host)
	if err != nil {
	rt := &network.RequestType{Type: network.RequestTypeSendSubNodeAgentData}
	if err = rt.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending request type")
		return "0"
	}

	req := &network.SubNodeAgentDataRequest{
		TaskUUID:           TaskUUID,
		DataUUID:           DataUUID,
		AgentMode:          AgentMode,
		TranMode:           TranMode,
		DataInfo:           DataInfo,
		SubNodeSrcPubkey:   SubNodeSrcPubkey,
		SubNodeAgentPubkey: SubNodeAgentPubkey,
		SubNodeAgentIp:     SubNodeAgentIp,
		SubNodeDestPubkey:  SubNodeDestPubkey,
		SubNodeDestIp:      SubNodeDestIp,
		Data:               dt,
	}

	if err = req.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending SubNodeSrcData request")
		return "0"
	}

	resp := &network.SubNodeAgentDataResponse{}

	if err = resp.Read(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("receiving SubNodeSrcData response")
		return "0"
	}
	return string(resp.Hash)
}
