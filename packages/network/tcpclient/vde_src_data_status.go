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

//func SendVDESrcData(host string, TaskUUID string, DataUUID string, AgentMode string, DataInfo string, dt []byte ) (hash string) {
func SendVDESrcData(host string, TaskUUID string, DataUUID string, AgentMode string, DataInfo string, VDESrcPubkey string, VDEAgentPubkey string, VDEAgentIp string, VDEDestPubkey string, VDEDestIp string, dt []byte) (hash string) {

	conn, err := newConnection(host)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.NetworkError, "error": err, "host": host}).Error("on creating tcp connection")
		return "0"
	}
	defer conn.Close()

	rt := &network.RequestType{Type: network.RequestTypeSendVDESrcData}
	if err = rt.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending request type")
		return "0"
	}

	req := &network.VDESrcDataRequest{
		TaskUUID:       TaskUUID,
		DataUUID:       DataUUID,
		AgentMode:      AgentMode,
		DataInfo:       DataInfo,
		VDESrcPubkey:   VDESrcPubkey,
		VDEAgentPubkey: VDEAgentPubkey,
		VDEAgentIp:     VDEAgentIp,
		VDEDestPubkey:  VDEDestPubkey,
		VDEDestIp:      VDEDestIp,
		Data:           dt,
	}

	if err = req.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending VDESrcData request")
		return "0"
	}

	resp := &network.VDESrcDataResponse{}

	if err = resp.Read(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("receiving VDESrcData response")
		return "0"
	}

	return string(resp.Hash)
}

func SendVDESrcDataAgent(host string, TaskUUID string, DataUUID string, AgentMode string, DataInfo string, VDESrcPubkey string, VDEAgentPubkey string, VDEAgentIp string, VDEDestPubkey string, VDEDestIp string, dt []byte) (hash string) {
	conn, err := newConnection(host)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.NetworkError, "error": err, "host": host}).Error("on creating tcp connection")
		return "0"
	}
	defer conn.Close()

	rt := &network.RequestType{Type: network.RequestTypeSendVDESrcDataAgent}
	if err = rt.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending request type")
		return "0"
	}

	req := &network.VDESrcDataAgentRequest{
		TaskUUID:       TaskUUID,
		DataUUID:       DataUUID,
	}

	if err = req.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending VDESrcDataAgent request")
		return "0"
	}

	resp := &network.VDESrcDataResponse{}

	if err = resp.Read(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("receiving VDESrcDataAgent response")
		return "0"
	}

	return string(resp.Hash)
}
