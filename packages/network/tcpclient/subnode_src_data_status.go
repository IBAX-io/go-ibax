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

//func SendSubNodeSrcData(host string, TaskUUID string, DataUUID string, AgentMode string, DataInfo string, dt []byte ) (hash string) {
func SendSubNodeSrcData(host string, TaskUUID string, DataUUID string, AgentMode string, TranMode string, DataInfo string, SubNodeSrcPubkey string, SubNodeAgentPubkey string, SubNodeAgentIp string, SubNodeDestPubkey string, SubNodeDestIp string, dt []byte) (hash string) {

	conn, err := newConnection(host)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.NetworkError, "error": err, "host": host}).Error("on creating tcp connection")
		return "0"
	}
	defer conn.Close()

	rt := &network.RequestType{Type: network.RequestTypeSendSubNodeSrcData}
	if err = rt.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending request type")
		return "0"
	}

	req := &network.SubNodeSrcDataRequest{
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
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending VDESrcData request")
		return "0"
	}

	resp := &network.SubNodeSrcDataResponse{}

	if err = resp.Read(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("receiving VDESrcData response")
		return "0"
	}

	return string(resp.Hash)
}

func SendSubNodeSrcDataAgent(host string, TaskUUID string, DataUUID string, AgentMode string, TranMode string, DataInfo string, SubNodeSrcPubkey string, SubNodeAgentPubkey string, SubNodeAgentIp string, SubNodeDestPubkey string, SubNodeDestIp string, dt []byte) (hash string) {
	conn, err := newConnection(host)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.NetworkError, "error": err, "host": host}).Error("on creating tcp connection")
		return "0"
	}
	defer conn.Close()

		SubNodeSrcPubkey:   SubNodeSrcPubkey,
		SubNodeAgentPubkey: SubNodeAgentPubkey,
		SubNodeAgentIp:     SubNodeAgentIp,
		SubNodeDestPubkey:  SubNodeDestPubkey,
		SubNodeDestIp:      SubNodeDestIp,
		Data:               dt,
	}

	if err = req.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending VDESrcDataAgent request")
		return "0"
	}

	resp := &network.SubNodeSrcDataResponse{}

	if err = resp.Read(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("receiving VDESrcDataAgent response")
		return "0"
	}

	return string(resp.Hash)
}
