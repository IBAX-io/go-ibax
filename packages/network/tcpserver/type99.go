/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpserver

import (
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/crypto"
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
		TaskSender: r.TaskSender,
		TaskType:   r.TaskType,
		MimeType:   r.MimeType,
		Name:       r.FileName,
		Hash:       hash,
		//Data: r.Data,
		Data: data,
	}

	err = PrivateFilePackets.Create()
	if err != nil {
		log.WithError(err)
		return nil, err
	}

	return resp, nil
}
