/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpclient

import (
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network"

	log "github.com/sirupsen/logrus"
)

func SentPrivateData(host string, dt []byte) (hash string) {
	conn, err := newConnection(host)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.NetworkError, "error": err, "host": host}).Error("on creating tcp connection")
		time.Sleep(time.Millisecond * 100)
		return "0"
	}
	defer conn.Close()

	rt := &network.RequestType{Type: network.RequestTypeSendPrivateData}
	if err = rt.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending request type")
		time.Sleep(time.Millisecond * 100)
		return "0"
	}

	req := &network.PrivateDateRequest{
		Data: dt,
	}

	if err = req.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending privatedata request")
		time.Sleep(time.Millisecond * 100)
		return "0"
	}

	resp := &network.PrivateDateResponse{}

	if err = resp.Read(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("receiving privatedata response")
		time.Sleep(time.Millisecond * 100)
		return "0"
	}

	return string(resp.Hash)
}

func SentPrivateFile(host string, TaskUUID string, TaskName string, TaskSender string, TaskType string, FileName string, MimeType string, dt []byte) (hash string) {
	req := &network.PrivateFileRequest{
		TaskUUID:   TaskUUID,
		TaskName:   TaskName,
		TaskSender: TaskSender,
		TaskType:   TaskType,
		FileName:   FileName,
		MimeType:   MimeType,
		Data:       dt,
	}

	if err = req.Write(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("sending privatefile request")
		time.Sleep(time.Millisecond * 100)
		return "0"
	}

	resp := &network.PrivateFileResponse{}

	if err = resp.Read(conn); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "host": host}).Error("receiving privatefile response")
		time.Sleep(time.Millisecond * 100)
		return "0"
	}

	return string(resp.Hash)
}
