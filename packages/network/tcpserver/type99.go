/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpserver

import (
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/crypto"
	"github.com/IBAX-io/go-ibax/packages/crypto/ecies"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/network"

	log "github.com/sirupsen/logrus"
)

func Type99(r *network.PrivateFileRequest) (*network.PrivateFileResponse, error) {
	node_pri := syspar.GetNodePrivKey()

	data, err := ecies.EccDeCrypto(r.Data, node_pri)
	if err != nil {
		log.WithError(err)
		return nil, err
	}

	//hash, err := crypto.HashHex(r.Data)
	hash, err := crypto.HashHex(data)
	if err != nil {
		log.WithError(err)
		return nil, err
	}
	resp := &network.PrivateFileResponse{}
	resp.Hash = hash

	PrivateFilePackets := model.PrivateFilePackets{

		TaskUUID:   r.TaskUUID,
		TaskName:   r.TaskName,
		return nil, err
	}

	return resp, nil
}
