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
