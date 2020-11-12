/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpserver

import (
	"net"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/service"

	log "github.com/sirupsen/logrus"
)

// HandleTCPRequest proceed TCP requests
func HandleTCPRequest(rw net.Conn) {
	dType := &network.RequestType{}
	err := dType.Read(rw)
	if err != nil {
		log.Errorf("read request type failed: %s", err)
		return
	}

	log.WithFields(log.Fields{"request_type": dType.Type}).Debug("tcpserver got request type")
	var response interface{}

	switch dType.Type {
	case network.RequestTypeHonorNode:
		if service.IsNodePaused() {
			return
		}
		err = Type1(rw)

	case network.RequestTypeNotHonorNode:
		if err = req.Read(rw); err == nil {
			response, err = Type4(req)
		}

	case network.RequestTypeBlockCollection:
		req := &network.GetBodiesRequest{}
		if err = req.Read(rw); err == nil {
			err = Type7(req, rw)
		}

	case network.RequestTypeMaxBlock:
		response, err = Type10()

	case network.RequestTypeSendSubNodeSrcData:
		req := &network.SubNodeSrcDataRequest{}
		if err = req.Read(rw); err == nil {
			response, err = Type200(req)
		}
	case network.RequestTypeSendSubNodeSrcDataAgent:
		req := &network.SubNodeSrcDataAgentRequest{}
		if err = req.Read(rw); err == nil {
			response, err = Type201(req)
		}
	case network.RequestTypeSendSubNodeAgentData:
		req := &network.SubNodeAgentDataRequest{}
		if err = req.Read(rw); err == nil {
			response, err = Type202(req)
		}
	//
	case network.RequestTypeSendVDESrcData:
		req := &network.VDESrcDataRequest{}
		if err = req.Read(rw); err == nil {
			response, err = Type100(req)
		}
	case network.RequestTypeSendVDESrcDataAgent:
		req := &network.VDESrcDataAgentRequest{}
		if err = req.Read(rw); err == nil {
			response, err = Type101(req)
		}
	case network.RequestTypeSendVDEAgentData:
		req := &network.VDEAgentDataRequest{}
		if err = req.Read(rw); err == nil {
			response, err = Type102(req)
		}

	case network.RequestTypeSendPrivateData:
		req := &network.PrivateDateRequest{}
		if err = req.Read(rw); err == nil {
			response, err = Type88(req)
		}

	case network.RequestTypeSendPrivateFile:
		req := &network.PrivateFileRequest{}
		if err = req.Read(rw); err == nil {
			response, err = Type99(req)
		}
	}

	if err != nil || response == nil {
		return
	}

	log.WithFields(log.Fields{"response": response, "request_type": dType.Type}).Debug("tcpserver responded")
	if err = response.(network.SelfReaderWriter).Write(rw); err != nil {
		// err = SendRequest(response, rw)
		log.Errorf("tcpserver handle error: %s", err)
	}
}

// TcpListener is listening tcp address
func TcpListener(laddr string) error {

	if strings.HasPrefix(laddr, "127.") {
		log.Warn("Listening at local address: ", laddr)
	}

	l, err := net.Listen("tcp", laddr)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ConnectionError, "error": err, "host": laddr}).Error("Error listening")
		return err
	}

	go func() {
		defer l.Close()
		for {
			conn, err := l.Accept()
			if err != nil {
				log.WithFields(log.Fields{"type": consts.ConnectionError, "error": err, "host": laddr}).Error("Error accepting")
				time.Sleep(time.Second)
			} else {
				go func(conn net.Conn) {
					HandleTCPRequest(conn)
					conn.Close()
				}(conn)
			}
		}
	}()

	return nil
}
