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


	rt := &network.RequestType{Type: network.RequestTypeSendVDEAgentData}
	if err = rt.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending request type")
		return "0"
	}

	req := &network.VDEAgentDataRequest{
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

	resp := &network.VDEAgentDataResponse{}

	if err = resp.Read(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("receiving VDESrcData response")
		return "0"
	}
	return string(resp.Hash)
}
